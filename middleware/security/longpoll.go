// Package middleware — Long-Polling support.
//
// Long-polling is a technique where the server holds a request open until an
// event is available (or a timeout is reached), then responds.  It is useful
// as a fallback for environments that do not support SSE or WebSocket.
//
// # Architecture
//
// A LongPollManager maintains a registry of named topics.  Clients subscribe
// to a topic via PollHandler; publishers push events via Publish.
//
//	mgr := middleware.NewLongPollManager(middleware.LongPollConfig{
//	    DefaultTimeout: 30 * time.Second,
//	    MaxTimeout:     60 * time.Second,
//	    BufferSize:     64,
//	})
//
//	// Register the SSE-alternative polling endpoint
//	app.GET("/events", mgr.PollHandler("topic_key"))
//
//	// Publish from any goroutine / handler
//	mgr.Publish("my-topic", map[string]any{"type": "order.created", "id": 42})
//
// # Client flow
//
//  1. Client sends GET /events?topic=my-topic (or whatever topic param you choose).
//  2. Server parks the goroutine until an event arrives or DefaultTimeout elapses.
//  3. If an event arrives: 200 JSON with the event body.
//  4. If timeout: 204 No Content (client should immediately re-connect).
//
// # Topic extraction
//
// The topic key is read from the Context using the TopicFunc you supply, or
// defaults to the "topic" query parameter.
package security

import (
	"net/http"
	"sync"
	"time"

	"github.com/astra-go/astra"
)

// LongPollConfig configures the long-poll manager.
type LongPollConfig struct {
	// DefaultTimeout is the maximum time a poll request waits for an event.
	// When the timeout elapses without an event the handler returns 204.
	// Default: 30 seconds.
	DefaultTimeout time.Duration

	// MaxTimeout caps the per-request timeout even if the client specifies a
	// larger value via the "timeout" query parameter. Default: 60 seconds.
	MaxTimeout time.Duration

	// BufferSize is the per-topic channel buffer size.
	// When the buffer is full, Publish blocks briefly then drops the event.
	// Default: 64.
	BufferSize int
}

// LongPollManager manages long-poll subscriptions across topics.
type LongPollManager struct {
	cfg     LongPollConfig
	mu      sync.Mutex
	topics  map[string][]chan any
}

// NewLongPollManager creates a LongPollManager with the given config.
func NewLongPollManager(cfg LongPollConfig) *LongPollManager {
	if cfg.DefaultTimeout == 0 {
		cfg.DefaultTimeout = 30 * time.Second
	}
	if cfg.MaxTimeout == 0 {
		cfg.MaxTimeout = 60 * time.Second
	}
	if cfg.BufferSize == 0 {
		cfg.BufferSize = 64
	}
	return &LongPollManager{
		cfg:    cfg,
		topics: make(map[string][]chan any),
	}
}

// Publish delivers event to all active subscribers of topic.
// Publish is safe to call from any goroutine.
func (m *LongPollManager) Publish(topic string, event any) {
	m.mu.Lock()
	subs := make([]chan any, len(m.topics[topic]))
	copy(subs, m.topics[topic])
	m.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- event:
		default:
			// Drop if the subscriber's buffer is full.
		}
	}
}

// PollHandler returns an Astra HandlerFunc that parks the request until an
// event is published on the given topic (resolved by topicFn) or the timeout
// elapses.
//
//	// Static topic
//	app.GET("/notifications", mgr.PollHandler(func(c *contract.Context) string {
//	    return "user:" + middleware.GetUserID(c)
//	}))
func (m *LongPollManager) PollHandler(topicFn func(c *astra.Ctx) string) astra.HandlerFunc {
	return func(c *astra.Ctx) error {
		topic := topicFn(c)
		if topic == "" {
			return c.JSON(http.StatusBadRequest, map[string]any{"error": "empty topic"})
		}

		// Determine per-request timeout.
		timeout := m.cfg.DefaultTimeout
		if timeout > m.cfg.MaxTimeout {
			timeout = m.cfg.MaxTimeout
		}

		ch := make(chan any, 1)
		m.subscribe(topic, ch)
		defer m.unsubscribe(topic, ch)

		select {
		case event := <-ch:
			return c.JSON(http.StatusOK, event)
		case <-time.After(timeout):
			// No event in time — tell client to reconnect.
			c.Writer().WriteHeader(http.StatusNoContent)
			return nil
		case <-c.Request().Context().Done():
			// Client disconnected.
			c.Writer().WriteHeader(http.StatusNoContent)
			return nil
		}
	}
}

// PollHandlerByQuery returns a PollHandler that reads the topic from the named
// query parameter (default: "topic").
func (m *LongPollManager) PollHandlerByQuery(param string) astra.HandlerFunc {
	if param == "" {
		param = "topic"
	}
	return m.PollHandler(func(c *astra.Ctx) string {
		return c.Request().URL.Query().Get(param)
	})
}

func (m *LongPollManager) subscribe(topic string, ch chan any) {
	m.mu.Lock()
	m.topics[topic] = append(m.topics[topic], ch)
	m.mu.Unlock()
}

func (m *LongPollManager) unsubscribe(topic string, ch chan any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	subs := m.topics[topic]
	for i, s := range subs {
		if s == ch {
			m.topics[topic] = append(subs[:i], subs[i+1:]...)
			break
		}
	}
	if len(m.topics[topic]) == 0 {
		delete(m.topics, topic)
	}
}
