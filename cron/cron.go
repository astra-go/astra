// Package cron provides a job scheduler for Astra applications.
//
// # Interval-based scheduling
//
//	s := cron.NewScheduler()
//	s.Every(5*time.Minute, "cache-warmup", func(ctx context.Context) {
//	    warmCache(ctx)
//	})
//	s.Start()
//	defer s.Shutdown(context.Background())
//
// # Cron expression scheduling
//
//	s.Cron("0 30 2 * * *", "nightly-report", func(ctx context.Context) {
//	    // runs every day at 02:30:00 (with second-resolution enabled)
//	})
//
// # Integration with App lifecycle (recommended)
//
//	s := cron.NewScheduler()
//	s.Every(time.Minute, "heartbeat", func(ctx context.Context) { ... })
//	app.OnStart(func(ctx context.Context) error { s.Start(); return nil })
//	app.OnStop(s.Shutdown)
//
// # Cron expression format (6-field, second resolution)
//
//	┌─────────────── second       (0–59)
//	│ ┌───────────── minute       (0–59)
//	│ │ ┌─────────── hour         (0–23)
//	│ │ │ ┌───────── day-of-month (1–31)
//	│ │ │ │ ┌─────── month        (1–12 or JAN–DEC)
//	│ │ │ │ │ ┌───── day-of-week  (0–6 or SUN–SAT)
//	│ │ │ │ │ │
//	* * * * * *
//
// Predefined schedules: @yearly, @monthly, @weekly, @daily, @hourly.
package cron

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	gocron "github.com/robfig/cron/v3"
)

// Job is implemented by types that can be run on a schedule.
// The supplied ctx is cancelled when the scheduler shuts down, giving
// long-running jobs a clean way to detect the shutdown signal.
type Job interface {
	Run(ctx context.Context)
}

// JobFunc is a function adapter for the Job interface.
type JobFunc func(ctx context.Context)

// Run implements Job.
func (f JobFunc) Run(ctx context.Context) { f(ctx) }

// Entry holds metadata about a registered job.
type Entry struct {
	// Name is the human-readable identifier supplied at registration time.
	Name string
	// ID is the opaque internal entry ID (useful for Remove).
	ID gocron.EntryID
	// Next is the next scheduled run time.
	Next time.Time
	// Prev is the most recent run time (zero if never run).
	Prev time.Time
}

// Scheduler manages periodic job execution.
//
// All methods are safe for concurrent use.
type Scheduler struct {
	c      *gocron.Cron
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.RWMutex
	names  map[gocron.EntryID]string
}

// NewScheduler creates a Scheduler with:
//   - Second-resolution cron parsing (6-field expressions)
//   - Automatic panic recovery per job (a panicking job does not stop the scheduler)
//
// Call Start to begin execution.
func NewScheduler() *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Scheduler{
		ctx:    ctx,
		cancel: cancel,
		names:  make(map[gocron.EntryID]string),
	}
	s.c = gocron.New(
		gocron.WithSeconds(),
		gocron.WithLogger(gocron.DefaultLogger),
		gocron.WithChain(
			// Recover from job panics and log them instead of crashing the scheduler.
			gocron.Recover(gocron.DefaultLogger),
		),
	)
	return s
}

// Every schedules job to run at the given interval starting from the first
// tick after Start is called.
//
//	s.Every(15*time.Minute, "metrics-flush", flushMetrics)
func (s *Scheduler) Every(d time.Duration, name string, job Job) error {
	if d <= 0 {
		return fmt.Errorf("cron: interval must be positive, got %v", d)
	}
	spec := fmt.Sprintf("@every %s", d)
	return s.register(spec, name, job)
}

// Cron schedules job using a cron expression.
//
// The scheduler uses 6-field (second-resolution) expressions by default.
// Standard 5-field expressions work too — the parser treats a 5-field
// expression as if the seconds field is "0".
//
//	s.Cron("0 0 * * * *",  "hourly",  job)   // every hour on the hour
//	s.Cron("*/15 * * * *", "quarter", job)   // every 15 minutes
//	s.Cron("@daily",       "nightly", job)   // at midnight
func (s *Scheduler) Cron(expr, name string, job Job) error {
	return s.register(expr, name, job)
}

func (s *Scheduler) register(spec, name string, job Job) error {
	ctx := s.ctx
	id, err := s.c.AddFunc(spec, func() {
		if ctx.Err() != nil {
			return // scheduler is shutting down; skip this tick
		}
		job.Run(ctx)
	})
	if err != nil {
		return fmt.Errorf("cron: register %q (%s): %w", name, spec, err)
	}
	s.mu.Lock()
	s.names[id] = name
	s.mu.Unlock()
	return nil
}

// Remove unregisters the job with the given Entry.ID.
// The running instance of the job (if any) is not interrupted.
func (s *Scheduler) Remove(id gocron.EntryID) {
	s.c.Remove(id)
	s.mu.Lock()
	delete(s.names, id)
	s.mu.Unlock()
}

// Start begins executing registered jobs. Non-blocking — returns immediately
// and runs jobs in background goroutines.
func (s *Scheduler) Start() {
	s.c.Start()
	slog.Info("cron: scheduler started", slog.Int("jobs", len(s.c.Entries())))
}

// Entries returns a snapshot of all registered jobs and their next/prev run times.
func (s *Scheduler) Entries() []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	raw := s.c.Entries()
	out := make([]Entry, len(raw))
	for i, e := range raw {
		out[i] = Entry{
			Name: s.names[e.ID],
			ID:   e.ID,
			Next: e.Next,
			Prev: e.Prev,
		}
	}
	return out
}

// Shutdown stops the scheduler gracefully.
// It signals all running jobs via context cancellation, then waits for them
// to finish or for ctx to expire, whichever comes first.
//
// Typical usage with App.OnStop:
//
//	app.OnStop(scheduler.Shutdown)
func (s *Scheduler) Shutdown(ctx context.Context) error {
	s.cancel() // signal jobs: scheduler is stopping

	// s.c.Stop() returns a context that is Done when all running jobs have finished.
	stopCtx := s.c.Stop()
	select {
	case <-stopCtx.Done():
		slog.Info("cron: scheduler stopped cleanly")
		return nil
	case <-ctx.Done():
		return fmt.Errorf("cron: shutdown timeout: %w", ctx.Err())
	}
}
