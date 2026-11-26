package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"sync"
	"syscall"
	"time"

	"github.com/echoflying/nominatim-cache/backend/internal/config"
	"github.com/echoflying/nominatim-cache/backend/internal/geocode"
	"github.com/echoflying/nominatim-cache/backend/internal/httpapi"
	"github.com/echoflying/nominatim-cache/backend/internal/jobs"
	"github.com/echoflying/nominatim-cache/backend/internal/store"
	"github.com/echoflying/nominatim-cache/backend/migrations"
)

var version = "dev"
var commit = "unknown"

func main() {
	if err := run(); err != nil {
		slog.Error("服务退出", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(cfg.Frontend, "index.html")); err != nil {
		return err
	}
	upstream, err := geocode.NewUpstream(cfg.Sources, cfg.BatchSource, cfg.Interval, cfg.UpstreamTimeout)
	if err != nil {
		return err
	}
	db, err := store.Open(cfg.DSN)
	if err != nil {
		return err
	}
	defer db.Close()
	startup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	err = migrations.Verify(startup, db)
	cancel()
	if err != nil {
		return err
	}
	if commit == "unknown" {
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, setting := range info.Settings {
				if setting.Key == "vcs.revision" {
					commit = setting.Value
				}
				if setting.Key == "vcs.modified" && setting.Value == "true" && commit != "unknown" {
					commit += "-dirty"
				}
			}
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	storage := store.Store{DB: db}
	upstream.Archive = storage.ArchiveUpstream
	locks := &geocode.KeyLocks{}
	wake := make(chan struct{}, 1)
	app := httpapi.Server{Config: cfg, Store: storage, Upstream: upstream, Locks: locks, Wake: wake,
		Started: time.Now(), Version: version, Commit: commit}
	server := func(handler http.Handler) *http.Server {
		return &http.Server{
			Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
			WriteTimeout: cfg.ReverseTimeout + 5*time.Second, IdleTimeout: 60 * time.Second,
		}
	}
	var endpoints = []struct {
		addr   string
		server *http.Server
	}{
		{cfg.Listen, server(app.Handler())},
		{cfg.AdminListen, server(app.AdminHandler())},
	}
	// 先绑定两个地址，避免只启动一半才发现端口冲突。
	listeners := make([]net.Listener, 0, len(endpoints))
	for _, endpoint := range endpoints {
		listener, err := net.Listen("tcp", endpoint.addr)
		if err != nil {
			return err
		}
		defer listener.Close()
		listeners = append(listeners, listener)
	}
	var workerGroup sync.WaitGroup
	worker := jobs.Worker{Store: storage, Upstream: upstream, Locks: locks, Wake: wake}
	for range 4 {
		workerGroup.Add(1)
		go func() {
			defer workerGroup.Done()
			worker.Run(ctx)
		}()
	}
	pruneDone := make(chan struct{})
	go func() {
		defer close(pruneDone)
		prune(ctx, storage, cfg.LogRetention, cfg.Location)
	}()
	serveErr := make(chan error, len(endpoints))
	for i, endpoint := range endpoints {
		go func() { serveErr <- endpoint.server.Serve(listeners[i]) }()
	}
	slog.Info("Nominatim Cache 启动", "addr", cfg.Listen, "admin_addr", cfg.AdminListen, "version", version, "commit", commit)
	select {
	case <-ctx.Done():
	case err = <-serveErr:
		stop()
	}
	shutdown, shutdownCancel := context.WithTimeout(context.Background(), cfg.ReverseTimeout+5*time.Second)
	defer shutdownCancel()
	shutdownErr := make(chan error, len(endpoints))
	for _, endpoint := range endpoints {
		go func() {
			closeErr := endpoint.server.Shutdown(shutdown)
			if closeErr != nil {
				_ = endpoint.server.Close()
			}
			shutdownErr <- closeErr
		}()
	}
	for range endpoints {
		if closeErr := <-shutdownErr; err == nil && closeErr != nil {
			err = closeErr
		}
	}
	workerGroup.Wait()
	<-pruneDone
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func prune(ctx context.Context, storage store.Store, logRetention time.Duration, location *time.Location) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			maintenance, cancel := context.WithTimeout(ctx, time.Minute)
			if err := storage.Prune(maintenance, now.In(location), logRetention, 7*24*time.Hour); err != nil && ctx.Err() == nil {
				slog.Warn("清理过期日志与批次失败", "error", err)
			}
			cancel()
		}
	}
}
