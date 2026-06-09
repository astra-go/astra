//go:build cron
// +build cron

package runner

// Package comment: see runner.go for package documentation.
// This file provides the cron backend, enabled with build tag "cron".

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	astracron "github.com/astra-go/astra/cron"
)

// CronRunner is a Runner backed by robfig/cron/v3 (via astra/cron).
// All methods are safe for concurrent use.
type CronRunner struct {
	s    *astracron.Scheduler
	mu   sync.RWMutex
	meta map[string]string // name → expr
}

// NewCronRunner creates a cron-backed Runner with second-resolution and panic recovery.
func NewCronRunner() *CronRunner {
	return &CronRunner{
		s:    astracron.NewScheduler(),
		meta: make(map[string]string),
	}
}

// Add schedules job using a cron expression.
// Returns an error if a job with the same name is already registered.
func (r *CronRunner) Add(name, expr string, job JobFunc) error {
	r.mu.Lock()
	if _, exists := r.meta[name]; exists {
		r.mu.Unlock()
		return fmt.Errorf("runner: job %q already registered", name)
	}
	r.meta[name] = expr
	r.mu.Unlock()

	return r.s.Cron(expr, name, astracron.JobFunc(func(ctx context.Context) {
		if err := job(ctx); err != nil {
			slog.Error("runner: job error", "name", name, "err", err)
		}
	}))
}

// Every schedules job at a fixed interval.
// Returns an error if a job with the same name is already registered.
func (r *CronRunner) Every(name string, d time.Duration, job JobFunc) error {
	r.mu.Lock()
	if _, exists := r.meta[name]; exists {
		r.mu.Unlock()
		return fmt.Errorf("runner: job %q already registered", name)
	}
	expr := fmt.Sprintf("@every %s", d)
	r.meta[name] = expr
	r.mu.Unlock()

	return r.s.Every(d, name, astracron.JobFunc(func(ctx context.Context) {
		if err := job(ctx); err != nil {
			slog.Error("runner: job error", "name", name, "err", err)
		}
	}))
}

// Start begins executing registered jobs. Non-blocking.
// Cancelling ctx triggers a graceful shutdown.
func (r *CronRunner) Start(ctx context.Context) error {
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
func (r *CronRunner) Stop(ctx context.Context) error {
	return r.s.Shutdown(ctx)
}

// Jobs returns a snapshot of all registered jobs and their schedule metadata.
func (r *CronRunner) Jobs() []JobInfo {
	entries := r.s.Entries()
	r.mu.RLock()
	defer r.mu.RUnlock()
	jobs := make([]JobInfo, len(entries))
	for i, e := range entries {
		jobs[i] = JobInfo{
			Name: e.Name,
			Expr: r.meta[e.Name],
			Next: e.Next,
			Prev: e.Prev,
		}
	}
	return jobs
}

// Verify CronRunner implements Runner at compile time.
var _ Runner = (*CronRunner)(nil)
