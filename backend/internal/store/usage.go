package store

import (
	"context"
	"sort"
	"time"
)

type APIEvent struct {
	Day            string
	OccurredAt     int64
	CallerID       string
	Operation      string
	Platform       string
	Version        string
	Status         int
	DurationMS     int64
	Points         int
	CacheHits      int
	UpstreamPoints int
	RateLimited    int
}

func (s Store) RecordAPIEvent(ctx context.Context, event APIEvent) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO api_request_events
		(event_day,occurred_at,caller_id,operation,client_platform,client_version,http_status,duration_ms,point_count,cache_hits,upstream_points)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`, event.Day, event.OccurredAt, event.CallerID, event.Operation,
		event.Platform, event.Version, event.Status, event.DurationMS, event.Points, event.CacheHits, event.UpstreamPoints)
	return err
}

type UsageMetric struct {
	Name           string `json:"name"`
	Requests       int64  `json:"requests"`
	Points         int64  `json:"points"`
	CacheHits      int64  `json:"cache_hits"`
	CacheMisses    int64  `json:"cache_misses"`
	UpstreamPoints int64  `json:"upstream_points"`
	ClientErrors   int64  `json:"client_errors"`
	ServerErrors   int64  `json:"server_errors"`
	RateLimited    int64  `json:"rate_limited"`
	DurationAvgMS  int64  `json:"duration_avg_ms"`
	DurationMaxMS  int64  `json:"duration_max_ms"`
	durationSum    int64
}

type UsageReport struct {
	From        string        `json:"from"`
	To          string        `json:"to"`
	Summary     UsageMetric   `json:"summary"`
	ByCaller    []UsageMetric `json:"by_caller"`
	ByOperation []UsageMetric `json:"by_operation"`
	ByPlatform  []UsageMetric `json:"by_platform"`
	ByVersion   []UsageMetric `json:"by_version"`
	Daily       []UsageMetric `json:"daily"`
}

func (s Store) Usage(ctx context.Context, from, to time.Time) (UsageReport, error) {
	start, end := from.Format(time.DateOnly), to.Format(time.DateOnly)
	report := UsageReport{From: start, To: end}
	query := `SELECT event_day,caller_id,operation,client_platform,client_version,status_class,requests,rate_limited,points,cache_hits,upstream_points,duration_sum_ms,duration_max_ms FROM api_request_daily_rollups WHERE event_day BETWEEN ? AND ?
		UNION ALL
		SELECT event_day,caller_id,operation,client_platform,client_version,FLOOR(http_status/100),1,(http_status=429),point_count,cache_hits,upstream_points,duration_ms,duration_ms FROM api_request_events WHERE event_day BETWEEN ? AND ?`
	rows, err := s.DB.QueryContext(ctx, query, start, end, start, end)
	if err != nil {
		return report, err
	}
	defer rows.Close()
	byCaller, byOperation, byPlatform, byVersion, byDay := map[string]*UsageMetric{}, map[string]*UsageMetric{}, map[string]*UsageMetric{}, map[string]*UsageMetric{}, map[string]*UsageMetric{}
	for rows.Next() {
		var day, caller, operation, platform, version string
		var statusClass, requests, limited, points, hits, upstream, durationSum, durationMax int64
		if err := rows.Scan(&day, &caller, &operation, &platform, &version, &statusClass, &requests, &limited, &points, &hits, &upstream, &durationSum, &durationMax); err != nil {
			return report, err
		}
		item := UsageMetric{Name: caller, Requests: requests, Points: points, CacheHits: hits, CacheMisses: max(int64(0), points-hits), UpstreamPoints: upstream, durationSum: durationSum, DurationMaxMS: durationMax}
		item.RateLimited = limited
		if item.Name == "" {
			item.Name = "anonymous"
		}
		if statusClass == 4 {
			item.ClientErrors = requests
		}
		if statusClass >= 5 {
			item.ServerErrors = requests
		}
		addUsage(&report.Summary, item)
		addUsageGroup(byCaller, item.Name, item)
		addUsageGroup(byOperation, operation, item)
		addUsageGroup(byPlatform, platform, item)
		if version == "" {
			version = "unspecified"
		}
		addUsageGroup(byVersion, version, item)
		addUsageGroup(byDay, day, item)
	}
	if err := rows.Err(); err != nil {
		return report, err
	}
	for _, metric := range []*UsageMetric{&report.Summary} {
		finalizeUsage(metric)
	}
	report.ByCaller = sortedUsage(byCaller)
	report.ByOperation = sortedUsage(byOperation)
	report.ByPlatform = sortedUsage(byPlatform)
	report.ByVersion = sortedUsage(byVersion)
	report.Daily = sortedUsage(byDay)
	sort.Slice(report.Daily, func(i, j int) bool { return report.Daily[i].Name < report.Daily[j].Name })
	return report, nil
}

func addUsage(target *UsageMetric, value UsageMetric) {
	target.Requests += value.Requests
	target.Points += value.Points
	target.CacheHits += value.CacheHits
	target.CacheMisses += value.CacheMisses
	target.UpstreamPoints += value.UpstreamPoints
	target.ClientErrors += value.ClientErrors
	target.ServerErrors += value.ServerErrors
	target.RateLimited += value.RateLimited
	target.durationSum += value.durationSum
	target.DurationMaxMS = max(target.DurationMaxMS, value.DurationMaxMS)
}

func addUsageGroup(groups map[string]*UsageMetric, name string, value UsageMetric) {
	group := groups[name]
	if group == nil {
		group = &UsageMetric{Name: name}
		groups[name] = group
	}
	addUsage(group, value)
}

func finalizeUsage(item *UsageMetric) {
	if item.Requests > 0 {
		item.DurationAvgMS = item.durationSum / item.Requests
	}
	item.durationSum = 0
}

func sortedUsage(groups map[string]*UsageMetric) []UsageMetric {
	result := make([]UsageMetric, 0, len(groups))
	for _, item := range groups {
		finalizeUsage(item)
		result = append(result, *item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Requests == result[j].Requests {
			return result[i].Name < result[j].Name
		}
		return result[i].Requests > result[j].Requests
	})
	return result
}

func (s Store) RollupAPIEvents(ctx context.Context, before time.Time) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	day := before.Format(time.DateOnly)
	_, err = tx.ExecContext(ctx, `INSERT INTO api_request_daily_rollups
		(event_day,caller_id,operation,client_platform,client_version,status_class,requests,rate_limited,points,cache_hits,upstream_points,duration_sum_ms,duration_max_ms)
		SELECT event_day,caller_id,operation,client_platform,client_version,FLOOR(http_status/100),COUNT(*),SUM(http_status=429),SUM(point_count),SUM(cache_hits),SUM(upstream_points),SUM(duration_ms),MAX(duration_ms)
		FROM api_request_events WHERE event_day<? GROUP BY event_day,caller_id,operation,client_platform,client_version,FLOOR(http_status/100)
		ON DUPLICATE KEY UPDATE requests=requests+VALUES(requests),rate_limited=rate_limited+VALUES(rate_limited),points=points+VALUES(points),cache_hits=cache_hits+VALUES(cache_hits),upstream_points=upstream_points+VALUES(upstream_points),duration_sum_ms=duration_sum_ms+VALUES(duration_sum_ms),duration_max_ms=GREATEST(duration_max_ms,VALUES(duration_max_ms))`, day)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM api_request_events WHERE event_day<?", day); err != nil {
		return err
	}
	return tx.Commit()
}
