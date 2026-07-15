package taskqueue

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// mockBroker is an in-memory Broker for unit testing.
type mockBroker struct {
	mu       sync.Mutex
	tasks    map[string]*Task          // taskID → task
	pending  []*Task                   // pending queue (FIFO)
	active   map[string]time.Time      // taskID → deadline
	dead     map[string]*Task          // dead tasks
	closed   bool
	inspect  mockInspect               // calls recorded for verification
}

type mockInspect struct {
	enqueueCalls   int
	dequeueCalls   int
	ackCalls       int
	nackCalls      int
	scheduleCalls  int
	reapCalls      int
	closeCalls     int
	nackRetries    []nackRecord
}

type nackRecord struct {
	taskID  string
	lastErr string
	retryAt time.Time
	dead    bool
}

func newMockBroker() *mockBroker {
	return &mockBroker{
		tasks:  make(map[string]*Task),
		active: make(map[string]time.Time),
		dead:   make(map[string]*Task),
	}
}

func (m *mockBroker) Enqueue(ctx context.Context, task *Task) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inspect.enqueueCalls++

	if task.ID == "" {
		return fmt.Errorf("task missing ID")
	}

	// Check deduplication
	if task.UniqueKey != "" {
		for _, t := range m.tasks {
			if t.UniqueKey == task.UniqueKey && time.Since(t.CreatedAt) < task.UniqueFor {
				return ErrDuplicateTask
			}
		}
	}

	task.UpdatedAt = time.Now()
	if task.ProcessAt.IsZero() || time.Now().After(task.ProcessAt) {
		task.State = StatePending
		task.ProcessAt = time.Now()
	} else {
		task.State = StateScheduled
	}
	m.tasks[task.ID] = task
	if task.State == StatePending {
		m.pending = append(m.pending, task)
	}
	return nil
}

func (m *mockBroker) Dequeue(ctx context.Context, queues []string, deadline time.Time) (*Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inspect.dequeueCalls++

	// Pick first pending task matching any requested queue.
	for i, t := range m.pending {
		for _, q := range queues {
			if t.Queue == q {
				// Remove from pending.
				m.pending = append(m.pending[:i], m.pending[i+1:]...)
				t.State = StateActive
				t.UpdatedAt = time.Now()
				m.active[t.ID] = deadline
				return t, nil
			}
		}
	}
	return nil, ErrNoTask
}

func (m *mockBroker) Ack(ctx context.Context, task *Task) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inspect.ackCalls++

	task.State = StateDone
	task.UpdatedAt = time.Now()
	delete(m.active, task.ID)
	return nil
}

func (m *mockBroker) Nack(ctx context.Context, task *Task, lastErr string, retryAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inspect.nackCalls++

	task.LastError = lastErr
	task.UpdatedAt = time.Now()

	rec := nackRecord{
		taskID:  task.ID,
		lastErr: lastErr,
		retryAt: retryAt,
	}
	m.inspect.nackRetries = append(m.inspect.nackRetries, rec)

	if retryAt.IsZero() {
		task.State = StateDead
		m.dead[task.ID] = task
		rec.dead = true
		m.inspect.nackRetries[len(m.inspect.nackRetries)-1] = rec
	} else {
		task.State = StateRetry
		task.ProcessAt = retryAt
	}
	delete(m.active, task.ID)
	return nil
}

func (m *mockBroker) Schedule(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inspect.scheduleCalls++

	for _, t := range m.tasks {
		if t.State == StateScheduled || t.State == StateRetry {
			if time.Now().After(t.ProcessAt) {
				t.State = StatePending
				t.UpdatedAt = time.Now()
				m.pending = append(m.pending, t)
			}
		}
	}
	return nil
}

func (m *mockBroker) ReapStale(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inspect.reapCalls++

	now := time.Now()
	for id, deadline := range m.active {
		if now.After(deadline) {
			t := m.tasks[id]
			t.State = StatePending
			t.UpdatedAt = now
			m.pending = append(m.pending, t)
			delete(m.active, id)
		}
	}
	return nil
}

func (m *mockBroker) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inspect.closeCalls++
	m.closed = true
	return nil
}

func (m *mockBroker) PendingCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.pending)
}

func (m *mockBroker) ActiveCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.active)
}

func (m *mockBroker) DeadCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.dead)
}
