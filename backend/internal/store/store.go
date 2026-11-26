package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
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
	UpstreamArchiveID *int64  `json:"-"`
	Key               string  `json:"cache_key"`
	Lat               float64 `json:"lat"`
	Lon               float64 `json:"lon"`
	DisplayName       *string `json:"display_name"`
	Address           *string `json:"address"`
	Type              *string `json:"place_type"`
	Source            *string `json:"source"`
	Raw               string  `json:"nominatim_response"`
	First             int64   `json:"first_cached_at"`
	Last              int64   `json:"last_accessed_at"`
	Count             *int64  `json:"access_count"`
}

const cacheColumns = "cache_key,lat,lon,display_name,address,place_type,source,nominatim_response,first_cached_at,last_accessed_at,access_count"
const cacheReadColumns = cacheColumns + ",upstream_archive_id"

type scanner interface{ Scan(...any) error }

func scanCache(row scanner) (Cache, error) {
	var c Cache
	err := row.Scan(&c.Key, &c.Lat, &c.Lon, &c.DisplayName, &c.Address, &c.Type, &c.Source, &c.Raw, &c.First, &c.Last, &c.Count, &c.UpstreamArchiveID)
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
	return scanCache(s.DB.QueryRowContext(ctx, "SELECT "+cacheReadColumns+" FROM cache_entries WHERE cache_key=?", key))
}

func (s Store) Touch(ctx context.Context, key string, now int64) error {
	_, err := s.DB.ExecContext(ctx, "UPDATE cache_entries SET last_accessed_at=?,access_count=COALESCE(access_count,0)+1 WHERE cache_key=?", now, key)
	return err
}

type execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func insertCache(ctx context.Context, tx execer, point geocode.Point, key string, r geocode.Response, now, archiveID int64) error {
	raw, err := json.Marshal(r)
	if err != nil {
		return err
	}
	address, err := json.Marshal(r.Address())
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO cache_entries ("+cacheReadColumns+") VALUES (?,?,?,?,?,?,?,?,?,?,1,NULLIF(?,0)) "+
		"ON DUPLICATE KEY UPDATE cache_key=VALUES(cache_key)", key, point.Lat, point.Lon,
		r.Text("display_name"), string(address), geocode.PlaceType(r), r.Text("_source"), string(raw), now, now, archiveID)
	return err
}

func (s Store) InsertCache(ctx context.Context, point geocode.Point, key string, r geocode.Response, archiveID int64) error {
	return insertCache(ctx, s.DB, point, key, r, time.Now().UnixMilli(), archiveID)
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
		_, err = tx.ExecContext(ctx, "UPDATE cache_entries SET lat=?,lon=?,display_name=?,address=?,source='manual',nominatim_response=?,first_cached_at=?,upstream_archive_id=NULL WHERE cache_key=?",
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
	rows, err := s.DB.QueryContext(ctx, "SELECT "+cacheReadColumns+" FROM cache_entries"+where+
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

func (s Store) RecordAccess(ctx context.Context, p geocode.Point, key string, status int, output string, upstream geocode.UpstreamPayload, cached *Cache) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	hit := cached != nil
	now := time.Now().UnixMilli()
	log, err := tx.ExecContext(ctx, "INSERT INTO access_logs (cache_key,lat,lon,cached,accessed_at) VALUES (?,?,?,?,?)",
		key, p.Lat, p.Lon, hit, now)
	if err != nil {
		return err
	}
	id, err := log.LastInsertId()
	if err != nil {
		return err
	}
	var originalID *int64
	var originalSource *string
	var originalBody any
	if hit {
		originalID = cached.UpstreamArchiveID
		if originalID == nil {
			originalSource, originalBody = cached.Source, cached.Raw
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO access_log_responses
		(access_log_id,upstream_source,upstream_body,response_status,response_body,upstream_archived,
		 cache_original_archive_id,cache_original_source,cache_original_body)
		VALUES (?,NULLIF(?,''),?,?,?,?,?,?,?)`, id, upstream.Source, upstream.Body, status, output,
		upstream.ArchiveID > 0, originalID, originalSource, originalBody); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE stats SET total_requests=total_requests+1, cache_hits=cache_hits+?, cache_misses=cache_misses+?, last_updated_at=? WHERE id=1",
		hit, !hit && status == http.StatusOK, now); err != nil {
		return err
	}
	return tx.Commit()
}

type AccessLog struct {
	ID     int64    `json:"id"`
	Key    *string  `json:"cache_key"`
	Lat    *float64 `json:"lat"`
	Lon    *float64 `json:"lon"`
	Hit    int      `json:"cached"`
	Source *string  `json:"source"`
	At     int64    `json:"accessed_at"`
	Label  string   `json:"accessed_at_str"`
}

type AccessLogDetail struct {
	UpstreamSource      *string `json:"upstream_source"`
	UpstreamBody        *string `json:"upstream_body"`
	ResponseStatus      *int    `json:"response_status"`
	ResponseBody        *string `json:"response_body"`
	CacheOriginalSource *string `json:"cache_original_source"`
	CacheOriginalBody   *string `json:"cache_original_body"`
	CacheOriginalKind   *string `json:"cache_original_kind"`
}

func (s Store) LogDetail(ctx context.Context, id int64) (AccessLogDetail, error) {
	var result AccessLogDetail
	err := s.DB.QueryRowContext(ctx, `SELECT r.upstream_source,r.upstream_body,r.response_status,NULLIF(r.response_body,''),
		COALESCE(a.source,r.cache_original_source),COALESCE(a.response_body,r.cache_original_body),
		CASE WHEN a.response_body IS NOT NULL THEN 'upstream' WHEN r.cache_original_body IS NOT NULL THEN 'cache_snapshot' END
		FROM access_logs l LEFT JOIN access_log_responses r ON r.access_log_id=l.id
		LEFT JOIN upstream_archive a ON a.id=r.cache_original_archive_id WHERE l.id=?`, id).
		Scan(&result.UpstreamSource, &result.UpstreamBody, &result.ResponseStatus, &result.ResponseBody,
			&result.CacheOriginalSource, &result.CacheOriginalBody, &result.CacheOriginalKind)
	return result, err
}

func (s Store) Logs(ctx context.Context, hit *int, zone *time.Location, limit, offset int) ([]AccessLog, int64, error) {
	where := ""
	args := []any{}
	if hit != nil {
		where, args = " WHERE l.cached=?", []any{*hit}
	}
	var total int64
	if err := s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM access_logs l"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT l.id,l.cache_key,l.lat,l.lon,l.cached,l.accessed_at,
		CASE WHEN l.cached=1 THEN COALESCE(a.source,NULLIF(r.cache_original_source,'')) ELSE r.upstream_source END
		FROM access_logs l LEFT JOIN access_log_responses r ON r.access_log_id=l.id
		LEFT JOIN upstream_archive a ON a.id=r.cache_original_archive_id`+where+
		" ORDER BY l.accessed_at DESC,l.id DESC LIMIT ? OFFSET ?", append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result := make([]AccessLog, 0)
	for rows.Next() {
		var item AccessLog
		if err := rows.Scan(&item.ID, &item.Key, &item.Lat, &item.Lon, &item.Hit, &item.At, &item.Source); err != nil {
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

func (s Store) Daily(ctx context.Context, now time.Time, days int) ([]Day, error) {
	y, m, d := now.Date()
	start := time.Date(y, m, d, 0, 0, 0, 0, now.Location()).AddDate(0, 0, 1-days)
	result := make([]Day, 0, days)
	for range days {
		end := start.AddDate(0, 0, 1)
		item := Day{Date: start.Format(time.DateOnly), Label: fmt.Sprintf("%d/%d", start.Month(), start.Day())}
		err := s.DB.QueryRowContext(ctx, `SELECT
			COUNT(*) + COALESCE((SELECT total FROM access_daily_rollups WHERE day=?),0),
			COALESCE(SUM(cached=1),0) + COALESCE((SELECT hits FROM access_daily_rollups WHERE day=?),0)
			FROM access_logs WHERE accessed_at>=? AND accessed_at<?`,
			item.Date, item.Date, start.UnixMilli(), end.UnixMilli()).Scan(&item.Total, &item.Hits)
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
	for {
		count, err := s.rollUpAccessLogs(ctx, now.Add(-logRetention))
		if err != nil {
			return err
		}
		if count == 0 {
			break
		}
	}
	for {
		result, err := s.DB.ExecContext(ctx, "DELETE FROM batch_jobs WHERE finished_at<? ORDER BY finished_at LIMIT 50", now.Add(-batchRetention).UnixMilli())
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil || count == 0 {
			return err
		}
	}
}
