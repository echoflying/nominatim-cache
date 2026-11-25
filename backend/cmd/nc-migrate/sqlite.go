package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/echoflying/nominatim-cache/backend/internal/store"
	_ "modernc.org/sqlite"
)

func openSnapshot(ctx context.Context, path string) (*sql.DB, string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, "", err
	}
	for _, suffix := range []string{"-wal", "-journal"} {
		info, err := os.Stat(path + suffix)
		if err == nil && info.Size() > 0 {
			return nil, "", errors.New("源库存在未合并的 WAL 或日志，请使用 SQLite .backup 生成独立快照后导入")
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, "", err
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, "", err
	}
	if !info.Mode().IsRegular() {
		return nil, "", errors.New("快照必须是普通文件")
	}
	digest := sha256.New()
	if _, err := io.Copy(digest, f); err != nil {
		return nil, "", err
	}
	db, err := sql.Open("sqlite", sqliteURI(path, "mode=ro&immutable=1"))
	if err != nil {
		return nil, "", err
	}
	db.SetMaxOpenConns(1)
	var integrity string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil {
		db.Close()
		return nil, "", err
	}
	if integrity != "ok" {
		db.Close()
		return nil, "", errors.New("SQLite 快照完整性检查失败")
	}
	return db, hex.EncodeToString(digest.Sum(nil)), nil
}

func sqliteURI(path, query string) string {
	filePath := filepath.ToSlash(path)
	if !strings.HasPrefix(filePath, "/") {
		filePath = "/" + filePath
	}
	return (&url.URL{Scheme: "file", Path: filePath, RawQuery: query}).String()
}

type rowReader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type reader struct {
	db            rowReader
	addressColumn string
}

func legacyReader(ctx context.Context, db *sql.DB) (reader, error) {
	rows, err := db.QueryContext(ctx, "PRAGMA table_info(cache_entries)")
	if err != nil {
		return reader{}, err
	}
	defer rows.Close()
	col := "NULL"
	for rows.Next() {
		var name, dataType string
		var position, notNull, pk int
		var def any
		if err := rows.Scan(&position, &name, &dataType, &notNull, &def, &pk); err != nil {
			return reader{}, err
		}
		if name == "address" {
			col = "address"
		}
	}
	return reader{db: db, addressColumn: col}, rows.Err()
}

type counts struct {
	CacheEntries int64 `json:"cache_entries"`
	AccessLogs   int64 `json:"access_logs"`
	Stats        int64 `json:"stats"`
}

type counters struct {
	Requests int64 `json:"total_requests"`
	Hits     int64 `json:"cache_hits"`
	Misses   int64 `json:"cache_misses"`
	Updated  int64 `json:"last_updated_at"`
}

type receipt struct {
	SHA256     string   `json:"sha256"`
	DataSHA256 string   `json:"data_sha256"`
	Counts     counts   `json:"counts"`
	Stats      counters `json:"stats"`
	ImportedAt int64    `json:"imported_at,omitempty"`
}

func (r reader) copy(ctx context.Context, tx *sql.Tx) (result receipt, err error) {
	digest := sha256.New()
	encoder := json.NewEncoder(digest)
	result.Counts.CacheEntries, err = r.copyCache(ctx, tx, encoder)
	if err != nil {
		return result, err
	}
	result.Counts.AccessLogs, err = r.copyLogs(ctx, tx, encoder)
	if err != nil {
		return result, err
	}
	err = r.db.QueryRowContext(ctx, "SELECT total_requests,cache_hits,cache_misses,last_updated_at FROM stats WHERE id=1").
		Scan(&result.Stats.Requests, &result.Stats.Hits, &result.Stats.Misses, &result.Stats.Updated)
	if err != nil {
		return result, fmt.Errorf("读取统计记录: %w", err)
	}
	result.Counts.Stats = 1
	if err := encoder.Encode(result.Stats); err != nil {
		return result, err
	}
	if tx != nil {
		if _, err := tx.ExecContext(ctx, "UPDATE stats SET total_requests=?,cache_hits=?,cache_misses=?,last_updated_at=? WHERE id=1",
			result.Stats.Requests, result.Stats.Hits, result.Stats.Misses, result.Stats.Updated); err != nil {
			return result, err
		}
	}
	result.DataSHA256 = hex.EncodeToString(digest.Sum(nil))
	return result, nil
}

func (r reader) copyCache(ctx context.Context, tx *sql.Tx, encoder *json.Encoder) (int64, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id,cache_key,lat,lon,display_name,"+r.addressColumn+
		",place_type,source,nominatim_response,first_cached_at,last_accessed_at,access_count FROM cache_entries ORDER BY id")
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var insert *sql.Stmt
	if tx != nil {
		insert, err = tx.PrepareContext(ctx, "INSERT INTO cache_entries (id,cache_key,lat,lon,display_name,address,place_type,source,nominatim_response,first_cached_at,last_accessed_at,access_count) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)")
		if err != nil {
			return 0, err
		}
		defer insert.Close()
	}
	var count int64
	for rows.Next() {
		var row struct {
			ID int64 `json:"id"`
			store.Cache
		}
		err := rows.Scan(&row.ID, &row.Key, &row.Lat, &row.Lon, &row.DisplayName, &row.Address,
			&row.Type, &row.Source, &row.Raw, &row.First, &row.Last, &row.Count)
		if err != nil {
			return 0, err
		}
		if !json.Valid([]byte(row.Raw)) || !strings.HasPrefix(strings.TrimSpace(row.Raw), "{") {
			return 0, fmt.Errorf("缓存 ID %d 的原始响应无效", row.ID)
		}
		if row.Address != nil && strings.TrimSpace(*row.Address) == "" {
			row.Address = nil
		}
		row.Lat += 0 // 统一 SQLite 与 MariaDB 的零值表示，旧缓存键原样保留。
		row.Lon += 0
		if err := encoder.Encode(row); err != nil {
			return 0, err
		}
		if insert != nil {
			_, err := insert.ExecContext(ctx, row.ID, row.Key, row.Lat, row.Lon, row.DisplayName, row.Address,
				row.Type, row.Source, row.Raw, row.First, row.Last, row.Count)
			if err != nil {
				return 0, fmt.Errorf("导入缓存 ID %d: %w", row.ID, err)
			}
		}
		count++
	}
	return count, rows.Err()
}

func (r reader) copyLogs(ctx context.Context, tx *sql.Tx, encoder *json.Encoder) (int64, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id,cache_key,lat,lon,cached,accessed_at FROM access_logs ORDER BY id")
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var insert *sql.Stmt
	if tx != nil {
		insert, err = tx.PrepareContext(ctx, "INSERT INTO access_logs (id,cache_key,lat,lon,cached,accessed_at) VALUES (?,?,?,?,?,?)")
		if err != nil {
			return 0, err
		}
		defer insert.Close()
	}
	var count int64
	for rows.Next() {
		var row store.AccessLog
		if err := rows.Scan(&row.ID, &row.Key, &row.Lat, &row.Lon, &row.Hit, &row.At); err != nil {
			return 0, err
		}
		if row.Lat != nil {
			*row.Lat += 0
		}
		if row.Lon != nil {
			*row.Lon += 0
		}
		if err := encoder.Encode(row); err != nil {
			return 0, err
		}
		if insert != nil {
			if _, err := insert.ExecContext(ctx, row.ID, row.Key, row.Lat, row.Lon, row.Hit, row.At); err != nil {
				return 0, fmt.Errorf("导入访问记录 ID %d: %w", row.ID, err)
			}
		}
		count++
	}
	return count, rows.Err()
}

func importSnapshot(ctx context.Context, source reader, target *sql.DB, sum string) (receipt, error) {
	tx, err := target.BeginTx(ctx, nil)
	if err != nil {
		return receipt{}, err
	}
	defer tx.Rollback()
	var empty bool
	err = tx.QueryRowContext(ctx, "SELECT total_requests=0 AND cache_hits=0 AND cache_misses=0 AND last_updated_at=0 FROM stats WHERE id=1 FOR UPDATE").Scan(&empty)
	if err != nil {
		return receipt{}, err
	}
	var previous string
	err = tx.QueryRowContext(ctx, "SELECT receipt FROM sqlite_imports WHERE checksum=?", sum).Scan(&previous)
	if err == nil {
		var result receipt
		err := json.Unmarshal([]byte(previous), &result)
		return result, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return receipt{}, err
	}
	var hasData bool
	err = tx.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM cache_entries) OR EXISTS (SELECT 1 FROM access_logs) OR EXISTS (SELECT 1 FROM batch_jobs) OR EXISTS (SELECT 1 FROM sqlite_imports)").Scan(&hasData)
	if err != nil {
		return receipt{}, err
	}
	if !empty || hasData {
		return receipt{}, errors.New("目标库已有数据，请使用未启动过应用的新库")
	}
	result, err := source.copy(ctx, tx)
	if err != nil {
		return receipt{}, err
	}
	maria := reader{db: tx, addressColumn: "address"}
	verified, err := maria.copy(ctx, nil)
	if err != nil {
		return receipt{}, err
	}
	if result.DataSHA256 != verified.DataSHA256 || result.Counts != verified.Counts {
		return receipt{}, errors.New("MariaDB 读回的行数或内容摘要与源 SQLite 不一致")
	}
	result.SHA256, result.ImportedAt = sum, time.Now().UnixMilli()
	raw, err := json.Marshal(result)
	if err != nil {
		return receipt{}, err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO sqlite_imports (checksum,imported_at,receipt) VALUES (?,?,?)", sum, result.ImportedAt, string(raw)); err != nil {
		return receipt{}, err
	}
	return result, tx.Commit()
}
