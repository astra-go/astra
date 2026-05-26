package taskqueue

import "context"

// Client enqueues tasks to a broker. It is safe for concurrent use.
type Client struct {
	broker Broker
}

// NewClient creates a new Client backed by broker.
func NewClient(broker Broker) *Client {
	return &Client{broker: broker}
}

// Enqueue submits the task to the broker.
// It returns ErrDuplicateTask if the task has a unique key that already exists.
func (c *Client) Enqueue(ctx context.Context, task *Task) error {
	return c.broker.Enqueue(ctx, task)
}

// EnqueueTask is a convenience wrapper that creates a Task and enqueues it.
//
//	client.EnqueueTask(ctx, "email:welcome", payload,
//	    taskqueue.WithQueue("critical"),
//	    taskqueue.WithMaxRetries(5),
//	)
func (c *Client) EnqueueTask(ctx context.Context, taskType string, payload []byte, opts ...TaskOption) (*Task, error) {
	t := NewTask(taskType, payload, opts...)
	if err := c.broker.Enqueue(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// Close releases resources held by the underlying broker.
func (c *Client) Close() error {
	return c.broker.Close()
}
