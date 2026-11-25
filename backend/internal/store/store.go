package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/echoflying/nominatim-cache/backend/internal/geocode"
	"github.com/go-sql-driver/mysql"
)

type Store struct {
	DB *sql.DB
}

func Open(dsn string) (*sql.DB, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil || cfg.DBName == "" {
		return nil, errors.New("需要有效的 MariaDB DSN 和明确的数据库名")
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 5 * time.Second
	}
	cfg.Collation = "utf8mb4_unicode_ci"
	cfg.MultiStatements = false
	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		return nil, errors.New("无法初始化 MariaDB 连接")
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(12)
	db.SetMaxIdleConns(6)
	db.SetConnMaxLifetime(5 * time.Minute)
	return db, nil
}

type Cache struct {
	Key         string  `json:"cache_key"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	DisplayName *string `json:"display_name"`
	Address     *string `json:"address"`
	Type        *string `json:"place_type"`
	Source      *string `json:"source"`
	Raw         string  `json:"nominatim_response"`
	First       int64   `json:"first_cached_at"`
	Last        int64   `json:"last_accessed_at"`
	Count       *int64  `json:"access_count"`
}

const cacheColumns = "cache_key,lat,lon,display_name,address,place_type,source,nominatim_response,first_cached_at,last_accessed_at,access_count"

type scanner interface{ Scan(...any) error }

func scanCache(row scanner) (Cache, error) {
	var c Cache
	err := row.Scan(&c.Key, &c.Lat, &c.Lon, &c.DisplayName, &c.Address, &c.Type, &c.Source, &c.Raw, &c.First, &c.Last, &c.Count)
	return c, err
}

func (c Cache) Original() (geocode.Response, error) {
	raw, err := geocode.DecodeResponse([]byte(c.Raw))
	if err != nil || raw == nil {
		return nil, errors.New("已缓存的响应格式无效")
	}
	return raw, nil
}

func (c Cache) Public() (geocode.Response, error) {
	raw, err := c.Original()
	if err != nil {
		return nil, err
	}
	r := geocode.Response{
		"lat": raw["lat"], "lon": raw["lon"], "display_name": c.DisplayName,
		"address": raw.Address(), "cached": true,
		"cache_stats": map[string]any{"first_cached_at": c.First, "last_accessed_at": c.Last, "access_count": c.Count},
	}
	// 地址质量应读取完整原始响应，其中保留了类别、POI 名及类型。
	p := geocode.WithPlace(raw)
	for _, key := range []string{"place_name", "place_level", "place_quality", "place_source_field", "quality_reason"} {
		if value, ok := p[key]; ok {
			r[key] = value
		}
	}
	return r, nil
}

func (s Store) Cache(ctx context.Context, key string) (Cache, error) {
	return scanCache(s.DB.QueryRowContext(ctx, "SELECT "+cacheColumns+" FROM cache_entries WHERE cache_key=?", key))
}

func (s Store) Touch(ctx context.Context, key string, now int64) error {
	_, err := s.DB.ExecContext(ctx, "UPDATE cache_entries SET last_accessed_at=?,access_count=COALESCE(access_count,0)+1 WHERE cache_key=?", now, key)
	return err
}

type execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func insertCache(ctx context.Context, tx execer, point geocode.Point, key string, r geocode.Response, now int64) error {
	raw, err := json.Marshal(r)
	if err != nil {
		return err
	}
	address, err := json.Marshal(r.Address())
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO cache_entries ("+cacheColumns+") VALUES (?,?,?,?,?,?,?,?,?,?,1) "+
		"ON DUPLICATE KEY UPDATE cache_key=VALUES(cache_key)", key, point.Lat, point.Lon,
		r.Text("display_name"), string(address), geocode.PlaceType(r), r.Text("_source"), string(raw), now, now)
	return err
}

func (s Store) InsertCache(ctx context.Context, point geocode.Point, key string, r geocode.Response) error {
	return insertCache(ctx, s.DB, point, key, r, time.Now().UnixMilli())
}

func (s Store) ManualCache(ctx context.Context, point geocode.Point, displayName string, address map[string]any) (bool, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	key, now := point.Key(), time.Now().UnixMilli()
	var previousType sql.NullString
	err = tx.QueryRowContext(ctx, "SELECT place_type FROM cache_entries WHERE cache_key=? FOR UPDATE", key).Scan(&previousType)
	exists := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	response := geocode.Response{"lat": point.Lat, "lon": point.Lon, "display_name": displayName, "address": address}
	raw, err := json.Marshal(response)
	if err != nil {
		return false, err
	}
	formattedAddress, err := json.Marshal(address)
	if err != nil {
		return false, err
	}
	if exists {
		_, err = tx.ExecContext(ctx, "UPDATE cache_entries SET lat=?,lon=?,display_name=?,address=?,source='manual',nominatim_response=?,first_cached_at=? WHERE cache_key=?",
			point.Lat, point.Lon, displayName, string(formattedAddress), string(raw), now, key)
	} else {
		_, err = tx.ExecContext(ctx, "INSERT INTO cache_entries ("+cacheColumns+") VALUES (?,?,?,?,?,?,'manual',?,?,?,1)",
			key, point.Lat, point.Lon, displayName, string(formattedAddress), "other", string(raw), now, now)
	}
	if err != nil {
		return false, err
	}
	return exists, tx.Commit()
}

func (s Store) DeleteCache(ctx context.Context, key string) (int64, error) {
	result, err := s.DB.ExecContext(ctx, "DELETE FROM cache_entries WHERE cache_key=?", key)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s Store) ClearCache(ctx context.Context) (int64, error) {
	result, err := s.DB.ExecContext(ctx, "DELETE FROM cache_entries")
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s Store) ListCache(ctx context.Context, search, order, sort string, limit, offset int) ([]Cache, int64, error) {
	where := ""
	args := []any{}
	if search != "" {
		where = " WHERE display_name LIKE ?"
		args = append(args, "%"+search+"%")
	}
	var total int64
	if err := s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM cache_entries"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	column, direction := "last_accessed_at", "DESC"
	if sort == "cached" {
		column = "first_cached_at"
	}
	if strings.EqualFold(order, "ASC") {
		direction = "ASC"
	}
	rows, err := s.DB.QueryContext(ctx, "SELECT "+cacheColumns+" FROM cache_entries"+where+
		" ORDER BY "+column+" "+direction+",id "+direction+" LIMIT ? OFFSET ?", append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result := make([]Cache, 0)
	for rows.Next() {
		c, err := scanCache(rows)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, c)
	}
	return result, total, rows.Err()
}

type Stats struct {
	Total      int64   `json:"total_requests"`
	Hits       int64   `json:"cache_hits"`
	Misses     int64   `json:"cache_misses"`
	HitRate    float64 `json:"hit_rate"`
	CacheCount int64   `json:"cache_count"`
	Uptime     int64   `json:"uptime"`
	Updated    int64   `json:"last_updated_at"`
}

func (s Store) Stats(ctx context.Context, started time.Time) (Stats, error) {
	var result Stats
	err := s.DB.QueryRowContext(ctx, "SELECT total_requests,cache_hits,cache_misses,last_updated_at,(SELECT COUNT(*) FROM cache_entries) FROM stats WHERE id=1").
		Scan(&result.Total, &result.Hits, &result.Misses, &result.Updated, &result.CacheCount)
	if result.Total > 0 {
		result.HitRate = math.Round(1000*float64(result.Hits)/float64(result.Total)) / 10
	}
	result.Uptime = int64(time.Since(started).Seconds())
	return result, err
}

func (s Store) RecordAccess(ctx context.Context, p geocode.Point, key string, hit, success bool) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UnixMilli()
	if _, err := tx.ExecContext(ctx, "INSERT INTO access_logs (cache_key,lat,lon,cached,accessed_at) VALUES (?,?,?,?,?)",
		key, p.Lat, p.Lon, hit, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE stats SET total_requests=total_requests+1, cache_hits=cache_hits+?, cache_misses=cache_misses+?, last_updated_at=? WHERE id=1",
		hit, !hit && success, now); err != nil {
		return err
	}
	return tx.Commit()
}

type AccessLog struct {
	ID    int64    `json:"id"`
	Key   *string  `json:"cache_key"`
	Lat   *float64 `json:"lat"`
	Lon   *float64 `json:"lon"`
	Hit   int      `json:"cached"`
	At    int64    `json:"accessed_at"`
	Label string   `json:"accessed_at_str"`
}

func (s Store) Logs(ctx context.Context, hit *int, zone *time.Location, limit, offset int) ([]AccessLog, int64, error) {
	where := ""
	args := []any{}
	if hit != nil {
		where, args = " WHERE cached=?", []any{*hit}
	}
	var total int64
	if err := s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM access_logs"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.DB.QueryContext(ctx, "SELECT id,cache_key,lat,lon,cached,accessed_at FROM access_logs"+where+
		" ORDER BY accessed_at DESC,id DESC LIMIT ? OFFSET ?", append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result := make([]AccessLog, 0)
	for rows.Next() {
		var item AccessLog
		if err := rows.Scan(&item.ID, &item.Key, &item.Lat, &item.Lon, &item.Hit, &item.At); err != nil {
			return nil, 0, err
		}
		item.Label = time.UnixMilli(item.At).In(zone).Format("2006/1/2 15:04:05")
		result = append(result, item)
	}
	return result, total, rows.Err()
}

type Day struct {
	Date   string `json:"date"`
	Label  string `json:"label"`
	Hits   int64  `json:"hits"`
	Misses int64  `json:"misses"`
	Total  int64  `json:"total"`
}

func (s Store) Daily(ctx context.Context, now time.Time) ([]Day, error) {
	y, m, d := now.Date()
	start := time.Date(y, m, d, 0, 0, 0, 0, now.Location()).AddDate(0, 0, -6)
	result := make([]Day, 0, 7)
	for range 7 {
		end := start.AddDate(0, 0, 1)
		item := Day{Date: start.Format(time.DateOnly), Label: fmt.Sprintf("%d/%d", start.Month(), start.Day())}
		err := s.DB.QueryRowContext(ctx, "SELECT COUNT(*),COALESCE(SUM(cached=1),0) FROM access_logs WHERE accessed_at>=? AND accessed_at<?",
			start.UnixMilli(), end.UnixMilli()).Scan(&item.Total, &item.Hits)
		if err != nil {
			return nil, err
		}
		item.Misses = item.Total - item.Hits
		result = append(result, item)
		start = end
	}
	return result, nil
}

func (s Store) Prune(ctx context.Context, now time.Time, logRetention, batchRetention time.Duration) error {
	for _, deletion := range []struct {
		query string
		age   time.Duration
	}{
		{"DELETE FROM access_logs WHERE accessed_at<? ORDER BY accessed_at LIMIT 5000", logRetention},
		{"DELETE FROM batch_jobs WHERE finished_at<? ORDER BY finished_at LIMIT 50", batchRetention},
	} {
		for {
			result, err := s.DB.ExecContext(ctx, deletion.query, now.Add(-deletion.age).UnixMilli())
			if err != nil {
				return err
			}
			count, err := result.RowsAffected()
			if err != nil {
				return err
			}
			if count == 0 {
				break
			}
		}
	}
	return nil
}
