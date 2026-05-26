// Package runner provides a unified interface for task scheduling across
// multiple backends. Pick a backend based on your infrastructure:
//
//	runner/cron      — in-process cron (wraps robfig/cron/v3 via astra/cron)
//	runner/gocron    — in-process cron with optional distributed locking (go-co-op/gocron/v2)
//	runner/taskqueue — distributed, persistent task queue (any Astra Broker)
//	runner/dagu      — DAG-based workflow orchestrator (Dagu REST API + HTTP callbacks)
//
// # Quick start (cron backend)
//
//	import cronrunner "github.com/astra-go/astra/runner/cron"
//
//	r := cronrunner.New()
//	r.Add("cleanup", "0 2 * * *", func(ctx context.Context) error {
//	    return cleanupExpiredSessions(ctx)
//	})
//	r.Every("heartbeat", time.Minute, func(ctx context.Context) error {
//	    return pingUpstream(ctx)
//	})
//	r.Start(ctx)
//	defer r.Stop(context.Background())
//
// # Switching backends
//
// All backends implement Runner, so switching is a one-line change:
//
//	// in-process cron
//	var r runner.Runner = cronrunner.New()
//
//	// distributed task queue (persistent, retryable, horizontally scalable)
//	var r runner.Runner = tqrunner.New(redisBroker, taskqueue.ServerConfig{})
//
//	// Dagu-managed DAGs with full web UI and history
//	var r runner.Runner, _ = dagurunner.New(dagurunner.Config{...})
//
// # Cron expression format
//
// All backends accept 5-field, 6-field (second resolution), named, and @every
// expressions:
//
//	"0 9 * * *"      — every day at 09:00 (5-field)
//	"0 0 9 * * *"    — every day at 09:00:00 (6-field, second resolution)
//	"@daily"         — every day at midnight
//	"@every 5m"      — every 5 minutes (also usable with Every())
package runner

import (
	"context"
	"time"
)

// Runner is the unified task scheduling interface.
// All methods are safe for concurrent use unless documented otherwise.
type Runner interface {
	// Add schedules job using a cron expression.
	//
	// Supports:
	//   - 5-field standard cron:  "0 9 * * *"
	//   - 6-field with seconds:   "0 0 9 * * *"
	//   - Named schedules:        @yearly @monthly @weekly @daily @hourly
	//   - Interval shorthand:     "@every 5m30s"
	//
	// Returns an error if a job with the same name is already registered.
	Add(name, expr string, job JobFunc) error

	// Every schedules job to run at the given interval.
	// The first tick fires after d has elapsed from the time Start is called.
	//
	// Returns an error if a job with the same name is already registered.
	Every(name string, d time.Duration, job JobFunc) error

	// Start begins job execution. Non-blocking — returns immediately.
	// ctx is propagated to running jobs; cancelling it triggers a graceful shutdown.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the runner.
	// Blocks until all in-flight jobs finish or ctx expires.
	Stop(ctx context.Context) error

	// Jobs returns a point-in-time snapshot of all registered jobs.
	Jobs() []JobInfo
}

// JobFunc is a function that can be scheduled by a Runner.
// A non-nil error is logged by the runner but does not affect future scheduling.
type JobFunc func(ctx context.Context) error

// JobInfo holds read-only metadata about a scheduled job.
type JobInfo struct {
	// Name is the human-readable identifier supplied at registration.
	Name string

	// Expr is the schedule expression ("0 9 * * *", "@every 5m", etc.).
	// Empty string means the job was registered via Every() without conversion.
	Expr string

	// Next is the next scheduled execution time.
	// Zero value means the runner has not started yet.
	Next time.Time

	// Prev is the most recent execution time.
	// Zero value means the job has never run.
	Prev time.Time
}
