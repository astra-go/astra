// Package mq - Redis-based delay compensation and idempotency.
//
// This file provides Redis-backed delay queue and idempotency checks
// for backends that lack native support. It lives in the mq/ package
// (not a sub-package) to avoid import cycles with mq.Message and mq.Producer.
package mq

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// Compensator provides Redis-based compensation for missing backend capabilities.
type Compensator struct {
	rdb *goredis.Client
}

// NewCompensator creates a new Redis compensator.
func NewCompensator(rdb *goredis.Client) *Compensator {
	return &Compensator{rdb: rdb}
}

// EnqueueDelay stores a delayed message in Redis (ZSET with score = deliverAt).
// The DelayScanner will pick it up when the delay expires.
func (c *Compensator) EnqueueDelay(ctx context.Context, msg *Message) error {
	if msg.Delay <= 0 {
		return fmt.Errorf("mq: delay must be positive")
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("mq: marshal message: %w", err)
	}
	deliverAt := time.Now().Add(msg.Delay).UnixMilli()
	key := "mq:delay:" + msg.Topic
	_, err = c.rdb.ZAdd(ctx, key, goredis.Z{
		Score:  float64(deliverAt),
		Member: string(payload),
	}).Result()
	return err
}

// CheckIdempotency checks and records idempotency key.
// Returns true if the key already exists (duplicate), false if not.
// If not duplicate, records the key with the given TTL.
func (c *Compensator) CheckIdempotency(ctx context.Context, idempKey string, ttl time.Duration) (bool, error) {
	if idempKey == "" {
		return false, nil
	}
	key := "mq:idem:" + idempKey
	ok, err := c.rdb.SetNX(ctx, key, "1", ttl).Result()
	if err != nil {
		return false, fmt.Errorf("mq: check idempotency: %w", err)
	}
	// SetNX returns true if key was set (not duplicate), false if key exists (duplicate)
	return !ok, nil
}

// ReleaseIdempotency removes the idempotency key after successful consumption.
func (c *Compensator) ReleaseIdempotency(ctx context.Context, idempKey string) error {
	if idempKey == "" {
		return nil
	}
	key := "mq:idem:" + idempKey
	_, err := c.rdb.Del(ctx, key).Result()
	return err
}

// DelayScanner scans Redis ZSET for delayed messages and re-enqueues them.
type DelayScanner struct {
	rdb       *goredis.Client
	producer  Producer
	interval  time.Duration
	stopCh    chan struct{}
}

// NewDelayScanner creates a new DelayScanner.
func NewDelayScanner(rdb *goredis.Client, producer Producer, interval time.Duration) *DelayScanner {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &DelayScanner{
		rdb:      rdb,
		producer: producer,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Run starts the delay scanner. It blocks until Stop() is called or ctx is cancelled.
func (s *DelayScanner) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.scanAndRequeue(ctx)
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// Stop stops the delay scanner.
func (s *DelayScanner) Stop() {
	close(s.stopCh)
}

// scanAndRequeue scans Redis ZSET for messages with deliverAt <= now and re-publishes them.
func (s *DelayScanner) scanAndRequeue(ctx context.Context) {
	now := time.Now().UnixMilli()
	topics, err := s.rdb.Keys(ctx, "mq:delay:*").Result()
	if err != nil {
		return
	}
	for _, key := range topics {
		members, err := s.rdb.ZRangeByScoreWithScores(ctx, key, &goredis.ZRangeBy{
			Min:    "0",
			Max:    fmt.Sprintf("%d", now),
			Offset: 0,
			Count:  100,
		}).Result()
		if err != nil {
			continue
		}
		for _, z := range members {
			payload, ok := z.Member.(string)
			if !ok {
				continue
			}
			var msg Message
			if err := json.Unmarshal([]byte(payload), &msg); err != nil {
				continue
			}
			if err := s.producer.Publish(ctx, &msg); err != nil {
				continue
			}
			s.rdb.ZRem(ctx, key, payload)
		}
	}
}
