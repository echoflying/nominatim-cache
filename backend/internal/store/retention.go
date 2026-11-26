package store

import (
	"context"
	"strings"
	"time"

	"github.com/echoflying/nominatim-cache/backend/internal/geocode"
)

func (s Store) ArchiveUpstream(ctx context.Context, a geocode.UpstreamAttempt) (int64, error) {
	result, err := s.DB.ExecContext(ctx, `INSERT INTO upstream_archive
		(accessed_at,origin,source,lat,lon,upstream_status,response_body,incomplete,error_message)
		VALUES (?,?,?,?,?,NULLIF(?,0),?,?,NULLIF(?,''))`,
		a.StartedAt.UnixMilli(), a.Origin, a.Source, a.Point.Lat, a.Point.Lon,
		a.Status, a.Body, a.Incomplete, a.Error)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// Aggregate and remove the same set of rows atomically, including when maintenance resumes after a restart.
func (s Store) rollUpAccessLogs(ctx context.Context, cutoff time.Time) (int, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `SELECT id,accessed_at,cached FROM access_logs
		WHERE accessed_at<? ORDER BY accessed_at,id LIMIT 1000 FOR UPDATE`, cutoff.UnixMilli())
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	ids := make([]any, 0, 1000)
	var days []Day
	for rows.Next() {
		var id, at int64
		var cached int
		if err := rows.Scan(&id, &at, &cached); err != nil {
			return 0, err
		}
		ids = append(ids, id)
		date := time.UnixMilli(at).In(cutoff.Location()).Format(time.DateOnly)
		if len(days) == 0 || days[len(days)-1].Date != date {
			days = append(days, Day{Date: date})
		}
		day := &days[len(days)-1]
		day.Total++
		if cached == 1 {
			day.Hits++
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	for _, day := range days {
		if _, err := tx.ExecContext(ctx, `INSERT INTO access_daily_rollups (day,total,hits) VALUES (?,?,?)
			ON DUPLICATE KEY UPDATE total=total+VALUES(total), hits=hits+VALUES(hits)`, day.Date, day.Total, day.Hits); err != nil {
			return 0, err
		}
	}
	placeholders := strings.Repeat("?,", len(ids)-1) + "?"
	// Cover records from a previous binary and successful lookups whose first archive write failed.
	if _, err := tx.ExecContext(ctx, `INSERT INTO upstream_archive (accessed_at,origin,source,lat,lon,response_body,prior_log_id)
		SELECT l.accessed_at,'reverse',r.upstream_source,l.lat,l.lon,r.upstream_body,l.id
		FROM access_log_responses r JOIN access_logs l ON l.id=r.access_log_id
		WHERE l.id IN (`+placeholders+`) AND NOT r.upstream_archived AND r.upstream_source IS NOT NULL AND r.upstream_body IS NOT NULL
		ON DUPLICATE KEY UPDATE prior_log_id=VALUES(prior_log_id)`, ids...); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM access_logs WHERE id IN ("+placeholders+")", ids...); err != nil {
		return 0, err
	}
	return len(ids), tx.Commit()
}
