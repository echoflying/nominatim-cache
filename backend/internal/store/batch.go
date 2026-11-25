package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/echoflying/nominatim-cache/backend/internal/geocode"
)

var ErrBatchConflict = errors.New("同一个 Idempotency-Key 已用于其他坐标")
var ErrBatchQuota = errors.New("待处理坐标数超过上限")
var ErrLeaseLost = errors.New("任务租约已失效")

const batchSourceError = "batch_source_unavailable"

func randomID() string {
	var data [16]byte
	rand.Read(data[:])
	return hex.EncodeToString(data[:])
}

type Batch struct {
	ID         string `json:"job_id"`
	Status     string `json:"status"`
	CreatedAt  int64  `json:"created_at"`
	FinishedAt *int64 `json:"finished_at,omitempty"`
	Total      int    `json:"total"`
	Pending    int    `json:"pending"`
	Completed  int    `json:"completed"`
	Failed     int    `json:"failed"`
}

func (s Store) SubmitBatch(ctx context.Context, caller, requestKey, hash string, points []geocode.Point, pendingLimit int, hasSource bool) (string, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	// 同一个调用方的幂等检查与额度计算必须串行。
	if _, err := tx.ExecContext(ctx, "INSERT INTO batch_callers (caller) VALUES (?) ON DUPLICATE KEY UPDATE caller=VALUES(caller)", caller); err != nil {
		return "", err
	}
	var id, priorHash string
	err = tx.QueryRowContext(ctx, "SELECT id,input_hash FROM batch_jobs WHERE caller=? AND request_key=?", caller, requestKey).Scan(&id, &priorHash)
	switch {
	case err == nil && hash == priorHash:
		return id, nil
	case err == nil:
		return "", ErrBatchConflict
	case !errors.Is(err, sql.ErrNoRows):
		return "", err
	}

	byKey := make(map[string]geocode.Point)
	keys := make([]string, len(points))
	for i, p := range points {
		key := p.Key()
		keys[i] = key
		if _, exists := byKey[key]; !exists {
			byKey[key] = p
		}
	}
	cached, err := batchCache(ctx, tx, byKey)
	if err != nil {
		return "", err
	}

	var pending int
	if hasSource {
		for _, key := range keys {
			if cached[key] == nil {
				pending++
			}
		}
		if pending > 0 {
			var existing int
			if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM batch_jobs j JOIN batch_items i ON i.job_id=j.id WHERE j.caller=? AND j.finished_at IS NULL AND i.status='pending'", caller).Scan(&existing); err != nil {
				return "", err
			}
			if existing+pending > pendingLimit {
				return "", ErrBatchQuota
			}
		}
	}
	now := time.Now().UnixMilli()
	var finished *int64
	if pending == 0 {
		finished = &now
	}
	id = randomID()
	if _, err := tx.ExecContext(ctx, "INSERT INTO batch_jobs (id,caller,request_key,input_hash,created_at,finished_at) VALUES (?,?,?,?,?,?)",
		id, caller, requestKey, hash, now, finished); err != nil {
		return "", err
	}
	if pending > 0 {
		ordered := make([]string, 0, len(byKey))
		for key := range byKey {
			if cached[key] == nil {
				ordered = append(ordered, key)
			}
		}
		slices.Sort(ordered)
		if _, err := tx.ExecContext(ctx, "INSERT INTO geocode_tasks (cache_key,lat,lon,next_at) VALUES "+
			strings.Join(slices.Repeat([]string{"(?,?,?,?)"}, len(ordered)), ",")+" ON DUPLICATE KEY UPDATE cache_key=VALUES(cache_key)",
			taskArgs(ordered, byKey, now)...); err != nil {
			return "", err
		}
	}
	args := make([]any, 0, 8*len(points))
	for index, point := range points {
		key := keys[index]
		var raw, failure any
		status := "pending"
		if body := cached[key]; body != nil {
			status, raw = "cached", string(body)
		} else if !hasSource {
			status, failure = "failed", batchSourceError
		}
		args = append(args, id, index, point.Lat, point.Lon, key, status, raw, failure)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO batch_items (job_id,ordinal,lat,lon,cache_key,status,result,last_error) VALUES "+
		strings.Join(slices.Repeat([]string{"(?,?,?,?,?,?,?,?)"}, len(points)), ","), args...); err != nil {
		return "", err
	}
	return id, tx.Commit()
}

func taskArgs(keys []string, points map[string]geocode.Point, now int64) []any {
	args := make([]any, 0, 4*len(keys))
	for _, key := range keys {
		p := points[key]
		args = append(args, key, p.Lat, p.Lon, now)
	}
	return args
}

func batchCache(ctx context.Context, tx *sql.Tx, points map[string]geocode.Point) (map[string]json.RawMessage, error) {
	args := make([]any, 0, len(points))
	for key := range points {
		args = append(args, key)
	}
	rows, err := tx.QueryContext(ctx, "SELECT "+cacheColumns+" FROM cache_entries WHERE cache_key IN ("+
		strings.Join(slices.Repeat([]string{"?"}, len(args)), ",")+")", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := make(map[string]json.RawMessage)
	for rows.Next() {
		item, err := scanCache(rows)
		if err != nil {
			return nil, err
		}
		public, err := item.Public()
		if err != nil {
			continue
		}
		raw, err := json.Marshal(public)
		if err != nil {
			return nil, err
		}
		results[item.Key] = raw
	}
	return results, rows.Err()
}

func (s Store) Batch(ctx context.Context, caller, id string) (Batch, error) {
	now := time.Now().UnixMilli()
	const query = `SELECT j.id,j.created_at,j.finished_at,COUNT(*),
		COALESCE(SUM(i.status='pending'),0),COALESCE(SUM(i.status IN ('cached','resolved')),0),
		COALESCE(SUM(i.status IN ('no_result','failed')),0),
		COALESCE(SUM(i.status='pending' AND (COALESCE(t.attempts,0)>0 OR COALESCE(t.lease_until,0)>?)),0)
		FROM batch_jobs j JOIN batch_items i ON i.job_id=j.id
		LEFT JOIN geocode_tasks t ON t.cache_key=i.cache_key AND i.status='pending'
		WHERE j.id=? AND j.caller=? GROUP BY j.id,j.created_at,j.finished_at`
	var b Batch
	var started int
	err := s.DB.QueryRowContext(ctx, query, now, id, caller).
		Scan(&b.ID, &b.CreatedAt, &b.FinishedAt, &b.Total, &b.Pending, &b.Completed, &b.Failed, &started)
	switch {
	case b.Pending == 0 && b.Failed > 0:
		b.Status = "completed_with_errors"
	case b.Pending == 0:
		b.Status = "completed"
	case started > 0 || b.Completed > 0 || b.Failed > 0:
		b.Status = "running"
	default:
		b.Status = "pending"
	}
	return b, err
}

type BatchItem struct {
	Index   int             `json:"index"`
	Lat     float64         `json:"lat"`
	Lon     float64         `json:"lon"`
	Status  string          `json:"status"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *string         `json:"error,omitempty"`
	RetryAt *int64          `json:"retry_at,omitempty"`
}

func (s Store) BatchItems(ctx context.Context, caller, id string, limit, offset int) ([]BatchItem, error) {
	const query = `SELECT i.ordinal,i.lat,i.lon,i.status,i.result,i.last_error,t.last_error,t.next_at,t.lease_until
		FROM batch_jobs j JOIN batch_items i ON i.job_id=j.id
		LEFT JOIN geocode_tasks t ON t.cache_key=i.cache_key AND i.status='pending'
		WHERE j.id=? AND j.caller=? ORDER BY i.ordinal LIMIT ? OFFSET ?`
	rows, err := s.DB.QueryContext(ctx, query, id, caller, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]BatchItem, 0)
	for rows.Next() {
		var i BatchItem
		var raw []byte
		var taskError *string
		var next, lease *int64
		if err := rows.Scan(&i.Index, &i.Lat, &i.Lon, &i.Status, &raw, &i.Error, &taskError, &next, &lease); err != nil {
			return nil, err
		}
		i.Result = raw
		if i.Status == "pending" {
			now := time.Now().UnixMilli()
			if lease != nil && *lease > now {
				i.Status = "running"
			} else if taskError != nil {
				i.Error, i.RetryAt = taskError, next
			}
		}
		result = append(result, i)
	}
	return result, rows.Err()
}

type Task struct {
	Key      string
	Point    geocode.Point
	Attempts int
	Token    string
}

func (s Store) Claim(ctx context.Context, lease time.Duration) (Task, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return Task{}, err
	}
	defer tx.Rollback()
	var task Task
	now := time.Now()
	err = tx.QueryRowContext(ctx, "SELECT cache_key,lat,lon,attempts FROM geocode_tasks WHERE next_at<=? AND (lease_until IS NULL OR lease_until<=?) ORDER BY next_at,cache_key LIMIT 1 FOR UPDATE",
		now.UnixMilli(), now.UnixMilli()).Scan(&task.Key, &task.Point.Lat, &task.Point.Lon, &task.Attempts)
	if err != nil {
		return Task{}, err
	}
	task.Attempts++
	task.Token = randomID()
	if _, err := tx.ExecContext(ctx, "UPDATE geocode_tasks SET lease_token=?,lease_until=?,attempts=? WHERE cache_key=?",
		task.Token, now.Add(lease).UnixMilli(), task.Attempts, task.Key); err != nil {
		return Task{}, err
	}
	return task, tx.Commit()
}

func (s Store) Retry(ctx context.Context, task Task, next time.Time, reason string) error {
	result, err := s.DB.ExecContext(ctx, "UPDATE geocode_tasks SET lease_token=NULL,lease_until=NULL,next_at=?,last_error=? WHERE cache_key=? AND lease_token=? AND lease_until>?",
		next.UnixMilli(), reason, task.Key, task.Token, time.Now().UnixMilli())
	return owned(result, err)
}

func (s Store) Release(ctx context.Context, task Task) error {
	result, err := s.DB.ExecContext(ctx, "UPDATE geocode_tasks SET lease_token=NULL,lease_until=NULL,attempts=attempts-1 WHERE cache_key=? AND lease_token=? AND lease_until>?",
		task.Key, task.Token, time.Now().UnixMilli())
	return owned(result, err)
}

func owned(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count != 1 {
		return ErrLeaseLost
	}
	return err
}

// Finish resolves every batch item waiting on the key. Fresh upstream results
// and task completion are committed in the same transaction.
func (s Store) Finish(ctx context.Context, task Task, status, reason string, response geocode.Response) error {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UnixMilli()
	result, err := tx.ExecContext(ctx, "DELETE FROM geocode_tasks WHERE cache_key=? AND lease_token=? AND lease_until>?", task.Key, task.Token, now)
	if err := owned(result, err); err != nil {
		return err
	}
	if status == "resolved" {
		if err := insertCache(ctx, tx, task.Point, task.Key, response, now); err != nil {
			return err
		}
	}
	var raw, lastError any
	if response != nil {
		data, err := json.Marshal(geocode.WithPlace(response))
		if err != nil {
			return err
		}
		raw = string(data)
	}
	if reason != "" {
		lastError = reason
	}
	if _, err := tx.ExecContext(ctx, "UPDATE batch_items SET status=?,result=?,last_error=? WHERE cache_key=? AND status='pending'",
		status, raw, lastError, task.Key); err != nil {
		return err
	}
	const completed = `UPDATE batch_jobs j
		SET j.finished_at=?
		WHERE j.finished_at IS NULL
		AND EXISTS (SELECT 1 FROM batch_items i WHERE i.cache_key=? AND i.job_id=j.id)
		AND NOT EXISTS (SELECT 1 FROM batch_items pending WHERE pending.job_id=j.id AND pending.status='pending')`
	if _, err := tx.ExecContext(ctx, completed, now, task.Key); err != nil {
		return fmt.Errorf("标记批次完成: %w", err)
	}
	return tx.Commit()
}
