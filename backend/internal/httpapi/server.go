package httpapi

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/echoflying/nominatim-cache/backend/internal/config"
	"github.com/echoflying/nominatim-cache/backend/internal/geocode"
	"github.com/echoflying/nominatim-cache/backend/internal/store"
)

type Server struct {
	Config   config.Config
	Store    store.Store
	Upstream *geocode.Upstream
	Locks    *geocode.KeyLocks
	Wake     chan struct{}
	Started  time.Time
	Version  string
	Commit   string
	cold     chan struct{}
}

type jsonObject = map[string]any
type prefixKey struct{}

func (s Server) Handler() http.Handler {
	s.cold = make(chan struct{}, max(1, s.Config.ColdLimit))
	routes := http.NewServeMux()
	routes.HandleFunc("GET /health", s.health)
	routes.HandleFunc("GET /api/reverse", s.reverse)
	routes.HandleFunc("POST /api/reverse/batches", s.batchAuth(s.submitBatch))
	routes.HandleFunc("GET /api/reverse/batches/{id}", s.batchAuth(s.batchStatus))
	routes.HandleFunc("GET /api/reverse/batches/{id}/results", s.batchAuth(s.batchResults))

	mux := routes
	if prefix := s.Config.BasePath; prefix != "" {
		mux = http.NewServeMux()
		nested := http.StripPrefix(prefix, routes)
		mux.Handle(prefix+"/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nested.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), prefixKey{}, prefix)))
		}))
		mux.Handle("/", routes)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		securityHeaders(w)
		if r.URL.Path == "/api/reverse" || (s.Config.BasePath != "" && r.URL.Path == s.Config.BasePath+"/api/reverse") {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}

func (s Server) AdminHandler() http.Handler {
	routes := http.NewServeMux()
	routes.HandleFunc("GET /admin/cache/list", s.admin(s.listCache))
	routes.HandleFunc("GET /admin/cache/{key}", s.admin(s.detailCache))
	routes.HandleFunc("DELETE /admin/cache/{key}", s.admin(s.deleteCache))
	routes.HandleFunc("POST /admin/cache/clear", s.admin(s.clearCache))
	routes.HandleFunc("POST /admin/cache/add", s.admin(s.addCache))
	routes.HandleFunc("POST /admin/query", s.admin(s.compare))
	routes.HandleFunc("GET /admin/logs", s.admin(s.logs))
	routes.HandleFunc("GET /admin/stats", s.admin(s.stats))
	routes.HandleFunc("GET /admin/stats/daily", s.admin(s.daily))
	routes.HandleFunc("GET /{$}", s.asset("index.html"))
	routes.HandleFunc("GET /list.html", s.asset("list.html"))
	static := http.FileServer(http.Dir(s.Config.Frontend))
	routes.Handle("GET /css/", static)
	routes.Handle("GET /js/", static)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		securityHeaders(w)
		routes.ServeHTTP(w, r)
	})
}

func securityHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'self';base-uri 'self';frame-ancestors 'self';object-src 'none';style-src 'self' https: 'unsafe-inline';img-src 'self' data:")
}

func (s Server) asset(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(s.Config.Frontend, name))
	}
}

func (s Server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	ready := s.Store.DB.PingContext(ctx) == nil
	status, code := "ok", http.StatusOK
	if !ready {
		status, code = "unavailable", http.StatusServiceUnavailable
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, code, jsonObject{
		"status": status, "service": "nominatim-cache", "environment": s.Config.Environment,
		"timestamp": time.Now().UnixMilli(), "uptime": int64(time.Since(s.Started).Seconds()),
		"version": s.Version, "commit": s.Commit, "database_ready": ready,
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Debug("写入 API 响应失败", "error", err)
	}
}

func fail(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, jsonObject{"error": message})
}

func adminError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, jsonObject{"success": false, "error": message})
}

func adminOK(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, jsonObject{"success": true, "data": data})
}

func secureEqual(a, b string) bool {
	x, y := sha256.Sum256([]byte(a)), sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(x[:], y[:]) == 1
}

func (s Server) admin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		user, pass, ok := r.BasicAuth()
		userMatch, passMatch := secureEqual(user, s.Config.AdminUser), secureEqual(pass, s.Config.AdminPass)
		if ok && userMatch && passMatch {
			next(w, r)
			return
		}
		w.Header().Set("WWW-Authenticate", `Basic realm="Nominatim Cache Admin"`)
		message := "Authentication required"
		if ok {
			message = "Invalid credentials"
		}
		adminError(w, http.StatusUnauthorized, message)
	}
}

func decodeBody(w http.ResponseWriter, r *http.Request, data any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
	if err := decoder.Decode(data); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("trailing data")
	}
	return nil
}

func pagination(r *http.Request, defaultLimit, maxLimit int) (page int, limit int, offset int) {
	page, _ = strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ = strconv.Atoi(r.URL.Query().Get("limit"))
	page = min(max(page, 1), 1_000_000)
	if limit <= 0 {
		limit = defaultLimit
	}
	limit = min(limit, maxLimit)
	return page, limit, (page - 1) * limit
}

func coordinates(lat, lon string) (geocode.Point, bool) {
	a, errA := strconv.ParseFloat(strings.TrimSpace(lat), 64)
	b, errB := strconv.ParseFloat(strings.TrimSpace(lon), 64)
	p := geocode.Point{Lat: a, Lon: b}
	return p, errA == nil && errB == nil && p.Valid()
}

type pointInput struct {
	Lat any `json:"lat"`
	Lon any `json:"lon"`
}

func (p pointInput) parse() (geocode.Point, bool) {
	text := func(value any) string {
		switch v := value.(type) {
		case string:
			return v
		case json.Number:
			return v.String()
		default:
			return ""
		}
	}
	return coordinates(text(p.Lat), text(p.Lon))
}
