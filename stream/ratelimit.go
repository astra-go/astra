package stream

import (
	"errors"
	"net"
	"sync"
	"time"
)

// ErrRateLimited is returned by Send or Recv when the per-stream message rate
// limit is exceeded. Handlers can detect this with errors.Is and decide whether
// to drop the message, wait, or close the stream.
var ErrRateLimited = errors.New("stream: rate limit exceeded")

// msgRateLimiter is a per-stream token-bucket rate limiter.
// One instance is created per active stream when WithRateLimit is configured.
type msgRateLimiter struct {
	mu       sync.Mutex
	tokens   float64
	rate     float64 // tokens refilled per second
	burst    float64 // maximum token capacity
	lastTime time.Time
}

func newMsgRateLimiter(rate float64, burst int) *msgRateLimiter {
	return &msgRateLimiter{
		tokens:   float64(burst),
		rate:     rate,
		burst:    float64(burst),
		lastTime: time.Now(),
	}
}

func (r *msgRateLimiter) allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	r.tokens += now.Sub(r.lastTime).Seconds() * r.rate
	r.lastTime = now
	if r.tokens > r.burst {
		r.tokens = r.burst
	}
	if r.tokens >= 1.0 {
		r.tokens--
		return true
	}
	return false
}

// connLimiter tracks the number of concurrent active streams per client IP.
// One instance is shared across all connections to the same route handler.
type connLimiter struct {
	mu    sync.Mutex
	conns map[string]int
	max   int
}

func newConnLimiter(max int) *connLimiter {
	return &connLimiter{conns: make(map[string]int), max: max}
}

// acquire claims one stream slot for the given remote address.
// Returns (true, releaseFunc) on success, (false, nil) when the limit is reached.
// The caller must invoke releaseFunc when the stream closes.
func (c *connLimiter) acquire(remoteAddr string) (bool, func()) {
	key := hostOnly(remoteAddr)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conns[key] >= c.max {
		return false, nil
	}
	c.conns[key]++
	return true, func() {
		c.mu.Lock()
		c.conns[key]--
		if c.conns[key] == 0 {
			delete(c.conns, key)
		}
		c.mu.Unlock()
	}
}

// hostOnly strips the port from a "host:port" string.
// Returns addr unchanged when it cannot be parsed.
func hostOnly(addr string) string {
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return addr
}
