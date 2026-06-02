//go:build gocron
// +build gocron

package runner

// This file provides the gocron backend, enabled with build tag "gocron".

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	gocronv2 "github.com/go-co-op/gocron/v2"
)

// GocronOption configures the GocronRunner.
type GocronOption func(*gocronOptions)

type gocronOptions struct {
	schedulerOpts []gocronv2.SchedulerOption
}

// WithGocronLocker enables distributed locking to prevent duplicate job execution
// across multiple service instances. Requires a Locker implementation such as
// github.com/go-co-op/gocron-redis-lock/v2.
//
//	locker, _ := gocronredislock.NewRedisLocker(redisClient)
//	r, err := runner.NewGocronRunner(runner.WithGocronLocker(locker))
func WithGocronLocker(locker gocronv2.Locker) GocronOption {
	return func(o *gocronOptions) {
		o.schedulerOpts = append(o.schedulerOpts, gocronv2.WithDistributedLocker(locker))
	}
}

// WithGocronLocation sets the timezone for cron expression evaluation. Default: time.Local.
func WithGocronLocation(loc *time.Location) GocronOption {
	return func(o *gocronOptions) {
		o.schedulerOpts = append(o.schedulerOpts, gocronv2.WithLocation(loc))
	}
}

// GocronRunner is a Runner backed by go-co-op/gocron/v2.
// All methods are safe for concurrent use.
type GocronRunner struct {
	s      gocronv2.Scheduler
	mu     sync.RWMutex
	jobs   map[string]gocronv2.Job // name → Job (for Jobs() metadata)
	cancel context.CancelFunc
}

// NewGocronRunner creates a gocron-backed Runner with the given options.
func NewGocronRunner(opts ...GocronOption) (*GocronRunner, error) {
	o := &gocronOptions{}
	for _, opt := range opts {
		opt(o)
	}
	// Bridge gocron's Logger interface to slog so job errors surface through
	// the application's existing logging stack.
	o.schedulerOpts = append(o.schedulerOpts, gocronv2.WithLogger(slog.Default()))

	s, err := gocronv2.NewScheduler(o.schedulerOpts...)
	if err != nil {
		return nil, fmt.Errorf("runner: new scheduler: %w", err)
	}
	return &GocronRunner{
		s:    s,
		jobs: make(map[string]gocronv2.Job),
	}, nil
}

// Add schedules job using a cron expression.
//
// 5-field ("0 9 * * *") and 6-field ("0 0 9 * * *") expressions are both
// accepted. Named schedules (@daily, @every 5m) are supported too.
//
// Returns an error if a job with the same name is already registered.
func (r *GocronRunner) Add(name, expr string, job JobFunc) error {
	r.mu.Lock()
	if _, exists := r.jobs[name]; exists {
		r.mu.Unlock()
		return fmt.Errorf("runner: job %q already registered", name)
	}
	r.mu.Unlock()

	// gocron v2 passes the job's context to the function automatically when
	// the first parameter is context.Context. The context is cancelled when
	// the scheduler shuts down, giving jobs a clean signal to stop.
	j, err := r.s.NewJob(
		gocronv2.CronJob(expr, isGocronSixField(expr)),
		gocronv2.NewTask(func(ctx context.Context) {
			if err := job(ctx); err != nil {
				slog.Error("runner: job error", "name", name, "err", err)
			}
		}),
		gocronv2.WithName(name),
	)
	if err != nil {
		return fmt.Errorf("runner: add %q: %w", name, err)
	}

	r.mu.Lock()
	r.jobs[name] = j
	r.mu.Unlock()
	return nil
}

// Every schedules job at a fixed interval.
// Returns an error if a job with the same name is already registered.
func (r *GocronRunner) Every(name string, d time.Duration, job JobFunc) error {
	r.mu.Lock()
	if _, exists := r.jobs[name]; exists {
		r.mu.Unlock()
		return fmt.Errorf("runner: job %q already registered", name)
	}
	r.mu.Unlock()

	j, err := r.s.NewJob(
		gocronv2.DurationJob(d),
		gocronv2.NewTask(func(ctx context.Context) {
			if err := job(ctx); err != nil {
				slog.Error("runner: job error", "name", name, "err", err)
			}
		}),
		gocronv2.WithName(name),
	)
	if err != nil {
		return fmt.Errorf("runner: every %q: %w", name, err)
	}

	r.mu.Lock()
	r.jobs[name] = j
	r.mu.Unlock()
	return nil
}

// Start begins executing registered jobs. Non-blocking.
// Cancelling ctx triggers graceful shutdown of the scheduler.
func (r *GocronRunner) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)

	r.mu.Lock()
	r.cancel = cancel
	r.mu.Unlock()

	r.s.Start()

	// Mirror ctx cancellation to the scheduler so that context-based app
	// lifecycle management works: cancel(ctx) → scheduler shuts down.
	go func() {
		<-ctx.Done()
		_ = r.s.Shutdown()
	}()
	return nil
}

// Stop gracefully shuts down the scheduler.
// Blocks until all running jobs complete or ctx expires.
func (r *GocronRunner) Stop(ctx context.Context) error {
	r.mu.Lock()
	if r.cancel != nil {
		r.cancel()
	}
	r.mu.Unlock()
	done := make(chan error, 1)
	go func() { done <- r.s.Shutdown() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Jobs returns a snapshot of all registered jobs and their timing metadata.
func (r *GocronRunner) Jobs() []JobInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]JobInfo, 0, len(r.jobs))
	for _, j := range r.jobs {
		next, _ := j.NextRun()
		prev, _ := j.LastRun()
		out = append(out, JobInfo{
			Name: j.Name(),
			Next: next,
			Prev: prev,
		})
	}
	return out
}

// isGocronSixField reports whether expr is a 6-field cron expression (includes
// a leading seconds field). Named expressions starting with '@' and standard
// 5-field expressions return false.
func isGocronSixField(expr string) bool {
	expr = strings.TrimSpace(expr)
	if strings.HasPrefix(expr, "@") {
		return false
	}
	return len(strings.Fields(expr)) == 6
}

// Verify GocronRunner implements Runner at compile time.
var _ Runner = (*GocronRunner)(nil)
