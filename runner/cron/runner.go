// Package cron provides a cron-backed Runner implementation for Astra,
// wrapping the astra/cron package (robfig/cron/v3).
//
// Jobs run in-process. For distributed scheduling with persistence and retries,
// use runner/taskqueue instead.
//
// # Usage
//
//	r := cron.New()
//	r.Add("report", "0 9 * * *", func(ctx context.Context) error {
//	    return generateDailyReport(ctx)
//	})
//	r.Every("heartbeat", time.Minute, func(ctx context.Context) error {
//	    return pingHealthcheck(ctx)
//	})
//	r.Start(ctx)
//	defer r.Stop(context.Background())
//
// # App lifecycle integration
//
//	r := cron.New()
//	r.Add("cleanup", "@daily", cleanupFn)
//	app.OnStart(r.Start)
//	app.OnStop(r.Stop)
package cron

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	astracron "github.com/astra-go/astra/cron"
	"github.com/astra-go/astra/runner"
)

// Runner is a Runner backed by robfig/cron/v3 (via astra/cron).
// All methods are safe for concurrent use.
type Runner struct {
	s    *astracron.Scheduler
	mu   sync.RWMutex
	meta map[string]string // name → expr
}

// New creates a cron-backed Runner with second-resolution and panic recovery.
func New() *Runner {
	return &Runner{
		s:    astracron.NewScheduler(),
		meta: make(map[string]string),
	}
}

// Add schedules job using a cron expression.
// Returns an error if a job with the same name is already registered.
func (r *Runner) Add(name, expr string, job runner.JobFunc) error {
	r.mu.Lock()
	if _, exists := r.meta[name]; exists {
		r.mu.Unlock()
		return fmt.Errorf("runner/cron: job %q already registered", name)
	}
	r.meta[name] = expr
	r.mu.Unlock()

	return r.s.Cron(expr, name, astracron.JobFunc(func(ctx context.Context) {
		if err := job(ctx); err != nil {
			slog.Error("runner/cron: job error", "name", name, "err", err)
		}
	}))
}

// Every schedules job at a fixed interval.
// Returns an error if a job with the same name is already registered.
func (r *Runner) Every(name string, d time.Duration, job runner.JobFunc) error {
	r.mu.Lock()
	if _, exists := r.meta[name]; exists {
		r.mu.Unlock()
		return fmt.Errorf("runner/cron: job %q already registered", name)
	}
	expr := fmt.Sprintf("@every %s", d)
	r.meta[name] = expr
	r.mu.Unlock()

	return r.s.Every(d, name, astracron.JobFunc(func(ctx context.Context) {
		if err := job(ctx); err != nil {
			slog.Error("runner/cron: job error", "name", name, "err", err)
		}
	}))
}

// Start begins executing registered jobs. Non-blocking.
// Cancelling ctx triggers a graceful shutdown.
func (r *Runner) Start(ctx context.Context) error {
	r.s.Start()
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = r.s.Shutdown(shutCtx)
	}()
	return nil
}

// Stop gracefully stops the runner.
// Blocks until all running jobs finish or ctx expires.
func (r *Runner) Stop(ctx context.Context) error {
	return r.s.Shutdown(ctx)
}

// Jobs returns a snapshot of all registered jobs and their schedule metadata.
func (r *Runner) Jobs() []runner.JobInfo {
	entries := r.s.Entries()
	r.mu.RLock()
	defer r.mu.RUnlock()
	jobs := make([]runner.JobInfo, len(entries))
	for i, e := range entries {
		jobs[i] = runner.JobInfo{
			Name: e.Name,
			Expr: r.meta[e.Name],
			Next: e.Next,
			Prev: e.Prev,
		}
	}
	return jobs
}

// Verify Runner implements runner.Runner at compile time.
var _ runner.Runner = (*Runner)(nil)
