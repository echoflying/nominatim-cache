package geocode

import (
	"context"
	"testing"
	"time"
)

func TestUpstreamGatePrioritizesInteractiveWaiters(t *testing.T) {
	gate := &upstreamGate{}
	if err := gate.acquire(t.Context(), PriorityBatch, 0); err != nil {
		t.Fatal(err)
	}
	started := make(chan RequestPriority, 2)
	queue := func(priority RequestPriority) {
		t.Helper()
		go func() {
			if err := gate.acquire(t.Context(), priority, 0); err != nil {
				return
			}
			started <- priority
			gate.release(0)
		}()
	}
	queue(PriorityBatch)
	waitForGateWaiters(t, gate, 1)
	queue(PriorityInteractive)
	waitForGateWaiters(t, gate, 2)
	gate.release(0)

	if got := <-started; got != PriorityInteractive {
		t.Fatalf("first granted priority = %v, want interactive", got)
	}
	if got := <-started; got != PriorityBatch {
		t.Fatalf("second granted priority = %v, want batch", got)
	}
}

func TestUpstreamGateAgesBulkWaiters(t *testing.T) {
	now := time.Now()
	waiter := &gateWaiter{priority: PriorityBatch, enqueued: now.Add(-45 * time.Second)}
	if got := effectivePriority(waiter, now); got != 0 {
		t.Fatalf("aged priority = %d, want interactive class 0", got)
	}
}

func TestUpstreamGateSkipsCanceledHighPriorityWaiter(t *testing.T) {
	gate := &upstreamGate{}
	if err := gate.acquire(t.Context(), PriorityBatch, 0); err != nil {
		t.Fatal(err)
	}

	canceledCtx, cancel := context.WithCancel(t.Context())
	canceledDone := make(chan error, 1)
	go func() { canceledDone <- gate.acquire(canceledCtx, PriorityInteractive, 0) }()
	waitForGateWaiters(t, gate, 1)
	started := make(chan RequestPriority, 1)
	go func() {
		if err := gate.acquire(t.Context(), PriorityBatch, 0); err != nil {
			return
		}
		started <- PriorityBatch
		gate.release(0)
	}()
	waitForGateWaiters(t, gate, 2)

	cancel()
	select {
	case err := <-canceledDone:
		if err == nil {
			t.Fatal("canceled waiter was granted")
		}
	case <-time.After(time.Second):
		t.Fatal("canceled waiter did not leave the queue")
	}
	gate.release(0)
	select {
	case got := <-started:
		if got != PriorityBatch {
			t.Fatalf("granted priority = %v, want batch", got)
		}
	case <-time.After(time.Second):
		t.Fatal("remaining waiter was not granted")
	}
}

func waitForGateWaiters(t *testing.T, gate *upstreamGate, count int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		gate.mu.Lock()
		ready := len(gate.waiters) == count
		gate.mu.Unlock()
		if ready {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for priority queue")
}
