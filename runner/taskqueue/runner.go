// Package taskqueue provides a distributed Runner backed by the Astra task queue.
//
// Jobs registered with Add/Every are:
//  1. Enqueued as tasks at each cron tick via RegisterCron.
//  2. Executed by the embedded worker pool (taskqueue.Server).
//
// Compared to the cron/gocron backends, this gives you:
//   - Persistence: tasks survive service restarts.
//   - Retries: automatic exponential-backoff retry on handler failure.
//   - Horizontal scaling: multiple instances share work without double-execution.
//     (A 24-hour unique window prevents duplicate enqueues from concurrent servers.)
//
// # Usage
//
//	import (
//	    tqrunner "github.com/astra-go/astra/runner/taskqueue"
//	    tqredis  "github.com/astra-go/astra/taskqueue/redis"
//	    "github.com/astra-go/astra/taskqueue"
//	)
//
//	broker, _ := tqredis.New(tqredis.Config{Addr: "localhost:6379"})
//	r := tqrunner.New(broker, taskqueue.ServerConfig{Concurrency: 5})
//	r.Add("report:daily", "0 9 * * *", func(ctx context.Context) error {
//	    return generateReport(ctx)
//	})
//	r.Start(ctx)
//	defer r.Stop(context.Background())
package taskqueue

import (
	"context"
	"fmt"
	"sync"
	"time"

	tq "github.com/astra-go/astra/taskqueue"

	"github.com/astra-go/astra/runner"
)

// defaultUniqueWindow is the dedup window applied to every registered job.
// Within this window, at most one task per job will be enqueued, even when
// multiple server instances run the same RegisterCron schedule.
const defaultUniqueWindow = 23*time.Hour + 59*time.Minute

// Runner is a distributed Runner backed by the Astra task queue.
// All methods are safe for concurrent use.
type Runner struct {
	server  *tq.Server
	mux     *tq.ServeMux
	mu      sync.RWMutex
	jobs    []runner.JobInfo
	started bool
}

// New creates a taskqueue-backed Runner.
// cfg.Broker must not be set; pass the broker as the first argument.
func New(broker tq.Broker, cfg tq.ServerConfig) *Runner {
	cfg.Broker = broker
	mux := tq.NewServeMux()
	return &Runner{
		server: tq.NewServer(cfg),
		mux:    mux,
	}
}

// Add schedules a distributed task triggered by a cron expression.
//
// Internally this:
//  1. Registers fn as a handler in the embedded ServeMux.
//  2. Calls server.RegisterCron to enqueue a task at each cron tick.
//  3. Applies a 24-hour WithUnique window so multiple instances never
//     enqueue the same job twice in the same scheduling cycle.
func (r *Runner) Add(name, expr string, job runner.JobFunc) error {
	r.mu.Lock()
	for _, j := range r.jobs {
		if j.Name == name {
			r.mu.Unlock()
			return fmt.Errorf("runner/taskqueue: job %q already registered", name)
		}
	}
	r.jobs = append(r.jobs, runner.JobInfo{Name: name, Expr: expr})
	r.mu.Unlock()

	r.mux.HandleFunc(name, func(ctx context.Context, _ *tq.Task) error {
		return job(ctx)
	})

	return r.server.RegisterCron(expr, name, nil,
		tq.WithUnique(name, defaultUniqueWindow),
	)
}

// Every schedules a distributed task at a fixed interval.
func (r *Runner) Every(name string, d time.Duration, job runner.JobFunc) error {
	return r.Add(name, fmt.Sprintf("@every %s", d), job)
}

// Start launches the worker pool and cron scheduler in a background goroutine.
// Returns immediately.
func (r *Runner) Start(ctx context.Context) error {
	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return fmt.Errorf("runner/taskqueue: already started")
	}
	r.started = true
	r.mu.Unlock()

	go func() { _ = r.server.Run(ctx, r.mux) }()
	return nil
}

// Stop gracefully shuts down the worker pool.
// In-flight tasks are allowed to complete up to ServerConfig.ShutdownTimeout.
func (r *Runner) Stop(_ context.Context) error {
	r.server.Stop()
	return nil
}

// Jobs returns a snapshot of all registered jobs.
// Next/Prev times are not tracked by the taskqueue backend and will be zero.
func (r *Runner) Jobs() []runner.JobInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]runner.JobInfo, len(r.jobs))
	copy(out, r.jobs)
	return out
}

// Verify Runner implements runner.Runner at compile time.
var _ runner.Runner = (*Runner)(nil)
