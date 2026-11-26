package geocode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var ErrNoResult = errors.New("no_result")
var ErrBatchSourceUnavailable = errors.New("batch_source_unavailable")

type SourceConfig struct {
	Name   string
	URL    string
	Photon bool
}

type source struct {
	config SourceConfig
	url    *url.URL
	gate   *upstreamGate
}

type Upstream struct {
	client   *http.Client
	sources  []source
	batch    *source
	interval time.Duration
	sequence atomic.Uint64
	Archive  func(context.Context, UpstreamAttempt) (int64, error)
}

func NewUpstream(configs []SourceConfig, batch *SourceConfig, interval, timeout time.Duration) (*Upstream, error) {
	u := &Upstream{interval: interval, client: &http.Client{
		Timeout: timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
	gates := make(map[string]*upstreamGate)
	build := func(cfg SourceConfig) (source, error) {
		endpoint, err := url.Parse(cfg.URL)
		if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Hostname() == "" || endpoint.User != nil || endpoint.Fragment != "" {
			return source{}, fmt.Errorf("invalid URL for upstream %s", cfg.Name)
		}
		host := strings.TrimSuffix(strings.ToLower(endpoint.Hostname()), ".")
		gate := gates[host]
		if gate == nil {
			gate = &upstreamGate{}
			gates[host] = gate
		}
		return source{config: cfg, url: endpoint, gate: gate}, nil
	}
	for _, cfg := range configs {
		s, err := build(cfg)
		if err != nil {
			return nil, err
		}
		u.sources = append(u.sources, s)
	}
	if batch != nil {
		s, err := build(*batch)
		if err != nil {
			return nil, err
		}
		host := strings.TrimSuffix(strings.ToLower(s.url.Hostname()), ".")
		switch host {
		case "nominatim.openstreetmap.org", "photon.komoot.io", "photon.komoot.de":
			return nil, errors.New("public geocoding endpoints cannot be configured as the batch source")
		}
		u.batch = &s
	}
	return u, nil
}

func (u *Upstream) HasBatchSource() bool { return u.batch != nil }

func (u *Upstream) Batch(ctx context.Context, point Point, priorities ...RequestPriority) (Response, UpstreamPayload, error) {
	var payload UpstreamPayload
	if u.batch == nil {
		return nil, payload, ErrBatchSourceUnavailable
	}
	priority := PriorityBatch
	if len(priorities) > 0 {
		priority = priorities[0]
	}
	response, err := u.request(ctx, *u.batch, point, priority, &payload, "batch")
	return response, payload, err
}

type UpstreamPayload struct {
	Source    string
	Body      []byte
	ArchiveID int64
}

type UpstreamAttempt struct {
	Source     string
	Origin     string
	Point      Point
	StartedAt  time.Time
	Status     int
	Body       []byte
	Incomplete bool
	Error      string
}

func (u *Upstream) Reverse(ctx context.Context, point Point) (Response, UpstreamPayload, error) {
	var payload UpstreamPayload
	const prefix = "所有上游 Nominatim 服务均不可用"
	if len(u.sources) == 0 {
		return nil, payload, errors.New(prefix + ": 未配置上游")
	}
	first := int((u.sequence.Add(1) - 1) % uint64(len(u.sources)))
	var failures []string
	for offset := range u.sources {
		src := u.sources[(first+offset)%len(u.sources)]
		response, err := u.reverseAttempt(ctx, src, point, len(u.sources)-offset, &payload)
		if err == nil {
			return response, payload, nil
		}
		failures = append(failures, src.config.Name+": "+err.Error())
		if ctx.Err() != nil {
			break
		}
	}
	if len(failures) == 0 && ctx.Err() != nil {
		failures = append(failures, ctx.Err().Error())
	}
	return nil, payload, fmt.Errorf("%s: %s", prefix, strings.Join(failures, "; "))
}

func (u *Upstream) reverseAttempt(ctx context.Context, src source, point Point, remaining int, payload *UpstreamPayload) (Response, error) {
	if deadline, ok := ctx.Deadline(); ok {
		// 为后续源保留时间。单个尝试也受 HTTP 客户端的上游超时限制。
		attempt, cancel := context.WithTimeout(ctx, time.Until(deadline)/time.Duration(remaining))
		defer cancel()
		return u.request(attempt, src, point, PriorityInteractive, payload, "reverse")
	}
	return u.request(ctx, src, point, PriorityInteractive, payload, "reverse")
}

type Comparison struct {
	Source      string         `json:"source"`
	Success     bool           `json:"success"`
	DisplayName string         `json:"display_name"`
	Address     map[string]any `json:"address"`
}

func (u *Upstream) Compare(ctx context.Context, point Point) []Comparison {
	results := make([]Comparison, 0, len(u.sources))
	for _, src := range u.sources {
		response, err := u.request(ctx, src, point, PriorityInteractive, nil, "compare")
		result := Comparison{Source: src.config.Name, Address: map[string]any{}}
		if err != nil {
			result.DisplayName = "失败: " + err.Error()
		} else {
			result.Success, result.DisplayName, result.Address = true, response.Text("display_name"), response.Address()
		}
		results = append(results, result)
	}
	return results
}

type RetryLater struct{ At time.Time }

func (e *RetryLater) Error() string { return "上游暂不可用" }

func (u *Upstream) request(ctx context.Context, src source, point Point, priority RequestPriority, payload *UpstreamPayload, origin string) (_ Response, resultErr error) {
	var recorded *UpstreamAttempt
	defer func() {
		if recorded == nil {
			return
		}
		if resultErr != nil {
			recorded.Error = resultErr.Error()
		}
		recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()
		if id, err := u.Archive(recordCtx, *recorded); err != nil {
			slog.Error("保存上游查询归档失败", "source", recorded.Source, "origin", origin, "error", err)
		} else if payload != nil && resultErr == nil {
			payload.ArchiveID = id
		}
	}()
	g := src.gate
	if err := g.acquire(ctx, priority, u.interval); err != nil {
		return nil, err
	}
	defer g.release(u.interval)
	if time.Now().Before(g.until) {
		return nil, &RetryLater{At: g.until}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	endpoint := *src.url
	query := endpoint.Query()
	query.Set("lat", strconv.FormatFloat(point.Lat, 'f', -1, 64))
	query.Set("lon", strconv.FormatFloat(point.Lon, 'f', -1, 64))
	if !src.config.Photon {
		query.Set("format", "jsonv2")
		query.Set("addressdetails", "1")
		query.Set("extratags", "1")
		query.Set("accept-language", "zh-CN")
		query.Set("zoom", "18")
	}
	endpoint.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, errors.New("无法创建上游请求")
	}
	req.Header.Set("User-Agent", "PhotoOrdo-Local/1.0 (echoflying@gmail.com)")
	req.Header.Set("Referer", "http://localhost:3000/")
	req.Header.Set("Accept-Language", "zh-CN,zh")
	if u.Archive != nil {
		recorded = &UpstreamAttempt{Source: src.config.Name, Origin: origin, Point: point, StartedAt: time.Now()}
	}
	response, requestErr := u.client.Do(req)
	var result Response
	var rawBody []byte
	var retryAt time.Time
	switch {
	case requestErr != nil:
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		err = errors.New("上游连接失败或超时")
	default:
		defer response.Body.Close()
		const maxBody = 2 << 20
		var readErr error
		if recorded != nil || response.StatusCode >= 200 && response.StatusCode < 300 {
			rawBody, readErr = io.ReadAll(io.LimitReader(response.Body, maxBody+1))
		} else if response.StatusCode == http.StatusNotFound {
			rawBody, readErr = io.ReadAll(io.LimitReader(response.Body, 16<<10))
		}
		if recorded != nil {
			recorded.Status = response.StatusCode
			recorded.Body = rawBody[:min(len(rawBody), maxBody)]
			recorded.Incomplete = readErr != nil || len(rawBody) > maxBody
		}
		switch {
		case response.StatusCode == http.StatusNotFound:
			err = errors.New("HTTP 404")
			// 反代未配置路径时也会返回 404。只有明确的上游无结果信号才结束任务。
			if readErr == nil && len(rawBody) <= 16<<10 {
				if _, parseErr := parseUpstream(rawBody, point); errors.Is(parseErr, ErrNoResult) {
					err = ErrNoResult
				}
			}
		case response.StatusCode < 200 || response.StatusCode >= 300:
			err = fmt.Errorf("HTTP %d", response.StatusCode)
			if response.StatusCode == http.StatusTooManyRequests || response.StatusCode == http.StatusServiceUnavailable {
				retryAt = parseRetryAfter(response.Header.Get("Retry-After"), time.Now())
				if response.StatusCode == http.StatusTooManyRequests && retryAt.IsZero() {
					retryAt = time.Now().Add(time.Minute)
				}
			}
		default:
			if readErr != nil || len(rawBody) > maxBody {
				err = errors.New("无法读取上游响应")
			} else {
				result, err = parseUpstream(rawBody, point)
			}
		}
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err == nil || errors.Is(err, ErrNoResult) {
		g.fails, g.backoff, g.until = 0, 0, time.Time{}
		if result != nil {
			result["_source"] = src.config.Name
			if payload != nil {
				*payload = UpstreamPayload{Source: src.config.Name, Body: rawBody}
			}
		}
		return result, err
	}

	g.fails++
	if g.fails >= 3 {
		g.fails = 0
		g.backoff = max(4*time.Minute, min(g.backoff*2, 256*time.Minute))
		g.until = time.Now().Add(g.backoff)
	}
	if retryAt.After(g.until) {
		g.until = retryAt
	}
	slog.Warn("上游查询失败", "source", src.config.Name, "error", err)
	if time.Now().Before(g.until) {
		return nil, fmt.Errorf("%v: %w", err, &RetryLater{At: g.until})
	}
	return nil, err
}

func parseRetryAfter(header string, now time.Time) time.Time {
	if seconds, err := strconv.ParseInt(header, 10, 32); err == nil && seconds > 0 {
		return now.Add(time.Duration(seconds) * time.Second)
	}
	if date, err := http.ParseTime(header); err == nil {
		return date
	}
	return time.Time{}
}

func parseUpstream(raw []byte, point Point) (Response, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] == '<' {
		return nil, errors.New("上游返回空内容或 HTML/XML")
	}
	if raw[0] != '{' && raw[0] != '[' && raw[0] != '"' {
		if bytes.Equal(raw, []byte("null")) || bytes.Equal(raw, []byte("false")) {
			return nil, errors.New("无有效数据")
		}
		return textResponse(string(raw), point), nil
	}
	if raw[0] == '"' {
		var name string
		if err := json.Unmarshal(raw, &name); err != nil || strings.TrimSpace(name) == "" || strings.HasPrefix(strings.TrimSpace(name), "<") {
			return nil, errors.New("无有效数据")
		}
		return textResponse(strings.TrimSpace(name), point), nil
	}
	r, err := DecodeResponse(raw)
	if err != nil || r == nil {
		return nil, errors.New("上游返回无效 JSON")
	}
	if r.Text("type") == "FeatureCollection" {
		return convertPhoton(r, point)
	}
	if r["error"] != nil {
		if strings.EqualFold(strings.TrimSuffix(strings.TrimSpace(r.Text("error")), "."), "unable to geocode") {
			return nil, ErrNoResult
		}
		return nil, errors.New("上游返回错误")
	}
	if strings.TrimSpace(r.Text("display_name")) == "" {
		return nil, errors.New("无有效数据")
	}
	return r, nil
}

func textResponse(name string, point Point) Response {
	return Response{
		"lat": strconv.FormatFloat(point.Lat, 'f', -1, 64), "lon": strconv.FormatFloat(point.Lon, 'f', -1, 64),
		"display_name": name, "address": map[string]any{},
	}
}

func convertPhoton(r Response, point Point) (Response, error) {
	features, ok := r["features"].([]any)
	if !ok {
		return nil, errors.New("Photon 返回无效数据")
	}
	if len(features) == 0 {
		return nil, ErrNoResult
	}
	feature, _ := features[0].(map[string]any)
	props, _ := feature["properties"].(map[string]any)
	fields := []string{"country", "state", "county", "village", "town", "city", "suburb", "neighbourhood", "road", "street", "house_number", "name"}
	address := make(map[string]any)
	var parts []string
	for _, field := range fields {
		if value, ok := props[field]; ok && value != nil && fmt.Sprint(value) != "" {
			address[field] = value
			parts = append(parts, fmt.Sprint(value))
		}
	}
	name := strings.Join(parts, ", ")
	if name == "" {
		name = "未知位置"
	}
	result := textResponse(name, point)
	result["address"] = address
	for _, field := range []string{"type", "extratags"} {
		if value, ok := props[field]; ok {
			result[field] = value
		}
	}
	return result, nil
}

// KeyLocks coordinate foreground and batch lookups for the same cache key.
type KeyLocks struct {
	mu    sync.Mutex
	locks map[string]*keyLock
}

type keyLock struct {
	slot chan struct{}
	refs int
}

func (k *KeyLocks) Lock(ctx context.Context, key string) (func(), error) {
	k.mu.Lock()
	if k.locks == nil {
		k.locks = make(map[string]*keyLock)
	}
	l := k.locks[key]
	if l == nil {
		l = &keyLock{slot: make(chan struct{}, 1)}
		k.locks[key] = l
	}
	l.refs++
	k.mu.Unlock()
	release := func() {
		k.mu.Lock()
		l.refs--
		if l.refs == 0 {
			delete(k.locks, key)
		}
		k.mu.Unlock()
	}
	select {
	case l.slot <- struct{}{}:
		return func() { <-l.slot; release() }, nil
	case <-ctx.Done():
		release()
		return nil, ctx.Err()
	}
}
