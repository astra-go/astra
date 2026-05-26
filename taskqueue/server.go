package taskqueue

import (
	"context"
	"log/slog"
	"math"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// ServerConfig configures a Server.
type ServerConfig struct {
	// Broker is the storage backend. Required.
	Broker Broker

	// Queues maps queue name → priority weight.
	// Workers poll queues in weighted order.
	// Default: {"default": 1}
	//
	// Example:
	//   map[string]int{"critical": 6, "default": 3, "low": 1}
	Queues map[string]int

	// Concurrency is the number of parallel worker goroutines. Default: 10.
	Concurrency int

	// ShutdownTimeout is how long Run waits for in-flight workers to finish
	// after Stop is called. Default: 30s.
	ShutdownTimeout time.Duration

	// ScheduleInterval is how often the scheduler promotes scheduled/retry tasks
	// to pending. Default: 5s.
	ScheduleInterval time.Duration

	// ReaperInterval is how often the reaper recovers stale active tasks.
	// Default: 60s.
	ReaperInterval time.Duration

	// Logger overrides the default slog.Default() logger.
	Logger *slog.Logger
}

func (c *ServerConfig) setDefaults() {
	if len(c.Queues) == 0 {
		c.Queues = map[string]int{DefaultQueue: 1}
	}
	if c.Concurrency <= 0 {
		c.Concurrency = 10
	}
	if c.ShutdownTimeout <= 0 {
		c.ShutdownTimeout = 30 * time.Second
	}
	if c.ScheduleInterval <= 0 {
		c.ScheduleInterval = 5 * time.Second
	}
	if c.ReaperInterval <= 0 {
		c.ReaperInterval = 60 * time.Second
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
}

// buildQueueList expands a weighted queue map into an ordered polling slice.
// e.g. {"critical":3,"default":1} → ["critical","critical","critical","default"]
func buildQueueList(queues map[string]int) []string {
	var list []string
	for name, weight := range queues {
		for i := 0; i < weight; i++ {
			list = append(list, name)
		}
	}
	return list
}

// retryDelay returns the backoff duration for the nth retry (0-indexed),
// using exponential backoff capped at 1 hour, with ±10% jitter.
func retryDelay(retried int) time.Duration {
	base := 10 * time.Second * time.Duration(math.Pow(2, float64(retried)))
	if base > time.Hour {
		base = time.Hour
	}
	// ±10% jitter
	jitter := time.Duration(float64(base) * 0.1 * (rand.Float64()*2 - 1))
	return base + jitter
}

// Server runs background workers that process tasks from the broker.
type Server struct {
	cfg    ServerConfig
	queues []string
	client *Client

	stopCh  chan struct{}
	wg      sync.WaitGroup
	cronSvc *cron.Cron
}

// NewServer creates a new Server with the given configuration.
func NewServer(cfg ServerConfig) *Server {
	cfg.setDefaults()
	return &Server{
		cfg:    cfg,
		queues: buildQueueList(cfg.Queues),
		client: NewClient(cfg.Broker),
		stopCh: make(chan struct{}),
	}
}

// Run starts the worker pool, scheduler, and reaper. It blocks until ctx is
// cancelled or Stop is called. After returning, all goroutines have exited.
func (s *Server) Run(ctx context.Context, mux *ServeMux) error {
	// Start Cron if any tasks were registered.
	if s.cronSvc != nil {
		s.cronSvc.Start()
	}

	// Launch worker goroutines.
	for i := 0; i < s.cfg.Concurrency; i++ {
		s.wg.Add(1)
		go s.runWorker(ctx, mux)
	}

	// Launch scheduler goroutine.
	s.wg.Add(1)
	go s.runScheduler(ctx)

	// Launch reaper goroutine.
	s.wg.Add(1)
	go s.runReaper(ctx)

	// Wait for context cancellation or Stop().
	select {
	case <-ctx.Done():
	case <-s.stopCh:
	}

	// Stop cron scheduler.
	if s.cronSvc != nil {
		stopCtx := s.cronSvc.Stop()
		<-stopCtx.Done()
	}

	// Wait for all goroutines to exit, with a shutdown timeout.
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(s.cfg.ShutdownTimeout):
		s.cfg.Logger.Warn("taskqueue: shutdown timeout exceeded, some workers may still be running")
	}

	return nil
}

// Stop signals the server to stop accepting new tasks and allows in-flight
// tasks to finish (up to ShutdownTimeout).
func (s *Server) Stop() {
	select {
	case <-s.stopCh:
		// already stopped
	default:
		close(s.stopCh)
	}
}

// RegisterCron registers a recurring task using a cron expression.
// expr follows standard 5-field cron syntax (minute hour day month weekday).
//
// Multiple server instances can use WithUnique to prevent duplicate execution:
//
//	srv.RegisterCron("0 9 * * *", "report:daily", nil,
//	    taskqueue.WithUnique("report:daily", 23*time.Hour),
//	)
func (s *Server) RegisterCron(expr string, taskType string, payload []byte, opts ...TaskOption) error {
	if s.cronSvc == nil {
		s.cronSvc = cron.New()
	}
	_, err := s.cronSvc.AddFunc(expr, func() {
		ctx := context.Background()
		if _, err := s.client.EnqueueTask(ctx, taskType, payload, opts...); err != nil {
			if err != ErrDuplicateTask {
				s.cfg.Logger.Warn("taskqueue: cron enqueue failed",
					slog.String("type", taskType),
					slog.String("err", err.Error()),
				)
			}
		}
	})
	return err
}

// ─── internal goroutines ──────────────────────────────────────────────────────

func (s *Server) runWorker(ctx context.Context, mux *ServeMux) {
	defer s.wg.Done()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		default:
		}

		// The deadline is a generous upper bound used by the reaper to detect
		// crashed workers. We use DefaultTimeout as the per-task ceiling.
		deadline := time.Now().Add(DefaultTimeout)
		task, err := s.cfg.Broker.Dequeue(ctx, s.queues, deadline)
		if err != nil {
			if err == ErrNoTask {
				// No work available — back off briefly.
				select {
				case <-time.After(200 * time.Millisecond):
				case <-s.stopCh:
					return
				case <-ctx.Done():
					return
				}
				continue
			}
			if ctx.Err() != nil {
				return
			}
			s.cfg.Logger.Error("taskqueue: dequeue error", slog.String("err", err.Error()))
			time.Sleep(time.Second)
			continue
		}

		s.processTask(ctx, mux, task)
	}
}

func (s *Server) processTask(ctx context.Context, mux *ServeMux, task *Task) {
	timeout := task.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	tCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	err := mux.ProcessTask(tCtx, task)
	if err == nil {
		if ackErr := s.cfg.Broker.Ack(tCtx, task); ackErr != nil {
			s.cfg.Logger.Error("taskqueue: ack error",
				slog.String("task_id", task.ID),
				slog.String("err", ackErr.Error()),
			)
		}
		return
	}

	// Handler returned an error — decide retry or dead.
	errStr := err.Error()
	var retryAt time.Time
	if task.Retried < task.MaxRetries {
		retryAt = time.Now().Add(retryDelay(task.Retried))
		task.Retried++
		s.cfg.Logger.Warn("taskqueue: task failed, will retry",
			slog.String("task_id", task.ID),
			slog.String("type", task.Type),
			slog.Int("retried", task.Retried),
			slog.Time("retry_at", retryAt),
			slog.String("err", errStr),
		)
	} else {
		s.cfg.Logger.Error("taskqueue: task dead (max retries exceeded)",
			slog.String("task_id", task.ID),
			slog.String("type", task.Type),
			slog.String("err", errStr),
		)
	}

	if nackErr := s.cfg.Broker.Nack(ctx, task, errStr, retryAt); nackErr != nil {
		s.cfg.Logger.Error("taskqueue: nack error",
			slog.String("task_id", task.ID),
			slog.String("err", nackErr.Error()),
		)
	}
}

func (s *Server) runScheduler(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(s.cfg.ScheduleInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := s.cfg.Broker.Schedule(ctx); err != nil && ctx.Err() == nil {
				s.cfg.Logger.Error("taskqueue: schedule error", slog.String("err", err.Error()))
			}
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (s *Server) runReaper(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(s.cfg.ReaperInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := s.cfg.Broker.ReapStale(ctx); err != nil && ctx.Err() == nil {
				s.cfg.Logger.Error("taskqueue: reap error", slog.String("err", err.Error()))
			}
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}
