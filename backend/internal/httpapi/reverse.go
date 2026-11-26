package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/echoflying/nominatim-cache/backend/internal/geocode"
)

var errColdBusy = errors.New("Reverse lookup busy")

func (s Server) reverse(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	point, valid := coordinates(query.Get("lat"), query.Get("lon"))
	if !valid {
		fail(w, http.StatusBadRequest, "Invalid coordinates")
		return
	}
	key := point.Key()
	timeout, logTimeout := s.reverseBudgets(r.Host)
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	response, hit, upstream, err := s.lookup(ctx, point, key)
	status := http.StatusOK
	if err == nil {
		if response.Text("place_quality") == "coarse" {
			slog.Warn("地址质量过粗", "cached", hit)
		}
	} else if errors.Is(err, errColdBusy) {
		w.Header().Set("Retry-After", "2")
		fail(w, http.StatusServiceUnavailable, err.Error())
		return
	} else {
		status = http.StatusInternalServerError
		response = geocode.Response{"error": err.Error()}
	}
	var output bytes.Buffer
	if hit {
		writeJSON(w, status, response)
	} else {
		writeJSONTo(w, io.MultiWriter(&output, w), status, response)
	}
	recordCtx, stop := context.WithTimeout(context.WithoutCancel(r.Context()), logTimeout)
	defer stop()
	if logErr := s.Store.RecordAccess(recordCtx, point, key, hit, status, output.String(), upstream); logErr != nil {
		slog.Warn("记录反查统计失败", "error", logErr)
	}
}

func (s Server) reverseBudgets(authority string) (time.Duration, time.Duration) {
	host, _, err := net.SplitHostPort(authority)
	if err != nil {
		host = authority
	}
	if s.Config.LegacyHost != "" && strings.EqualFold(strings.TrimSuffix(host, "."), s.Config.LegacyHost) {
		return min(s.Config.ReverseTimeout, s.Config.LegacyTimeout), 500 * time.Millisecond
	}
	return s.Config.ReverseTimeout, 2 * time.Second
}

func (s Server) lookup(ctx context.Context, point geocode.Point, key string) (geocode.Response, bool, geocode.UpstreamPayload, error) {
	var upstream geocode.UpstreamPayload
	readCache := func() (geocode.Response, error) {
		cached, err := s.Store.Cache(ctx, key)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			slog.Error("读取地址缓存失败", "error", err)
			return nil, errors.New("Cache unavailable")
		}
		response, err := cached.Public()
		if err != nil {
			slog.Warn("已缓存的地址响应无法读取")
			return nil, err
		}
		if err := s.Store.Touch(ctx, key, time.Now().UnixMilli()); err != nil {
			slog.Warn("更新地址缓存访问计数失败", "error", err)
		}
		return response, nil
	}
	if response, err := readCache(); response != nil || err != nil {
		return response, response != nil, upstream, err
	}
	select {
	case s.cold <- struct{}{}:
		defer func() { <-s.cold }()
	default:
		return nil, false, upstream, errColdBusy
	}
	release, err := s.Locks.Lock(ctx, key)
	if err != nil {
		return nil, false, upstream, errors.New("所有上游 Nominatim 服务均不可用: 等待查询超时")
	}
	defer release()
	if response, err := readCache(); response != nil || err != nil {
		return response, response != nil, upstream, err
	}
	response, upstream, err := s.Upstream.Reverse(ctx, point)
	if err != nil {
		return nil, false, upstream, err
	}
	if err := s.Store.InsertCache(ctx, point, key, response); err != nil {
		slog.Warn("保存上游地址失败", "error", err)
	}
	return geocode.WithPlace(response), false, upstream, nil
}
