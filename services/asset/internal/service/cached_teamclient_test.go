package service

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/hieu-seta/seta-training/pkg/cache"
)

// blockingTeamClient counts ManagersOf invocations and blocks until released,
// so concurrent callers pile up inside singleflight before the load returns.
type blockingTeamClient struct {
	calls   int64
	release chan struct{}
	result  []uuid.UUID
}

func (c *blockingTeamClient) ManagersOf(_ context.Context, _ uuid.UUID) ([]uuid.UUID, error) {
	atomic.AddInt64(&c.calls, 1)
	<-c.release // hold the load open while siblings join the flight
	return c.result, nil
}

// Concurrent cache misses for the same key must collapse into one inner load.
func TestCachedTeamClient_SingleflightCollapsesConcurrentMisses(t *testing.T) {
	uid := uuid.New()
	mgr := uuid.New()
	inner := &blockingTeamClient{release: make(chan struct{}), result: []uuid.UUID{mgr}}
	// cache.Noop always misses → every caller is forced through singleflight.
	c := NewCachedTeamClient(inner, cache.Noop{})

	const n = 20
	var entered int64 // goroutines that have reached the cached call
	var wg sync.WaitGroup
	results := make([][]uuid.UUID, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			atomic.AddInt64(&entered, 1)
			results[idx], errs[idx] = c.ManagersOf(context.Background(), uid)
		}(i)
	}

	// Wait until all callers have entered ManagersOf AND a load is in-flight,
	// so every sibling joins the same flight before the load returns.
	// Without singleflight, multiple callers would each invoke inner before release.
	for atomic.LoadInt64(&entered) < n || atomic.LoadInt64(&inner.calls) < 1 {
		runtime.Gosched()
	}
	// Yield generously so any straggler that incremented `entered` but has not yet
	// reached sf.Do joins the in-flight load rather than starting a second one.
	for i := 0; i < 1000; i++ {
		runtime.Gosched()
	}
	close(inner.release)
	wg.Wait()

	if got := atomic.LoadInt64(&inner.calls); got != 1 {
		t.Fatalf("inner ManagersOf called %d times, want 1 (singleflight collapse)", got)
	}
	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Errorf("caller %d err = %v", i, errs[i])
		}
		if len(results[i]) != 1 || results[i][0] != mgr {
			t.Errorf("caller %d result = %v, want [%v]", i, results[i], mgr)
		}
	}
}
