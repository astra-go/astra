package cron_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/astra-go/astra/cron"
	"github.com/astra-go/astra/testutil"
)

func TestScheduler_Every(t *testing.T) {
	s := cron.NewScheduler()
	var count atomic.Int64
	err := s.Every(time.Second, "tick", cron.JobFunc(func(ctx context.Context) {
		count.Add(1)
	}))
	testutil.AssertNoError(t, err)

	s.Start()
	time.Sleep(2500 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	testutil.AssertNoError(t, s.Shutdown(ctx))

	got := count.Load()
	if got < 2 {
		t.Errorf("expected at least 2 ticks, got %d", got)
	}
}

func TestScheduler_InvalidInterval(t *testing.T) {
	s := cron.NewScheduler()
	err := s.Every(0, "zero", cron.JobFunc(func(ctx context.Context) {}))
	testutil.AssertError(t, err)

	err = s.Every(-time.Second, "negative", cron.JobFunc(func(ctx context.Context) {}))
	testutil.AssertError(t, err)
}

func TestScheduler_Cron(t *testing.T) {
	s := cron.NewScheduler()
	var count atomic.Int64
	// Use @every 1s — the 6-field scheduler supports second resolution.
	err := s.Cron("@every 1s", "cron-tick", cron.JobFunc(func(ctx context.Context) {
		count.Add(1)
	}))
	testutil.AssertNoError(t, err)

	s.Start()
	time.Sleep(2500 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	testutil.AssertNoError(t, s.Shutdown(ctx))

	if count.Load() < 2 {
		t.Errorf("expected at least 2 cron ticks, got %d", count.Load())
	}
}

func TestScheduler_Entries(t *testing.T) {
	s := cron.NewScheduler()
	noop := cron.JobFunc(func(ctx context.Context) {})

	_ = s.Every(time.Minute, "job-a", noop)
	_ = s.Every(time.Hour, "job-b", noop)

	entries := s.Entries()
	testutil.AssertEqual(t, 2, len(entries))

	names := map[string]bool{}
	for _, e := range entries {
		names[e.Name] = true
	}
	if !names["job-a"] || !names["job-b"] {
		t.Errorf("entries missing expected names: %v", names)
	}
}

func TestScheduler_Remove(t *testing.T) {
	s := cron.NewScheduler()
	noop := cron.JobFunc(func(ctx context.Context) {})

	_ = s.Every(time.Minute, "keep", noop)
	_ = s.Every(time.Minute, "remove", noop)

	entries := s.Entries()
	testutil.AssertEqual(t, 2, len(entries))

	// Remove the entry named "remove"
	for _, e := range entries {
		if e.Name == "remove" {
			s.Remove(e.ID)
		}
	}

	entries = s.Entries()
	testutil.AssertEqual(t, 1, len(entries))
	testutil.AssertEqual(t, "keep", entries[0].Name)
}

func TestScheduler_ContextCancelledOnShutdown(t *testing.T) {
	s := cron.NewScheduler()

	// Signal channel lets us know the job goroutine has actually started.
	started := make(chan struct{}, 1)
	var ctxCancelled atomic.Bool

	_ = s.Every(500*time.Millisecond, "ctx-aware", cron.JobFunc(func(ctx context.Context) {
		select {
		case started <- struct{}{}: // signal only on first call
		default:
		}
		// Block until the scheduler cancels the context.
		select {
		case <-ctx.Done():
			ctxCancelled.Store(true)
		case <-time.After(5 * time.Second):
		}
	}))

	s.Start()

	// Wait for the first tick to be RUNNING before issuing shutdown.
	// This avoids the race where cancel() fires before the job goroutine
	// even starts, causing the wrapper's ctx.Err() guard to skip the job.
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("job did not start within 2 seconds")
	}

	shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	testutil.AssertNoError(t, s.Shutdown(shutCtx))

	if !ctxCancelled.Load() {
		t.Error("expected job context to be cancelled on shutdown")
	}
}

func TestScheduler_PanicRecovery(t *testing.T) {
	// A panicking job must not crash the scheduler
	s := cron.NewScheduler()
	var after atomic.Int64

	_ = s.Every(time.Second, "panicker", cron.JobFunc(func(ctx context.Context) {
		panic("intentional test panic")
	}))
	_ = s.Every(time.Second, "healthy", cron.JobFunc(func(ctx context.Context) {
		after.Add(1)
	}))

	s.Start()
	time.Sleep(2500 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = s.Shutdown(ctx)

	if after.Load() == 0 {
		t.Error("healthy job should still run after sibling panics")
	}
}
