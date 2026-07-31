package checker

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// onceTargetSource hands out its target list exactly once, on whichever
// tick asks first, and nil on every subsequent tick. That makes the total
// number of dispatched jobs deterministic regardless of how many times the
// scheduler ticks before the test's context expires.
type onceTargetSource struct {
	once    sync.Once
	targets []Target
}

func (s *onceTargetSource) DueTargets(context.Context, time.Time) ([]Target, error) {
	var out []Target
	s.once.Do(func() { out = s.targets })
	return out, nil
}

type recordingRecorder struct {
	mu      sync.Mutex
	results []Result
}

func (r *recordingRecorder) Record(_ context.Context, result Result) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.results = append(r.results, result)
	return nil
}

func (r *recordingRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.results)
}

// concurrencyTrackingProber records the highest number of simultaneous
// Probe calls it ever saw, so a test can assert the worker pool actually
// bounds concurrency rather than firing every job at once.
type concurrencyTrackingProber struct {
	inFlight    int64
	maxInFlight int64
	fn          func(ctx context.Context, target Target) Result
}

func (p *concurrencyTrackingProber) Probe(ctx context.Context, target Target) Result {
	cur := atomic.AddInt64(&p.inFlight, 1)
	for {
		max := atomic.LoadInt64(&p.maxInFlight)
		if cur <= max || atomic.CompareAndSwapInt64(&p.maxInFlight, max, cur) {
			break
		}
	}
	defer atomic.AddInt64(&p.inFlight, -1)
	return p.fn(ctx, target)
}

func makeTargets(n int, timeoutMS int32) []Target {
	targets := make([]Target, n)
	for i := range targets {
		targets[i] = Target{
			ID:                   int64(i + 1),
			Name:                 fmt.Sprintf("target-%d", i+1),
			URL:                  "http://example.invalid",
			ExpectedStatusMin:    200,
			ExpectedStatusMax:    300,
			IntervalSeconds:      1,
			TimeoutMS:            timeoutMS,
			ConsecutiveThreshold: 3,
		}
	}
	return targets
}

func TestEngineBoundsConcurrency(t *testing.T) {
	const workers = 3
	const numTargets = 20

	prober := &concurrencyTrackingProber{
		fn: func(ctx context.Context, target Target) Result {
			time.Sleep(20 * time.Millisecond)
			return Result{TargetID: target.ID, CheckedAt: time.Now(), Success: true}
		},
	}
	recorder := &recordingRecorder{}
	engine := &Engine{
		Targets:   &onceTargetSource{targets: makeTargets(numTargets, 1000)},
		Prober:    prober,
		Recorder:  recorder,
		Workers:   workers,
		TickEvery: 5 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	if err := engine.Run(ctx); err != context.DeadlineExceeded {
		t.Fatalf("Run() error = %v, want context.DeadlineExceeded", err)
	}

	if recorder.count() != numTargets {
		t.Errorf("recorded %d results, want %d", recorder.count(), numTargets)
	}
	if max := atomic.LoadInt64(&prober.maxInFlight); max > workers {
		t.Errorf("max concurrent probes = %d, want <= %d (worker pool should bound concurrency)", max, workers)
	}
}

func TestEngineSlowTargetDoesNotBlockOthers(t *testing.T) {
	slow := Target{ID: 1, URL: "http://slow.invalid", TimeoutMS: 1000, ExpectedStatusMax: 300, ConsecutiveThreshold: 1}
	fast := Target{ID: 2, URL: "http://fast.invalid", TimeoutMS: 1000, ExpectedStatusMax: 300, ConsecutiveThreshold: 1}

	var fastDone, slowStarted time.Time
	var mu sync.Mutex
	slowRelease := make(chan struct{})

	prober := &concurrencyTrackingProber{
		fn: func(ctx context.Context, target Target) Result {
			if target.ID == slow.ID {
				mu.Lock()
				slowStarted = time.Now()
				mu.Unlock()
				<-slowRelease
			} else {
				mu.Lock()
				fastDone = time.Now()
				mu.Unlock()
			}
			return Result{TargetID: target.ID, CheckedAt: time.Now(), Success: true}
		},
	}
	recorder := &recordingRecorder{}
	engine := &Engine{
		Targets:   &onceTargetSource{targets: []Target{slow, fast}},
		Prober:    prober,
		Recorder:  recorder,
		Workers:   2, // one per target, so neither has to wait on the other
		TickEvery: 5 * time.Millisecond,
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- engine.Run(ctx) }()

	// Give the fast target time to complete while the slow one is still
	// blocked in its handler.
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	fastRecorded := !fastDone.IsZero()
	slowHasStarted := !slowStarted.IsZero()
	mu.Unlock()

	if !slowHasStarted {
		t.Fatal("slow target's check never started")
	}
	if !fastRecorded {
		t.Error("fast target did not complete while slow target was still blocked; a slow target is blocking the pool")
	}

	close(slowRelease)
	cancel()
	<-done
}

func TestEnginePerTargetTimeoutIsIndependentOfRunContext(t *testing.T) {
	// The target's own TimeoutMS (20ms) is much shorter than the run
	// context's lifetime, so the probe's ctx should be cancelled by the
	// per-check timeout well before the run context ever would be.
	target := Target{ID: 1, URL: "http://example.invalid", TimeoutMS: 20, ExpectedStatusMax: 300, ConsecutiveThreshold: 1}

	var gotTimeout bool
	prober := &concurrencyTrackingProber{
		fn: func(ctx context.Context, target Target) Result {
			select {
			case <-ctx.Done():
				gotTimeout = true
				return Result{TargetID: target.ID, CheckedAt: time.Now(), Success: false, Err: ctx.Err().Error()}
			case <-time.After(2 * time.Second):
				return Result{TargetID: target.ID, CheckedAt: time.Now(), Success: true}
			}
		},
	}
	recorder := &recordingRecorder{}
	engine := &Engine{
		Targets:   &onceTargetSource{targets: []Target{target}},
		Prober:    prober,
		Recorder:  recorder,
		Workers:   1,
		TickEvery: 5 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	if err := engine.Run(ctx); err != context.DeadlineExceeded {
		t.Fatalf("Run() error = %v, want context.DeadlineExceeded", err)
	}
	if !gotTimeout {
		t.Error("probe never observed its context being cancelled by the per-target timeout")
	}
}

func TestEngineShutsDownCleanlyOnCancel(t *testing.T) {
	prober := &concurrencyTrackingProber{
		fn: func(ctx context.Context, target Target) Result {
			return Result{TargetID: target.ID, CheckedAt: time.Now(), Success: true}
		},
	}
	engine := &Engine{
		Targets:   &onceTargetSource{targets: makeTargets(5, 1000)},
		Prober:    prober,
		Recorder:  &recordingRecorder{},
		Workers:   2,
		TickEvery: 5 * time.Millisecond,
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- engine.Run(ctx) }()

	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return within 2s of cancellation")
	}
}
