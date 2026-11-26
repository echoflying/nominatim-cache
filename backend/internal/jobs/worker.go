package jobs

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/echoflying/nominatim-cache/backend/internal/geocode"
	"github.com/echoflying/nominatim-cache/backend/internal/store"
)

type Worker struct {
	Store    store.Store
	Upstream *geocode.Upstream
	Locks    *geocode.KeyLocks
	Wake     chan struct{}
}

func (w Worker) Run(ctx context.Context) {
	for ctx.Err() == nil {
		claimCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		task, err := w.Store.Claim(claimCtx, 6*time.Minute)
		cancel()
		if err == nil {
			if err := w.process(ctx, task); err != nil {
				if ctx.Err() != nil {
					releaseCtx, done := context.WithTimeout(context.Background(), 3*time.Second)
					if releaseErr := w.Store.Release(releaseCtx, task); releaseErr != nil && !errors.Is(releaseErr, store.ErrLeaseLost) {
						slog.Warn("释放批量任务租约失败", "error", releaseErr)
					}
					done()
				} else {
					slog.Error("保存批量反查任务状态失败", "error", err)
				}
			}
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) && ctx.Err() == nil {
			slog.Error("领取批量反查任务失败", "error", err)
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
		case <-w.Wake:
		case <-timer.C:
		}
		timer.Stop()
	}
}

func (w Worker) process(parent context.Context, task store.Task) error {
	ctx, cancel := context.WithTimeout(parent, 5*time.Minute)
	defer cancel()
	release, err := w.Locks.Lock(ctx, task.Key)
	if err != nil {
		return err
	}
	defer release()

	cached, err := w.Store.Cache(ctx, task.Key)
	if err == nil {
		if response, err := cached.Public(); err == nil {
			return w.Store.Finish(ctx, task, "cached", "", response, 0)
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	const maxAttempts = 3
	if task.Attempts > maxAttempts {
		return w.Store.Finish(ctx, task, "failed", "attempts_exhausted", nil, 0)
	}
	response, payload, err := w.Upstream.Batch(ctx, task.Point, task.Priority)
	updateCtx, stop := context.WithTimeout(parent, 5*time.Second)
	defer stop()
	switch {
	case err == nil:
		return w.Store.Finish(updateCtx, task, "resolved", "", response, payload.ArchiveID)
	case errors.Is(err, geocode.ErrNoResult):
		return w.Store.Finish(updateCtx, task, "no_result", "no_result", nil, 0)
	case errors.Is(err, geocode.ErrBatchSourceUnavailable):
		return w.Store.Finish(updateCtx, task, "failed", "batch_source_unavailable", nil, 0)
	case task.Attempts >= maxAttempts:
		return w.Store.Finish(updateCtx, task, "failed", "upstream_unavailable", nil, 0)
	default:
		next := time.Now().Add(time.Duration(1<<task.Attempts) * 5 * time.Second)
		var later *geocode.RetryLater
		if errors.As(err, &later) && later.At.After(next) {
			next = later.At
		}
		return w.Store.Retry(updateCtx, task, next, "upstream_unavailable")
	}
}
