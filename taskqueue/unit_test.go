package taskqueue

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════
// option.go — Task construction + option composition
// ══════════════════════════════════════════════════════════════════════════

func TestNewTask_Defaults(t *testing.T) {
	task := NewTask("email:welcome", []byte("hello"))

	if task.ID == "" {
		t.Error("NewTask ID must not be empty")
	}
	if task.Type != "email:welcome" {
		t.Errorf("Type = %q, want %q", task.Type, "email:welcome")
	}
	if string(task.Payload) != "hello" {
		t.Errorf("Payload = %q, want %q", string(task.Payload), "hello")
	}
	if task.Queue != DefaultQueue {
		t.Errorf("Queue = %q, want %q", task.Queue, DefaultQueue)
	}
	if task.State != StatePending {
		t.Errorf("State = %q, want %q", task.State, StatePending)
	}
	if task.MaxRetries != DefaultMaxRetries {
		t.Errorf("MaxRetries = %d, want %d", task.MaxRetries, DefaultMaxRetries)
	}
	if task.Timeout != DefaultTimeout {
		t.Errorf("Timeout = %v, want %v", task.Timeout, DefaultTimeout)
	}
	if task.ProcessAt.IsZero() {
		t.Error("ProcessAt must not be zero (defaults to now)")
	}
	if task.CreatedAt.IsZero() || task.UpdatedAt.IsZero() {
		t.Error("CreatedAt/UpdatedAt must not be zero")
	}
	if task.Retried != 0 {
		t.Errorf("Retried = %d, want 0", task.Retried)
	}
}

func TestNewTask_WithOptions(t *testing.T) {
	at := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	task := NewTask("report:generate", []byte("data"),
		WithQueue("critical"),
		WithMaxRetries(5),
		WithTimeout(10*time.Minute),
		WithProcessAt(at),
		WithUnique("my-key", 1*time.Hour),
	)

	if task.Queue != "critical" {
		t.Errorf("Queue = %q, want critical", task.Queue)
	}
	if task.MaxRetries != 5 {
		t.Errorf("MaxRetries = %d, want 5", task.MaxRetries)
	}
	if task.Timeout != 10*time.Minute {
		t.Errorf("Timeout = %v, want 10m", task.Timeout)
	}
	if task.ProcessAt != at {
		t.Errorf("ProcessAt = %v, want %v", task.ProcessAt, at)
	}
	if task.UniqueKey == "" {
		t.Error("UniqueKey must not be empty when WithUnique is used")
	}
	if task.UniqueFor != 1*time.Hour {
		t.Errorf("UniqueFor = %v, want 1h", task.UniqueFor)
	}
}

func TestWithProcessIn(t *testing.T) {
	before := time.Now()
	task := NewTask("test", nil, WithProcessIn(5*time.Second))
	after := time.Now()

	if task.ProcessAt.Before(before) || task.ProcessAt.After(after.Add(5*time.Second)) {
		t.Errorf("ProcessAt = %v, want ~now+5s (between %v and %v)", task.ProcessAt, before, after.Add(5*time.Second))
	}
}

func TestWithUnique_ContentAddressed(t *testing.T) {
	// Same payload → same dedup key.
	a := NewTask("my-task", []byte("same"), WithUnique("", 10*time.Minute))
	b := NewTask("my-task", []byte("same"), WithUnique("", 10*time.Minute))
	c := NewTask("my-task", []byte("different"), WithUnique("", 10*time.Minute))

	if a.UniqueKey != b.UniqueKey {
		t.Error("same payload should produce same dedup key")
	}
	if a.UniqueKey == c.UniqueKey {
		t.Error("different payload should produce different dedup key")
	}
}

func TestOptionOrder_LastWins(t *testing.T) {
	// ProcessAt vs ProcessIn: last applied wins.
	at := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	task := NewTask("test", nil,
		WithProcessIn(1*time.Minute),
		WithProcessAt(at),
	)
	if task.ProcessAt != at {
		t.Errorf("ProcessAt = %v, want %v", task.ProcessAt, at)
	}

	task2 := NewTask("test", nil,
		WithProcessAt(at),
		WithProcessIn(7*time.Second),
	)
	if task2.ProcessAt == at {
		t.Error("ProcessAt should have been overridden by WithProcessIn")
	}
}

// ══════════════════════════════════════════════════════════════════════════
// taskqueue.go — Task.Validate + ServeMux + sentinel errors
// ══════════════════════════════════════════════════════════════════════════

func TestTask_Validate(t *testing.T) {
	valid := NewTask("email:welcome", []byte("hello"))
	if err := valid.Validate(); err != nil {
		t.Errorf("Validate() on valid task: %v", err)
	}

	noID := NewTask("email:welcome", []byte("hello"))
	noID.ID = ""
	if err := noID.Validate(); err == nil {
		t.Error("Validate() with empty ID should fail")
	}

	noType := NewTask("email:welcome", []byte("hello"))
	noType.Type = ""
	if err := noType.Validate(); err == nil {
		t.Error("Validate() with empty Type should fail")
	}

	noQueue := NewTask("email:welcome", []byte("hello"))
	noQueue.Queue = ""
	if err := noQueue.Validate(); err == nil {
		t.Error("Validate() with empty Queue should fail")
	}
}

func TestServeMux_HandleAndDispatch(t *testing.T) {
	mux := NewServeMux()
	var called bool

	mux.HandleFunc("email:welcome", func(ctx context.Context, task *Task) error {
		called = true
		if task.Type != "email:welcome" {
			t.Errorf("task.Type = %q, want email:welcome", task.Type)
		}
		return nil
	})

	task := &Task{Type: "email:welcome", ID: "1", Queue: "default", Payload: []byte("{}")}
	if err := mux.ProcessTask(context.Background(), task); err != nil {
		t.Fatalf("ProcessTask: %v", err)
	}
	if !called {
		t.Error("handler was not called")
	}
}

func TestServeMux_MissingHandler(t *testing.T) {
	mux := NewServeMux()
	task := &Task{Type: "unknown:job", ID: "1", Queue: "default", Payload: []byte("{}")}

	err := mux.ProcessTask(context.Background(), task)
	if err == nil {
		t.Fatal("ProcessTask() = nil, want error")
	}
}

func TestServeMux_Panics(t *testing.T) {
	mux := NewServeMux()

	panicked := func() (p bool) {
		defer func() {
			if r := recover(); r != nil {
				p = true
			}
		}()
		mux.Handle("", func(ctx context.Context, task *Task) error { return nil })
		return
	}()
	if !panicked {
		t.Error("Handle with empty type should panic")
	}

	panicked = func() (p bool) {
		defer func() {
			if r := recover(); r != nil {
				p = true
			}
		}()
		mux.Handle("test", nil)
		return
	}()
	if !panicked {
		t.Error("Handle with nil handler should panic")
	}
}

func TestSentinelErrors(t *testing.T) {
	if !errors.Is(ErrDuplicateTask, ErrDuplicateTask) {
		t.Error("ErrDuplicateTask should be its own sentinel")
	}
	if !errors.Is(ErrNoTask, ErrNoTask) {
		t.Error("ErrNoTask should be its own sentinel")
	}
	if !errors.Is(ErrTaskNotFound, ErrTaskNotFound) {
		t.Error("ErrTaskNotFound should be its own sentinel")
	}

	// Verify they are distinct.
	if errors.Is(ErrDuplicateTask, ErrNoTask) {
		t.Error("ErrDuplicateTask should not match ErrNoTask")
	}
}

func TestTask_StateConstants(t *testing.T) {
	states := map[State]string{
		StatePending:   "pending",
		StateActive:    "active",
		StateScheduled: "scheduled",
		StateRetry:     "retry",
		StateDead:      "dead",
		StateDone:      "done",
	}
	for s, expected := range states {
		if string(s) != expected {
			t.Errorf("State(%q).String() = %q, want %q", s, string(s), expected)
		}
	}
}

func TestServeMux_Concurrent(t *testing.T) {
	mux := NewServeMux()
	for i := 0; i < 100; i++ {
		mux.HandleFunc("task", func(ctx context.Context, task *Task) error { return nil })
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = mux.ProcessTask(context.Background(), &Task{Type: "task", ID: "x", Queue: "default"})
			}
		}()
	}
	wg.Wait()
}

// ══════════════════════════════════════════════════════════════════════════
// dlq.go — DLQPayload construction
// ══════════════════════════════════════════════════════════════════════════

func TestNewDLQPayload(t *testing.T) {
	data := json.RawMessage(`{"user_id":42}`)
	dlq := NewDLQPayload("send_email", data, errors.New("timeout"), 3, "user-1")

	if dlq.OriginalTaskType != "send_email" {
		t.Errorf("OriginalTaskType = %q, want send_email", dlq.OriginalTaskType)
	}
	if dlq.Error != "timeout" {
		t.Errorf("Error = %q, want timeout", dlq.Error)
	}
	if dlq.Attempts != 3 {
		t.Errorf("Attempts = %d, want 3", dlq.Attempts)
	}
	if dlq.UIN != "user-1" {
		t.Errorf("UIN = %q, want user-1", dlq.UIN)
	}
	if dlq.FailedAt == "" {
		t.Error("FailedAt must not be empty")
	}
}

func TestNewDLQPayload_NilError(t *testing.T) {
	dlq := NewDLQPayload("test", nil, nil, 1, "")
	if dlq.Error != "" {
		t.Errorf("Error = %q, want empty for nil error", dlq.Error)
	}
}

func TestDLQPayload_ToMessage(t *testing.T) {
	dlq := NewDLQPayload("send_email",
		json.RawMessage(`{"to":"a@b.com"}`),
		errors.New("timeout"),
		3,
		"user-1",
	)

	msg, err := dlq.ToMessage()
	if err != nil {
		t.Fatalf("ToMessage: %v", err)
	}
	if msg.Topic != DLQTopic {
		t.Errorf("Topic = %q, want %q", msg.Topic, DLQTopic)
	}
	if msg.Headers["x-task-type"] != "send_email" {
		t.Errorf("x-task-type header = %q, want send_email", msg.Headers["x-task-type"])
	}

	// Round-trip: parse the payload back.
	var parsed DLQPayload
	if err := json.Unmarshal(msg.Payload, &parsed); err != nil {
		t.Fatalf("unmarshal back: %v", err)
	}
	if parsed.OriginalTaskType != "send_email" {
		t.Errorf("round-trip OriginalTaskType = %q", parsed.OriginalTaskType)
	}
	if parsed.UIN != "user-1" {
		t.Errorf("round-trip UIN = %q", parsed.UIN)
	}
}

// ══════════════════════════════════════════════════════════════════════════
// client.go — Client with mock broker
// ══════════════════════════════════════════════════════════════════════════

func TestClient_Enqueue(t *testing.T) {
	m := newMockBroker()
	c := NewClient(m)

	task := NewTask("email:welcome", []byte("hello"))
	if err := c.Enqueue(context.Background(), task); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	if m.inspect.enqueueCalls != 1 {
		t.Errorf("enqueueCalls = %d, want 1", m.inspect.enqueueCalls)
	}

	// Verify the task is in pending.
	if stored, ok := m.tasks[task.ID]; !ok {
		t.Fatal("task not stored in broker")
	} else if stored.State != StatePending {
		t.Errorf("task state = %q, want %q", stored.State, StatePending)
	}
}

func TestClient_Enqueue_Deduplication(t *testing.T) {
	m := newMockBroker()
	c := NewClient(m)

	task1 := NewTask("email:welcome", []byte("hello"),
		WithUnique("dedup-1", 10*time.Minute))
	if err := c.Enqueue(context.Background(), task1); err != nil {
		t.Fatalf("first Enqueue: %v", err)
	}

	task2 := NewTask("email:welcome", []byte("hello"),
		WithUnique("dedup-1", 10*time.Minute))
	if err := c.Enqueue(context.Background(), task2); !errors.Is(err, ErrDuplicateTask) {
		t.Fatalf("second Enqueue = %v, want ErrDuplicateTask", err)
	}
}

func TestClient_EnqueueTask(t *testing.T) {
	m := newMockBroker()
	c := NewClient(m)

	ret, err := c.EnqueueTask(context.Background(), "email:welcome", []byte("data"),
		WithQueue("critical"),
		WithMaxRetries(5),
	)
	if err != nil {
		t.Fatalf("EnqueueTask: %v", err)
	}
	if ret.ID == "" {
		t.Error("returned task ID must not be empty")
	}
	if ret.Type != "email:welcome" {
		t.Errorf("Type = %q", ret.Type)
	}
	if ret.Queue != "critical" {
		t.Errorf("Queue = %q", ret.Queue)
	}
	if ret.State != StatePending {
		t.Errorf("State = %q, want pending", ret.State)
	}
}

func TestClient_Close(t *testing.T) {
	m := newMockBroker()
	c := NewClient(m)

	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if m.inspect.closeCalls != 1 {
		t.Errorf("closeCalls = %d, want 1", m.inspect.closeCalls)
	}
	if !m.closed {
		t.Error("broker not marked closed")
	}
}

// ══════════════════════════════════════════════════════════════════════════
// server.go — Server lifecycle with mock broker
// ══════════════════════════════════════════════════════════════════════════

func TestServer_RunAndStop(t *testing.T) {
	m := newMockBroker()
	srv := NewServer(ServerConfig{
		Broker:      m,
		Concurrency: 2,
	})

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Run(ctx, NewServeMux())
	}()

	// Let the workers start.
	time.Sleep(100 * time.Millisecond)

	// Stop via context.
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func TestServer_StopMethod(t *testing.T) {
	m := newMockBroker()
	srv := NewServer(ServerConfig{
		Broker:      m,
		Concurrency: 2,
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Run(context.Background(), NewServeMux())
	}()

	time.Sleep(100 * time.Millisecond)
	srv.Stop()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after Stop")
	}
}

func TestServer_StopIdempotent(t *testing.T) {
	m := newMockBroker()
	srv := NewServer(ServerConfig{
		Broker:      m,
		Concurrency: 2,
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Run(context.Background(), NewServeMux())
	}()

	time.Sleep(100 * time.Millisecond)

	// Multiple stops must not panic.
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			srv.Stop()
		}()
	}
	wg.Wait()

	<-errCh
}

func TestServer_ProcessTask_Success(t *testing.T) {
	m := newMockBroker()
	task := NewTask("greet", []byte("hello"))
	_ = m.Enqueue(context.Background(), task)

	mux := NewServeMux()
	done := make(chan struct{})
	mux.HandleFunc("greet", func(ctx context.Context, t *Task) error {
		close(done)
		return nil
	})

	srv := NewServer(ServerConfig{
		Broker:      m,
		Concurrency: 1,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go srv.Run(ctx, mux)

	select {
	case <-done:
		// Task was processed.
	case <-time.After(3 * time.Second):
		t.Fatal("handler was not called")
	}

	// Allow time for Ack.
	time.Sleep(100 * time.Millisecond)

	if m.inspect.ackCalls < 1 {
		t.Errorf("ackCalls = %d, want >=1", m.inspect.ackCalls)
	}

	// Task should be marked done.
	m.mu.Lock()
	stored := m.tasks[task.ID]
	m.mu.Unlock()
	if stored.State != StateDone {
		t.Errorf("task state = %q, want done", stored.State)
	}
}

func TestServer_ProcessTask_FailureRetry(t *testing.T) {
	m := newMockBroker()
	// Create a task that has already been retried once, so next retryDelay
	// uses retried=1 (20s base). We short-circuit: manually simulate the Nack→Schedule→Dequeue cycle.
	task := NewTask("flaky", []byte("hello"), WithMaxRetries(2))
	_ = m.Enqueue(context.Background(), task)

	var mu sync.Mutex
	attempts := 0
	mux := NewServeMux()
	done := make(chan struct{})
	mux.HandleFunc("flaky", func(ctx context.Context, t *Task) error {
		mu.Lock()
		attempts++
		a := attempts
		mu.Unlock()
		if a == 1 {
			return errors.New("transient error")
		}
		close(done)
		return nil
	})

	srv := NewServer(ServerConfig{
		Broker:           m,
		Concurrency:      1,
		ScheduleInterval: 50 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go srv.Run(ctx, mux)

	// Wait for first attempt → Nack triggers. After Nack, task is in retry state.
	// We manually fast-forward the retry by setting ProcessAt to the past and calling Schedule.
	time.Sleep(500 * time.Millisecond) // let first attempt + Nack complete

	m.mu.Lock()
	if t, ok := m.tasks[task.ID]; ok && t.State == StateRetry {
		// Fast-forward past the retry delay.
		t.ProcessAt = time.Now().Add(-1 * time.Second)
	}
	m.mu.Unlock()

	// Run schedule to promote retry → pending.
	_ = m.Schedule(context.Background())

	select {
	case <-done:
		// Success.
	case <-time.After(5 * time.Second):
		mu.Lock()
		a := attempts
		mu.Unlock()
		t.Fatalf("handler completed: attempts=%d", a)
	}

	mu.Lock()
	a := attempts
	mu.Unlock()
	if a != 2 {
		t.Errorf("attempts = %d, want 2", a)
	}
}

func TestServer_ProcessTask_DeadAfterMaxRetries(t *testing.T) {
	m := newMockBroker()
	task := NewTask("doomed", []byte("hello"), WithMaxRetries(1))
	_ = m.Enqueue(context.Background(), task)

	var mu sync.Mutex
	attempts := 0
	mux := NewServeMux()
	mux.HandleFunc("doomed", func(ctx context.Context, t *Task) error {
		mu.Lock()
		attempts++
		mu.Unlock()
		return errors.New("fail")
	})

	srv := NewServer(ServerConfig{
		Broker:           m,
		Concurrency:      1,
		ScheduleInterval: 50 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())
	go srv.Run(ctx, mux)
	defer cancel()

	// Let first attempt (initial) complete → Nack with retryAt (retry).
	time.Sleep(500 * time.Millisecond)

	// Simulate the retry being due, let scheduler promote it, then second attempt runs.
	m.mu.Lock()
	if t, ok := m.tasks[task.ID]; ok && t.State == StateRetry {
		t.ProcessAt = time.Now().Add(-1 * time.Second)
	}
	m.mu.Unlock()
	_ = m.Schedule(context.Background())

	// Wait for second attempt (retry #1 of 1) → Nack with retryAt=0 (dead).
	time.Sleep(500 * time.Millisecond)

	cancel()

	mu.Lock()
	a := attempts
	mu.Unlock()
	if a != 2 {
		t.Errorf("attempts = %d, want 2 (initial + 1 retry)", a)
	}

	// Verify the task is dead.
	m.mu.Lock()
	stored := m.tasks[task.ID]
	m.mu.Unlock()
	if stored.State != StateDead {
		t.Errorf("task state = %q, want dead", stored.State)
	}

	// Should have Nack'd with retryAt=0 (dead).
	var deadNack *nackRecord
	for i := range m.inspect.nackRetries {
		if m.inspect.nackRetries[i].retryAt.IsZero() {
			deadNack = &m.inspect.nackRetries[i]
		}
	}
	if deadNack == nil {
		t.Error("expected a Nack with retryAt=0 (dead-letter)")
	}
}

func TestServer_Scheduler(t *testing.T) {
	m := newMockBroker()

	// Enqueue a scheduled task.
	task := NewTask("future", []byte("data"), WithProcessAt(time.Now()))
	// Manually tweak to scheduled for testing.
	task.State = StateScheduled
	task.ProcessAt = time.Now().Add(-1 * time.Second) // already due
	_ = m.Enqueue(context.Background(), task)

	// Run a schedule pass manually.
	_ = m.Schedule(context.Background())

	m.mu.Lock()
	stored := m.tasks[task.ID]
	m.mu.Unlock()
	if stored.State != StatePending {
		t.Errorf("task state = %q, want pending", stored.State)
	}
}

func TestServer_Reaper(t *testing.T) {
	m := newMockBroker()

	task := NewTask("stuck", []byte("data"))
	_ = m.Enqueue(context.Background(), task)

	// Simulate dequeue + crash: active but past deadline.
	_, _ = m.Dequeue(context.Background(), []string{"default"}, time.Now().Add(-1*time.Hour))
	// The deadline was 1 hour ago, so reaper should recover it.

	_ = m.ReapStale(context.Background())

	m.mu.Lock()
	stored := m.tasks[task.ID]
	m.mu.Unlock()
	if stored.State != StatePending {
		t.Errorf("task state = %q, want pending (recovered)", stored.State)
	}
	if m.inspect.reapCalls != 1 {
		t.Errorf("reapCalls = %d, want 1", m.inspect.reapCalls)
	}
}

func TestServer_MultipleQueues(t *testing.T) {
	m := newMockBroker()

	// Enqueue tasks in different queues.
	lowTask := NewTask("job", []byte("low"), WithQueue("low"))
	highTask := NewTask("job", []byte("high"), WithQueue("critical"))
	_ = m.Enqueue(context.Background(), lowTask)
	_ = m.Enqueue(context.Background(), highTask)

	// With a filtered queue list that only contains "critical",
	// only the critical task should be dequeued.
	task, err := m.Dequeue(context.Background(),
		[]string{"critical"},
		time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("Dequeue: %v", err)
	}
	if task.Queue != "critical" {
		t.Errorf("dequeued queue = %q, want critical", task.Queue)
	}

	// The low-priority task should still be pending.
	m.mu.Lock()
	stored := m.tasks[lowTask.ID]
	m.mu.Unlock()
	if stored.State != StatePending {
		t.Errorf("low task state = %q, want pending", stored.State)
	}

	// Now dequeue with "low".
	task2, err := m.Dequeue(context.Background(),
		[]string{"low"},
		time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("Dequeue low: %v", err)
	}
	if task2.Queue != "low" {
		t.Errorf("dequeued queue = %q, want low", task2.Queue)
	}
}

func TestServer_RegisterCron(t *testing.T) {
	m := newMockBroker()
	srv := NewServer(ServerConfig{
		Broker: m,
	})

	// Register a cron that fires every minute (won't actually fire in test).
	err := srv.RegisterCron("*/1 * * * *", "cron:test", []byte("data"),
		WithUnique("cron:test", 10*time.Minute))
	if err != nil {
		t.Fatalf("RegisterCron: %v", err)
	}

	if srv.cronSvc == nil {
		t.Error("cronSvc should be initialized after RegisterCron")
	}
}

func TestServer_NoHandlersReturnsError(t *testing.T) {
	mux := NewServeMux()
	task := &Task{Type: "no-handler", ID: "1", Queue: "default", Payload: []byte("{}")}
	err := mux.ProcessTask(context.Background(), task)
	if err == nil {
		t.Fatal("ProcessTask() with unregistered type should return error")
	}
}

func TestServer_ConfigDefaults(t *testing.T) {
	cfg := ServerConfig{}
	cfg.setDefaults()

	if cfg.Concurrency != 10 {
		t.Errorf("Concurrency = %d, want 10", cfg.Concurrency)
	}
	if cfg.ShutdownTimeout != 30*time.Second {
		t.Errorf("ShutdownTimeout = %v, want 30s", cfg.ShutdownTimeout)
	}
	if cfg.ScheduleInterval != 5*time.Second {
		t.Errorf("ScheduleInterval = %v, want 5s", cfg.ScheduleInterval)
	}
	if cfg.ReaperInterval != 60*time.Second {
		t.Errorf("ReaperInterval = %v, want 60s", cfg.ReaperInterval)
	}
	if cfg.DefaultTimeout != 5*time.Minute {
		t.Errorf("DefaultTimeout = %v, want 5m", cfg.DefaultTimeout)
	}
	if len(cfg.Queues) == 0 {
		t.Error("Queues should default to {'default': 1}")
	}
}

func TestBuildQueueList(t *testing.T) {
	queues := map[string]int{"critical": 3, "default": 1, "low": 2}
	list := buildQueueList(queues)

	if len(list) != 6 {
		t.Errorf("list length = %d, want 6", len(list))
	}

	// Count occurrences.
	counts := map[string]int{}
	for _, q := range list {
		counts[q]++
	}
	if counts["critical"] != 3 {
		t.Errorf("critical count = %d, want 3", counts["critical"])
	}
	if counts["default"] != 1 {
		t.Errorf("default count = %d, want 1", counts["default"])
	}
	if counts["low"] != 2 {
		t.Errorf("low count = %d, want 2", counts["low"])
	}
}

func TestRetryDelay(t *testing.T) {
	// retryDelay is the internal function in server.go.
	tests := []struct {
		retried int
		wantMin int // seconds, minimum
		wantMax int // seconds, maximum (with jitter)
	}{
		{0, 9, 11},    // 10s ± 10%
		{1, 18, 22},   // 20s ± 10%
		{2, 36, 44},   // 40s ± 10%
		{3, 72, 88},   // 80s ± 10%
		{7, 1151, 1408}, // 1280s ± 10%
	}
	for _, tt := range tests {
		d := retryDelay(tt.retried)
		secs := int(d.Seconds())
		if secs < tt.wantMin || secs > tt.wantMax {
			t.Errorf("retryDelay(%d) = %ds, want between %d and %d",
				tt.retried, secs, tt.wantMin, tt.wantMax)
		}
	}
}

func TestDefaultConstants(t *testing.T) {
	if DefaultQueue != "default" {
		t.Errorf("DefaultQueue = %q, want default", DefaultQueue)
	}
	if DefaultMaxRetries != 3 {
		t.Errorf("DefaultMaxRetries = %d, want 3", DefaultMaxRetries)
	}
	if DefaultTimeout != 30*time.Minute {
		t.Errorf("DefaultTimeout = %v, want 30m", DefaultTimeout)
	}
}

// ══════════════════════════════════════════════════════════════════════════
// Concurrency stress test
// ══════════════════════════════════════════════════════════════════════════

func TestServer_ConcurrentWorkerSafety(t *testing.T) {
	m := newMockBroker()

	// Enqueue 20 tasks.
	taskCount := 20
	var mu sync.Mutex
	processedIDs := make(map[string]bool)

	mux := NewServeMux()
	mux.HandleFunc("job", func(ctx context.Context, task *Task) error {
		atomicAdd := func() {
			mu.Lock()
			processedIDs[task.ID] = true
			mu.Unlock()
		}
		atomicAdd()
		return nil
	})

	for i := 0; i < taskCount; i++ {
		task := NewTask("job", []byte("data"))
		_ = m.Enqueue(context.Background(), task)
	}

	srv := NewServer(ServerConfig{
		Broker:      m,
		Concurrency: 5,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go srv.Run(ctx, mux)

	// Wait for all tasks to be processed.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		done := len(processedIDs)
		mu.Unlock()
		if done >= taskCount {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	mu.Lock()
	total := len(processedIDs)
	mu.Unlock()
	if total != taskCount {
		t.Errorf("processed %d/%d tasks", total, taskCount)
	}
}
