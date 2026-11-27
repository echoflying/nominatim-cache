package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/echoflying/nominatim-cache/backend/internal/store"
)

type usageCaptureKey struct{}

type usageCapture struct {
	caller         string
	operation      string
	platform       string
	version        string
	points         int
	cacheHits      int
	upstreamPoints int
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (w *statusRecorder) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusRecorder) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func (w *statusRecorder) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func requestOperation(method, path string) string {
	switch {
	case method == http.MethodGet && (path == "/reverse" || path == "/api/reverse"):
		return "single_reverse"
	case method == http.MethodPost && path == "/api/reverse/batches":
		return "batch_submit"
	case method == http.MethodGet && strings.HasSuffix(path, "/results") && strings.HasPrefix(path, "/api/reverse/batches/"):
		return "batch_results"
	case method == http.MethodGet && strings.HasPrefix(path, "/api/reverse/batches/"):
		return "batch_status"
	default:
		return ""
	}
}

func sanitizedHeader(value string, limit int) string {
	value = strings.TrimSpace(value)
	var out strings.Builder
	for _, r := range value {
		if out.Len() >= limit {
			break
		}
		if unicode.IsLetter(r) && r <= unicode.MaxASCII || unicode.IsDigit(r) && r <= unicode.MaxASCII || strings.ContainsRune("._-", r) {
			out.WriteRune(r)
		}
	}
	return out.String()
}

func captureFromRequest(r *http.Request) *usageCapture {
	capture, _ := r.Context().Value(usageCaptureKey{}).(*usageCapture)
	return capture
}

func setUsageCaller(r *http.Request, caller string) {
	if capture := captureFromRequest(r); capture != nil {
		capture.caller = caller
	}
}

func (s Server) recordUsage(r *http.Request, capture *usageCapture, status int, started time.Time) {
	if s.Store.DB == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), time.Second)
	defer cancel()
	location := s.Config.Location
	if location == nil {
		location = time.Local
	}
	now := time.Now().In(location)
	event := store.APIEvent{
		Day: now.Format(time.DateOnly), OccurredAt: now.UnixMilli(), CallerID: capture.caller,
		Operation: capture.operation, Platform: capture.platform, Version: capture.version,
		Status: status, DurationMS: time.Since(started).Milliseconds(), Points: capture.points,
		CacheHits: capture.cacheHits, UpstreamPoints: capture.upstreamPoints,
	}
	if err := s.Store.RecordAPIEvent(ctx, event); err != nil {
		slog.Warn("写入 API 用量统计失败", "error", err)
	}
}
