package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/echoflying/nominatim-cache/backend/internal/geocode"
)

type Config struct {
	DSN, Listen, Environment, Frontend, BasePath string
	AdminListen, AdminUser, AdminPass            string
	LegacyHost                                   string
	Location                                     *time.Location
	Tokens                                       map[string]string
	Sources                                      []geocode.SourceConfig
	BatchSource                                  *geocode.SourceConfig
	Interval, UpstreamTimeout, ReverseTimeout    time.Duration
	LogRetention, LegacyTimeout                  time.Duration
	BatchLimit, PendingLimit, ColdLimit          int
}

func Load() (Config, error) {
	cfg := Config{
		DSN: os.Getenv("MARIADB_DSN"), Listen: env("LISTEN_ADDR", "127.0.0.1:3000"),
		Environment: env("APP_ENV", "development"), Frontend: env("FRONTEND_DIR", "../frontend"),
		BasePath:    strings.TrimRight(os.Getenv("BASE_PATH"), "/"),
		AdminListen: env("ADMIN_LISTEN_ADDR", "127.0.0.1:3001"),
		LegacyHost:  strings.ToLower(strings.TrimSuffix(env("LEGACY_HOST", "nc.zaitu.cn"), ".")),
		AdminUser:   env("ADMIN_USERNAME", "admin"), AdminPass: env("ADMIN_PASSWORD", "admin123"),
		Tokens: make(map[string]string),
	}
	if cfg.DSN == "" {
		return cfg, errors.New("MARIADB_DSN is required")
	}
	if cfg.Environment == "production" && (cfg.AdminUser == "admin" || cfg.AdminPass == "admin123") {
		return cfg, errors.New("production requires non-default ADMIN_USERNAME and ADMIN_PASSWORD")
	}
	host, port, err := net.SplitHostPort(cfg.AdminListen)
	if err != nil || !net.ParseIP(host).IsLoopback() || port == "" {
		return cfg, errors.New("ADMIN_LISTEN_ADDR must be a loopback IP address and port")
	}
	if strings.ContainsAny(cfg.LegacyHost, ":/?#\\ \t\r\n") || cfg.LegacyHost == "" {
		return cfg, errors.New("LEGACY_HOST must be a hostname without a port")
	}
	if cfg.BasePath != "" && (!strings.HasPrefix(cfg.BasePath, "/") || path.Clean(cfg.BasePath) != cfg.BasePath ||
		strings.ContainsAny(cfg.BasePath, "%\\?# \t\r\n")) {
		return cfg, errors.New("BASE_PATH must be a clean absolute path prefix")
	}
	cfg.Location, err = time.LoadLocation(env("STATS_TIMEZONE", "Asia/Shanghai"))
	if err != nil {
		return cfg, errors.New("invalid STATS_TIMEZONE")
	}
	usedTokens := make(map[string]bool)
	for pair := range strings.SplitSeq(os.Getenv("BATCH_TOKENS"), ",") {
		if pair == "" {
			continue
		}
		name, token, ok := strings.Cut(pair, "=")
		if !ok || name == "" || len(name) > 64 || strings.TrimSpace(name) != name ||
			len(token) < 24 || strings.ContainsAny(token, " \t\r\n") || cfg.Tokens[name] != "" || usedTokens[token] {
			return cfg, errors.New("BATCH_TOKENS requires distinct caller=token pairs with tokens of at least 24 bytes")
		}
		cfg.Tokens[name], usedTokens[token] = token, true
	}
	for _, entry := range []struct {
		key, name string
		photon    bool
	}{
		{"NOMINATIM_PRIMARY", "mirror-earth.com", false},
		{"NOMINATIM_BACKUP_1", "photon.komoot.io", true},
		{"NOMINATIM_BACKUP_2", "openstreetmap.org", false},
	} {
		if address := os.Getenv(entry.key); address != "" {
			cfg.Sources = append(cfg.Sources, geocode.SourceConfig{Name: entry.name, URL: address, Photon: entry.photon})
		}
	}
	if cfg.Environment == "production" && len(cfg.Sources) == 0 {
		return cfg, errors.New("production requires at least one interactive upstream")
	}
	if address := os.Getenv("NOMINATIM_BATCH_URL"); address != "" {
		format := env("NOMINATIM_BATCH_FORMAT", "nominatim")
		if format != "nominatim" && format != "photon" {
			return cfg, errors.New("NOMINATIM_BATCH_FORMAT must be nominatim or photon")
		}
		cfg.BatchSource = &geocode.SourceConfig{Name: "batch", URL: address, Photon: format == "photon"}
	}
	for _, setting := range []struct {
		key            string
		def, low, high int
		dest           *time.Duration
		unit           time.Duration
	}{
		{"REQUEST_INTERVAL", 1500, 1000, 180000, &cfg.Interval, time.Millisecond},
		{"UPSTREAM_TIMEOUT_SECONDS", 10, 1, 60, &cfg.UpstreamTimeout, time.Second},
		{"REVERSE_TIMEOUT_SECONDS", 40, 2, 120, &cfg.ReverseTimeout, time.Second},
		{"LEGACY_TIMEOUT_MS", 6500, 1000, 7000, &cfg.LegacyTimeout, time.Millisecond},
		{"LOG_RETENTION_DAYS", 30, 1, 3650, &cfg.LogRetention, 24 * time.Hour},
	} {
		value, err := number(setting.key, setting.def, setting.low, setting.high)
		if err != nil {
			return cfg, err
		}
		*setting.dest = time.Duration(value) * setting.unit
	}
	cfg.ColdLimit, err = number("REVERSE_COLD_LIMIT", 16, 1, 500)
	if err != nil {
		return cfg, err
	}
	cfg.BatchLimit, err = number("BATCH_LIMIT", 500, 1, 5000)
	if err != nil {
		return cfg, err
	}
	cfg.PendingLimit, err = number("BATCH_PENDING_LIMIT", 2000, cfg.BatchLimit, 100000)
	return cfg, err
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func number(key string, def, low, high int) (int, error) {
	value, err := strconv.Atoi(env(key, strconv.Itoa(def)))
	if err != nil || value < low || value > high {
		return 0, fmt.Errorf("%s must be an integer between %d and %d", key, low, high)
	}
	return value, nil
}
