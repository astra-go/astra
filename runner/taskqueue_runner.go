//go:build tqrunner
// +build tqrunner

package runner

// This file provides the taskqueue backend, enabled with build tag "tqrunner".

import (
	"context"
	"fmt"
	"sync"
	"time"

	tq "github.com/astra-go/astra/taskqueue"
)

// TaskqueueRunnerUniqueWindow is the dedup window applied to every registered job.
// Within this window, at most one task per job will be enqueued, even when
// multiple server instances run the same RegisterCron schedule.
const TaskqueueRunnerUniqueWindow = 23*time.Hour + 59*time.Minute

// TaskqueueRunner is a distributed Runner backed by the Astra task queue.
// All methods are safe for concurrent use.
type TaskqueueRunner struct {
	server  *tq.Server
	mux     *tq.ServeMux
	mu      sync.RWMutex
	jobs    []JobInfo
	started bool
}

// NewTaskqueueRunner creates a taskqueue-backed Runner.
// cfg.Broker must not be set; pass the broker as the first argument.
func NewTaskqueueRunner(broker tq.Broker, cfg tq.ServerConfig) *TaskqueueRunner {
	cfg.Broker = broker
	mux := tq.NewServeMux()
	return &TaskqueueRunner{
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
func (r *TaskqueueRunner) Add(name, expr string, job JobFunc) error {
	r.mu.Lock()
	for _, j := range r.jobs {
		if j.Name == name {
			r.mu.Unlock()
			return fmt.Errorf("runner: job %q already registered", name)
		}
	}
	r.jobs = append(r.jobs, JobInfo{Name: name, Expr: expr})
	r.mu.Unlock()

	r.mux.HandleFunc(name, func(ctx context.Context, _ *tq.Task) error {
		return job(ctx)
	})

	return r.server.RegisterCron(expr, name, nil,
		tq.WithUnique(name, TaskqueueRunnerUniqueWindow),
	)
}

// Every schedules a distributed task at a fixed interval.
func (r *TaskqueueRunner) Every(name string, d time.Duration, job JobFunc) error {
	return r.Add(name, fmt.Sprintf("@every %s", d), job)
}

// Start launches the worker pool and cron scheduler in a background goroutine.
// Returns immediately.
func (r *TaskqueueRunner) Start(ctx context.Context) error {
	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return fmt.Errorf("runner: already started")
	}
	r.started = true
	r.mu.Unlock()

	go func() { _ = r.server.Run(ctx, r.mux) }()
	return nil
}

// Stop gracefully shuts down the worker pool.
// In-flight tasks are allowed to complete up to ServerConfig.ShutdownTimeout.
func (r *TaskqueueRunner) Stop(_ context.Context) error {
	r.server.Stop()
	return nil
}

// Jobs returns a snapshot of all registered jobs.
// Next/Prev times are not tracked by the taskqueue backend and will be zero.
func (r *TaskqueueRunner) Jobs() []JobInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]JobInfo, len(r.jobs))
	copy(out, r.jobs)
	return out
}

// Verify TaskqueueRunner implements Runner at compile time.
var _ Runner = (*TaskqueueRunner)(nil)
