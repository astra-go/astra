// Package gocron provides a Runner backed by go-co-op/gocron/v2.
//
// Compared to runner/cron (robfig/cron), this backend offers:
//   - Built-in distributed locking to prevent duplicate execution across
//     multiple instances (via WithLocker and e.g. gocron-redis-lock).
//   - Richer scheduling types (DurationRandom, Daily, Weekly, Monthly).
//   - Job-level event listeners for metrics / tracing.
//
// # Basic usage
//
//	r, err := gocron.New()
//	r.Add("report", "0 9 * * *", func(ctx context.Context) error {
//	    return generateDailyReport(ctx)
//	})
//	r.Start(ctx)
//	defer r.Stop(context.Background())
//
// # Distributed locking (multi-instance safe)
//
//	import gocronredislock "github.com/go-co-op/gocron-redis-lock/v2"
//
//	locker, _ := gocronredislock.NewRedisLocker(redisClient,
//	    gocronredislock.WithTryLockTimeout(time.Second),
//	)
//	r, err := gocron.New(gocron.WithLocker(locker))
//	// Only one instance will execute each job at a time across your fleet.
package gocron

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	gocronv2 "github.com/go-co-op/gocron/v2"

	"github.com/astra-go/astra/runner"
)

// Option configures the gocron Runner.
type Option func(*options)

type options struct {
	schedulerOpts []gocronv2.SchedulerOption
}

// WithLocker enables distributed locking to prevent duplicate job execution
// across multiple service instances. Requires a Locker implementation such as
// github.com/go-co-op/gocron-redis-lock/v2.
//
//	locker, _ := gocronredislock.NewRedisLocker(redisClient)
//	r, err := gocron.New(gocron.WithLocker(locker))
func WithLocker(locker gocronv2.Locker) Option {
	return func(o *options) {
		o.schedulerOpts = append(o.schedulerOpts, gocronv2.WithDistributedLocker(locker))
	}
}

// WithLocation sets the timezone for cron expression evaluation. Default: time.Local.
func WithLocation(loc *time.Location) Option {
	return func(o *options) {
		o.schedulerOpts = append(o.schedulerOpts, gocronv2.WithLocation(loc))
	}
}

// Runner is a Runner backed by go-co-op/gocron/v2.
// All methods are safe for concurrent use.
type Runner struct {
	s      gocronv2.Scheduler
	mu     sync.RWMutex
	jobs   map[string]gocronv2.Job // name → Job (for Jobs() metadata)
	cancel context.CancelFunc
}

// New creates a gocron-backed Runner with the given options.
func New(opts ...Option) (*Runner, error) {
	o := &options{}
	for _, opt := range opts {
		opt(o)
	}
	// Bridge gocron's Logger interface to slog so job errors surface through
	// the application's existing logging stack.
	o.schedulerOpts = append(o.schedulerOpts, gocronv2.WithLogger(slog.Default()))

	s, err := gocronv2.NewScheduler(o.schedulerOpts...)
	if err != nil {
		return nil, fmt.Errorf("runner/gocron: new scheduler: %w", err)
	}
	return &Runner{
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
func (r *Runner) Add(name, expr string, job runner.JobFunc) error {
	r.mu.Lock()
	if _, exists := r.jobs[name]; exists {
		r.mu.Unlock()
		return fmt.Errorf("runner/gocron: job %q already registered", name)
	}
	r.mu.Unlock()

	// gocron v2 passes the job's context to the function automatically when
	// the first parameter is context.Context. The context is cancelled when
	// the scheduler shuts down, giving jobs a clean signal to stop.
	j, err := r.s.NewJob(
		gocronv2.CronJob(expr, isSixField(expr)),
		gocronv2.NewTask(func(ctx context.Context) {
			if err := job(ctx); err != nil {
				slog.Error("runner/gocron: job error", "name", name, "err", err)
			}
		}),
		gocronv2.WithName(name),
	)
	if err != nil {
		return fmt.Errorf("runner/gocron: add %q: %w", name, err)
	}

	r.mu.Lock()
	r.jobs[name] = j
	r.mu.Unlock()
	return nil
}

// Every schedules job at a fixed interval.
// Returns an error if a job with the same name is already registered.
func (r *Runner) Every(name string, d time.Duration, job runner.JobFunc) error {
	r.mu.Lock()
	if _, exists := r.jobs[name]; exists {
		r.mu.Unlock()
		return fmt.Errorf("runner/gocron: job %q already registered", name)
	}
	r.mu.Unlock()

	j, err := r.s.NewJob(
		gocronv2.DurationJob(d),
		gocronv2.NewTask(func(ctx context.Context) {
			if err := job(ctx); err != nil {
				slog.Error("runner/gocron: job error", "name", name, "err", err)
			}
		}),
		gocronv2.WithName(name),
	)
	if err != nil {
		return fmt.Errorf("runner/gocron: every %q: %w", name, err)
	}

	r.mu.Lock()
	r.jobs[name] = j
	r.mu.Unlock()
	return nil
}

// Start begins executing registered jobs. Non-blocking.
// Cancelling ctx triggers graceful shutdown of the scheduler.
func (r *Runner) Start(ctx context.Context) error {
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
func (r *Runner) Stop(ctx context.Context) error {
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
func (r *Runner) Jobs() []runner.JobInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]runner.JobInfo, 0, len(r.jobs))
	for _, j := range r.jobs {
		next, _ := j.NextRun()
		prev, _ := j.LastRun()
		out = append(out, runner.JobInfo{
			Name: j.Name(),
			Next: next,
			Prev: prev,
		})
	}
	return out
}

// isSixField reports whether expr is a 6-field cron expression (includes
// a leading seconds field). Named expressions starting with '@' and standard
// 5-field expressions return false.
func isSixField(expr string) bool {
	expr = strings.TrimSpace(expr)
	if strings.HasPrefix(expr, "@") {
		return false
	}
	return len(strings.Fields(expr)) == 6
}

// Verify Runner implements runner.Runner at compile time.
var _ runner.Runner = (*Runner)(nil)
