package geocode

import (
	"context"
	"sync"
	"time"
)

type RequestPriority uint8

const (
	PriorityInteractive RequestPriority = iota
	PrioritySmallBatch
	PriorityBatch
)

type gateWaiter struct {
	priority RequestPriority
	enqueued time.Time
	ready    chan struct{}
	canceled bool
	granted  bool
}

type upstreamGate struct {
	mu          sync.Mutex
	active      bool
	nextAllowed time.Time
	until       time.Time
	fails       int
	backoff     time.Duration
	waiters     []*gateWaiter
	timer       *time.Timer
}

func (g *upstreamGate) acquire(ctx context.Context, priority RequestPriority, interval time.Duration) error {
	waiter := &gateWaiter{priority: priority, enqueued: time.Now(), ready: make(chan struct{})}
	g.mu.Lock()
	g.waiters = append(g.waiters, waiter)
	g.dispatchLocked(interval)
	g.mu.Unlock()

	select {
	case <-waiter.ready:
		return nil
	case <-ctx.Done():
		g.mu.Lock()
		if waiter.granted {
			g.active = false
			g.dispatchLocked(interval)
		} else {
			waiter.canceled = true
			g.dispatchLocked(interval)
		}
		g.mu.Unlock()
		return ctx.Err()
	}
}

func (g *upstreamGate) release(interval time.Duration) {
	g.mu.Lock()
	g.active = false
	g.dispatchLocked(interval)
	g.mu.Unlock()
}

func (g *upstreamGate) dispatchLocked(interval time.Duration) {
	if g.active {
		return
	}
	remaining := g.waiters[:0]
	for _, waiter := range g.waiters {
		if !waiter.canceled {
			remaining = append(remaining, waiter)
		}
	}
	g.waiters = remaining
	if len(g.waiters) == 0 {
		if g.timer != nil {
			g.timer.Stop()
			g.timer = nil
		}
		return
	}
	if delay := time.Until(g.nextAllowed); delay > 0 {
		if g.timer == nil {
			g.timer = time.AfterFunc(delay, func() {
				g.mu.Lock()
				g.timer = nil
				g.dispatchLocked(interval)
				g.mu.Unlock()
			})
		}
		return
	}
	best := 0
	bestPriority := effectivePriority(g.waiters[0], time.Now())
	for i := 1; i < len(g.waiters); i++ {
		priority := effectivePriority(g.waiters[i], time.Now())
		if priority < bestPriority || priority == bestPriority && g.waiters[i].enqueued.Before(g.waiters[best].enqueued) {
			best, bestPriority = i, priority
		}
	}
	waiter := g.waiters[best]
	g.waiters = append(g.waiters[:best], g.waiters[best+1:]...)
	waiter.granted = true
	g.active = true
	g.nextAllowed = time.Now().Add(interval)
	close(waiter.ready)
}

func effectivePriority(waiter *gateWaiter, now time.Time) int {
	aged := int(now.Sub(waiter.enqueued) / (20 * time.Second))
	return max(0, int(waiter.priority)-aged)
}
