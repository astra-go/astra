// MQ example: message-queue producer/consumer pattern with an in-process broker.
//
// This example mirrors the interface from github.com/astra-go/astra/mq.
// Swap InMemoryBroker for the rabbitmq / kafka sub-module to connect a real broker:
//
//   import "github.com/astra-go/astra/mq/rabbitmq"
//   p, _ := rabbitmq.NewProducer(rabbitmq.Config{URL: "amqp://guest:guest@localhost/"})
//   c, _ := rabbitmq.NewConsumer(rabbitmq.ConsumerConfig{...})
//
// Routes:
//   POST /orders          publish an order.created event
//   POST /orders/:id/ship publish an order.shipped event
//   GET  /events          stream of all published events (SSE)
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/middleware"
)

// ─── Broker abstraction (mirrors mq.Producer / mq.Consumer) ───────────────────

type Message struct {
	Topic   string
	Key     string
	Payload []byte
	Headers map[string]string
	Meta    map[string]any
}

type Handler func(ctx context.Context, msg *Message) error

type Producer interface {
	Publish(ctx context.Context, msg *Message) error
	Close() error
}

type Consumer interface {
	Subscribe(ctx context.Context, topics []string, group string, h Handler) error
	Close() error
}

// ─── In-process broker ────────────────────────────────────────────────────────

type InMemoryBroker struct {
	mu   sync.RWMutex
	subs map[string][]Handler // topic → handlers
}

func NewBroker() *InMemoryBroker {
	return &InMemoryBroker{subs: make(map[string][]Handler)}
}

func (b *InMemoryBroker) Subscribe(topic string, h Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subs[topic] = append(b.subs[topic], h)
}

func (b *InMemoryBroker) Publish(ctx context.Context, msg *Message) error {
	b.mu.RLock()
	handlers := append([]Handler(nil), b.subs[msg.Topic]...)
	b.mu.RUnlock()
	for _, h := range handlers {
		if err := h(ctx, msg); err != nil {
			return err
		}
	}
	return nil
}

func (b *InMemoryBroker) Close() error { return nil }

// ─── Event log (SSE feed) ─────────────────────────────────────────────────────

type EventLog struct {
	mu     sync.RWMutex
	events []Event
}

type Event struct {
	Topic   string         `json:"topic"`
	Payload map[string]any `json:"payload"`
	At      string         `json:"at"`
}

func (l *EventLog) Append(topic string, payload []byte) {
	var p map[string]any
	json.Unmarshal(payload, &p)
	l.mu.Lock()
	l.events = append(l.events, Event{Topic: topic, Payload: p, At: time.Now().Format(time.RFC3339)})
	l.mu.Unlock()
}

func (l *EventLog) All() []Event {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]Event, len(l.events))
	copy(out, l.events)
	return out
}

// ─── Order service ────────────────────────────────────────────────────────────

type OrderService struct {
	broker *InMemoryBroker
}

func (s *OrderService) CreateOrder(ctx context.Context, item string, qty int) error {
	payload, _ := json.Marshal(map[string]any{
		"item":       item,
		"qty":        qty,
		"created_at": time.Now().Format(time.RFC3339),
	})
	return s.broker.Publish(ctx, &Message{
		Topic:   "order.created",
		Key:     item,
		Payload: payload,
	})
}

func (s *OrderService) ShipOrder(ctx context.Context, orderID string) error {
	payload, _ := json.Marshal(map[string]any{
		"order_id":    orderID,
		"shipped_at":  time.Now().Format(time.RFC3339),
	})
	return s.broker.Publish(ctx, &Message{
		Topic:   "order.shipped",
		Key:     orderID,
		Payload: payload,
	})
}

// ─── Main ─────────────────────────────────────────────────────────────────────

func main() {
	broker := NewBroker()
	log := &EventLog{}

	// Consumers: log every event and print it.
	for _, topic := range []string{"order.created", "order.shipped"} {
		t := topic
		broker.Subscribe(t, func(_ context.Context, msg *Message) error {
			log.Append(msg.Topic, msg.Payload)
			fmt.Printf("[consumer] topic=%s payload=%s\n", msg.Topic, msg.Payload)
			return nil
		})
	}

	svc := &OrderService{broker: broker}

	app := astra.New(astra.WithShutdownTimeout(10))
	app.Use(
		middleware.RequestID(),
		middleware.Logger(),
		middleware.Recovery(),
	)

	// POST /orders — publish order.created
	app.POST("/orders", func(c *astra.Ctx) error {
		var req struct {
			Item string `json:"item"`
			Qty  int    `json:"qty"`
		}
		if err := c.BindJSON(&req); err != nil {
			return err
		}
		if req.Item == "" {
			return astra.NewHTTPError(http.StatusBadRequest, "item is required")
		}
		if req.Qty <= 0 {
			req.Qty = 1
		}
		if err := svc.CreateOrder(c.Request().Context(), req.Item, req.Qty); err != nil {
			return err
		}
		return c.JSON(http.StatusAccepted, astra.Map{"status": "published", "topic": "order.created"})
	})

	// POST /orders/:id/ship — publish order.shipped
	app.POST("/orders/:id/ship", func(c *astra.Ctx) error {
		if err := svc.ShipOrder(c.Request().Context(), c.Param("id")); err != nil {
			return err
		}
		return c.JSON(http.StatusAccepted, astra.Map{"status": "published", "topic": "order.shipped"})
	})

	// GET /events — return all logged events
	app.GET("/events", func(c *astra.Ctx) error {
		return c.JSON(http.StatusOK, astra.Map{"events": log.All()})
	})

	fmt.Println("MQ server :8080")
	fmt.Println("  POST /orders          {\"item\":\"widget\",\"qty\":3}")
	fmt.Println("  POST /orders/42/ship")
	fmt.Println("  GET  /events")
	if err := app.Run(":8080"); err != nil {
		panic(err)
	}
}
