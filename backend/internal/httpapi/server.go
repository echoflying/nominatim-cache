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
	routes.HandleFunc("GET /help", s.help)
	routes.HandleFunc("GET /reverse", s.reverse)
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
		path := strings.TrimPrefix(r.URL.Path, s.Config.BasePath)
		capture := &usageCapture{
			operation: requestOperation(r.Method, path),
			platform:  sanitizedHeader(r.Header.Get("X-Client-Platform"), 24),
			version:   sanitizedHeader(r.Header.Get("X-Client-Version"), 32),
		}
		if capture.platform == "" {
			capture.platform = "unknown"
		}
		started := time.Now()
		if capture.operation != "" {
			r = r.WithContext(context.WithValue(r.Context(), usageCaptureKey{}, capture))
		}
		recorder := &statusRecorder{ResponseWriter: w}
		if capture.operation != "" {
			defer func() {
				status := recorder.status
				if status == 0 {
					status = http.StatusOK
				}
				s.recordUsage(r, capture, status, started)
			}()
		}
		w = recorder
		securityHeaders(w)
		switch path {
		case "/reverse", "/api/reverse", "/health", "/help":
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Expose-Headers", "Retry-After")
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, X-Client-Platform, X-Client-Version")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		case "/api/reverse/batches":
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Expose-Headers", "Location, Retry-After")
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key, X-Client-Platform, X-Client-Version")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		default:
			if strings.HasPrefix(path, "/api/reverse/batches/") {
				w.Header().Set("Access-Control-Allow-Origin", "*")
				if r.Method == http.MethodOptions {
					w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Client-Platform, X-Client-Version")
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}
		}
		mux.ServeHTTP(w, r)
	})
}

func (s Server) help(w http.ResponseWriter, r *http.Request) {
	base := s.Config.BasePath
	batchEnabled, _ := s.Store.BatchCredentialsEnabled(r.Context())
	batchLimit := s.Config.BatchLimit
	if batchLimit <= 0 {
		batchLimit = 500
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, jsonObject{
		"service":     "nominatim-cache",
		"description": "以经纬度反查地点。返回用于分组的短地名，也保留完整地址。",
		"endpoints": []jsonObject{
			{"method": "GET", "path": base + "/reverse", "description": "按坐标反查地名"},
			{"method": "POST", "path": base + "/api/reverse/batches", "description": "提交通用批量反查，需要 Bearer 凭据；最大数量见 batch.max_points_per_job"},
			{"method": "GET", "path": base + "/api/reverse/batches/{job_id}", "description": "查看调用方自己的批量任务状态，需要 Bearer 凭据"},
			{"method": "GET", "path": base + "/api/reverse/batches/{job_id}/results", "description": "分页或按游标读取批量结果，需要 Bearer 凭据"},
			{"method": "GET", "path": base + "/health", "description": "检查数据库就绪状态，并返回版本和提交"},
			{"method": "GET", "path": base + "/help", "description": "读取本站接口指南"},
		},
		"example": base + "/reverse?lat=30.28746&lon=120.16145",
		"parameters": map[string]string{
			"lat": "必填，WGS84 纬度，范围 [-90, 90]",
			"lon": "必填，WGS84 经度，范围 [-180, 180]",
		},
		"response_fields": map[string]string{
			"place_name":     "用于展示或分组的短地名。质量为 precise 或 regional 且非空时使用。",
			"place_quality":  "precise 精确、regional 区域、coarse 过粗、none 无有效名称。过粗或无名称仍可返回 HTTP 200。",
			"display_name":   "完整地址，可用于详情展示。",
			"address":        "上游提供的结构化地址。",
			"cached":         "命中服务端缓存时为 true。",
			"batch_priority": "单点反查最高；少于 10 个去重坐标的小批量其次；大批量使用基础优先级并随等待时间提升。优先级不改变上游限速。",
			"batch_results":  "结果接口支持按 page/limit 分页，或使用 cursor、limit=1..50、wait=0..10 秒增量读取新完成项。",
		},
		"batch": map[string]any{
			"enabled":                         batchEnabled,
			"authentication":                  "Authorization: Bearer <token>；在运维后台的调用方凭据页创建。",
			"credential_encryption":           "密钥以 SHA-256 摘要鉴权，并由服务器 AES-GCM 主密钥加密保存；管理员可在运维后台按需查看。",
			"credential_management":           "/ops/nominatim/credentials.html",
			"max_points_per_job":              batchLimit,
			"pending_points_limit_per_caller": s.Config.PendingLimit,
			"caller_id_format":                "小写字母开头，后续可含小写字母、数字、点、下划线和短横线；按调用方平台分别创建。没有终端 ID。",
			"submit":                          map[string]any{"method": "POST", "path": base + "/api/reverse/batches", "headers": map[string]string{"Authorization": "Bearer <token>", "Idempotency-Key": "1 至 128 字节；相同键和相同坐标内容复用任务"}, "body": jsonObject{"points": []jsonObject{{"lat": 30.28746, "lon": 120.16145}}}, "success_status": 202, "response": "返回 job_id、status_url 和 results_url；轮询 status_url 或读取 results_url。"},
			"status_path":                     base + "/api/reverse/batches/{job_id}",
			"results_path":                    base + "/api/reverse/batches/{job_id}/results?cursor=0&limit=50&wait=8",
			"poll_interval_seconds":           2,
			"small_batch_threshold":           "去重后少于 10 个坐标",
			"priority":                        "单点反查优先级最高；小批量其次；大批量会随等待时间逐步提升。该顺序不会提高上游请求频率。",
			"result_polling":                  "结果接口兼容 page/limit 分页；增量方式使用 cursor=0、limit=50、wait=8，并在后续请求中传入 next_cursor。每项结果只返回一次，任务保留 7 天。状态为 pending、running、completed 或 completed_with_errors；结果项为 cached、resolved、pending、running、no_result 或 failed。",
		},
		"status_codes": map[string]string{
			"200": "查询完成。没有合适的短地名时 place_name 可为空。",
			"400": "坐标无效。",
			"401": "批量接口缺少或使用了无效 Bearer 凭据；单点请求若携带凭据但无效也返回 401。",
			"404": "批量 API 未启用，或任务不存在/不属于当前调用方。",
			"429": "公网入口限流。按 Retry-After 退避。",
			"500": "查询失败，可延后再试。",
			"502": "公网入口暂时无法连接反查服务。",
			"503": "反查繁忙或数据库未就绪。存在 Retry-After 时按其退避。",
			"504": "公网入口等待反查结果超时。",
		},
		"access": "单点反查免认证。批量反查需使用运维后台创建的 Bearer 凭据；管理接口需管理认证。",
		"client_metadata": map[string]string{
			"Authorization":     "单点反查可选 Bearer 凭据，用于归属调用方统计；不传仍可匿名查询。",
			"X-Client-Platform": "可选客户端平台标签，例如 android、web、ios。仅用于统计，不用于鉴权。",
			"X-Client-Version":  "可选客户端版本标签，仅用于统计，不用于鉴权。",
		},
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
	routes.HandleFunc("GET /admin/logs/{id}", s.admin(s.logDetail))
	routes.HandleFunc("GET /admin/stats", s.admin(s.stats))
	routes.HandleFunc("GET /admin/stats/daily", s.admin(s.daily))
	routes.HandleFunc("GET /admin/stats/usage", s.admin(s.usage))
	routes.HandleFunc("GET /admin/batch-credentials", s.admin(s.listBatchCredentials))
	routes.HandleFunc("POST /admin/batch-credentials", s.admin(s.createBatchCredential))
	routes.HandleFunc("DELETE /admin/batch-credentials/{id}", s.admin(s.revokeBatchCredential))
	routes.HandleFunc("POST /admin/batch-credentials/{id}/reveal", s.admin(s.revealBatchCredential))
	routes.HandleFunc("GET /{$}", s.asset("index.html"))
	routes.HandleFunc("GET /list.html", s.asset("list.html"))
	routes.HandleFunc("GET /requests.html", s.asset("requests.html"))
	routes.HandleFunc("GET /usage.html", s.asset("usage.html"))
	routes.HandleFunc("GET /credentials.html", s.asset("credentials.html"))
	static := http.FileServer(http.Dir(s.Config.Frontend))
	revalidateStatic := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		static.ServeHTTP(w, r)
	})
	routes.Handle("GET /css/", revalidateStatic)
	routes.Handle("GET /js/", revalidateStatic)
	routes.Handle("GET /vendor/", revalidateStatic)
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
		w.Header().Set("Cache-Control", "no-cache")
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
	writeJSONTo(w, w, status, body)
}

func writeJSONTo(w http.ResponseWriter, dest io.Writer, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(dest).Encode(body); err != nil {
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
