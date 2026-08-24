package jobpool

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestPoolRunsAllWork(t *testing.T) {
	pool := New(4, 2)
	defer pool.Close()
	var count atomic.Int64
	work := make([]Work, 10)
	for i := range work {
		work[i] = func(context.Context) error {
			count.Add(1)
			return nil
		}
	}
	pool.Submit("job", context.Background(), false, work)
	if err := pool.Wait(context.Background(), "job"); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if count.Load() != 10 {
		t.Fatalf("executed %d items, want 10", count.Load())
	}
}

func TestPoolCapacityCapsConcurrency(t *testing.T) {
	pool := New(8, 3)
	defer pool.Close()
	var active, peak atomic.Int64
	work := make([]Work, 12)
	for i := range work {
		work[i] = func(context.Context) error {
			cur := active.Add(1)
			for {
				prev := peak.Load()
				if cur <= prev || peak.CompareAndSwap(prev, cur) {
					break
				}
			}
			time.Sleep(5 * time.Millisecond)
			active.Add(-1)
			return nil
		}
	}
	pool.Submit("job", context.Background(), false, work)
	if err := pool.Wait(context.Background(), "job"); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if peak.Load() > 3 {
		t.Fatalf("peak concurrency %d, want <= 3", peak.Load())
	}
}

func TestPoolLiveResize(t *testing.T) {
	pool := New(8, 1)
	defer pool.Close()
	var active atomic.Int64
	work := make([]Work, 20)
	for i := range work {
		work[i] = func(context.Context) error {
			active.Add(1)
			time.Sleep(10 * time.Millisecond)
			active.Add(-1)
			return nil
		}
	}
	pool.Submit("job", context.Background(), false, work)
	go func() {
		time.Sleep(5 * time.Millisecond)
		pool.SetCapacity(4)
	}()
	if err := pool.Wait(context.Background(), "job"); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if active.Load() != 0 {
		t.Fatalf("items still active: %d", active.Load())
	}
}

func TestPoolPauseParksWork(t *testing.T) {
	pool := New(2, 2)
	defer pool.Close()
	var executed atomic.Int64
	work := make([]Work, 6)
	for i := range work {
		work[i] = func(context.Context) error {
			executed.Add(1)
			return nil
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool.Submit("job", ctx, true, work)
	done := make(chan error, 1)
	go func() {
		done <- pool.Wait(ctx, "job")
	}()
	select {
	case err := <-done:
		t.Fatalf("Wait returned while paused: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	if executed.Load() != 0 {
		t.Fatalf("paused job executed %d items, want 0", executed.Load())
	}
	pool.SetJobPaused("job", false)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Wait after resume: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Wait did not return after resume")
	}
	if executed.Load() != 6 {
		t.Fatalf("executed %d items, want 6", executed.Load())
	}
}

func TestPoolFirstErrorStopsJob(t *testing.T) {
	pool := New(4, 4)
	defer pool.Close()
	wantErr := errors.New("boom")
	work := make([]Work, 8)
	for i := range work {
		if i == 0 {
			work[i] = func(context.Context) error { return wantErr }
			continue
		}
		work[i] = func(context.Context) error { return nil }
	}
	pool.Submit("job", context.Background(), false, work)
	err := pool.Wait(context.Background(), "job")
	if !errors.Is(err, wantErr) {
		t.Fatalf("Wait error = %v, want %v", err, wantErr)
	}
}

func TestPoolCancelDropsQueuedWork(t *testing.T) {
	pool := New(1, 1)
	defer pool.Close()
	var executed atomic.Int64
	start := make(chan struct{})
	work := make([]Work, 5)
	for i := range work {
		i := i
		work[i] = func(ctx context.Context) error {
			if i == 0 {
				<-start
				return nil
			}
			executed.Add(1)
			return nil
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool.Submit("job", ctx, false, work)
	// Give the first item time to start, then cancel the job.
	time.Sleep(20 * time.Millisecond)
	cancel()
	close(start)
	if err := pool.Wait(ctx, "job"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait error = %v, want context.Canceled", err)
	}
	if executed.Load() != 0 {
		t.Fatalf("cancelled job executed %d queued items, want 0", executed.Load())
	}
}

// TestPoolPausedWorkFreesWorkersForOtherJobs verifies the pool is shared:
// a paused job's queued work reserves no capacity, so other jobs pick up the
// freed workers, and the paused job's work runs again once it is unblocked.
func TestPoolPausedWorkFreesWorkersForOtherJobs(t *testing.T) {
	pool := New(8, 1)
	defer pool.Close()
	ctx := context.Background()

	var held atomic.Bool
	hold := make(chan struct{})
	pool.Submit("blocker", ctx, false, []Work{func(context.Context) error {
		held.Store(true)
		<-hold
		return nil
	}})
	waitUntil(t, func() bool { return held.Load() })

	pausedRan := make(chan struct{}, 1)
	pool.Submit("paused", ctx, true, []Work{func(context.Context) error {
		pausedRan <- struct{}{}
		return nil
	}})

	otherRan := make(chan struct{}, 1)
	pool.Submit("other", ctx, false, []Work{func(context.Context) error {
		otherRan <- struct{}{}
		return nil
	}})

	// The blocker holds the only slot; the other job must wait and the
	// paused job must not run at all.
	select {
	case <-otherRan:
		t.Fatal("other job ran while the only slot was held")
	case <-pausedRan:
		t.Fatal("paused job ran while paused")
	case <-time.After(30 * time.Millisecond):
	}

	// Releasing the blocker frees the worker. The other job takes it; the
	// paused job's queued work does not, since it must not hold capacity.
	close(hold)
	select {
	case <-otherRan:
	case <-time.After(time.Second):
		t.Fatal("freed worker was not picked up by the other job")
	}
	select {
	case <-pausedRan:
		t.Fatal("paused job ran while still paused")
	case <-time.After(30 * time.Millisecond):
	}

	// Unblocking the paused job admits its queued work on the freed worker.
	pool.SetJobPaused("paused", false)
	select {
	case <-pausedRan:
	case <-time.After(time.Second):
		t.Fatal("paused job work did not run after resume")
	}
}

// TestPoolWorkersSharedAcrossJobs verifies concurrent execution across jobs:
// items from different jobs run at the same time on the shared pool.
func TestPoolWorkersSharedAcrossJobs(t *testing.T) {
	pool := New(8, 4)
	defer pool.Close()
	ctx := context.Background()
	var active, peak atomic.Int64
	release := make(chan struct{})
	work := func(context.Context) error {
		cur := active.Add(1)
		for {
			prev := peak.Load()
			if cur <= prev || peak.CompareAndSwap(prev, cur) {
				break
			}
		}
		defer active.Add(-1)
		<-release
		return nil
	}
	jobA := make([]Work, 4)
	jobB := make([]Work, 4)
	for i := 0; i < 4; i++ {
		jobA[i] = work
		jobB[i] = work
	}
	pool.Submit("a", ctx, false, jobA)
	pool.Submit("b", ctx, false, jobB)
	waitUntil(t, func() bool { return peak.Load() >= 2 })
	close(release)
	if err := pool.Wait(ctx, "a"); err != nil {
		t.Fatalf("Wait a: %v", err)
	}
	if err := pool.Wait(ctx, "b"); err != nil {
		t.Fatalf("Wait b: %v", err)
	}
}

// TestPoolCancelAloneReportsCanceled verifies that CancelJob makes Wait return
// context.Canceled even when the job's context was never cancelled — the
// previous behavior returned nil and let the caller mark a cancelled job done.
func TestPoolCancelAloneReportsCanceled(t *testing.T) {
	pool := New(1, 1)
	defer pool.Close()
	started := make(chan struct{})
	release := make(chan struct{})
	pool.Submit("job", context.Background(), false, []Work{func(context.Context) error {
		close(started)
		<-release
		return nil
	}})
	<-started
	pool.CancelJob("job")
	if err := pool.Wait(context.Background(), "job"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait error = %v, want context.Canceled", err)
	}
	close(release)
}

func waitUntil(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("condition not met within 2s")
}
