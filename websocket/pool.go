package websocket

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	gorilla "github.com/gorilla/websocket"
)

// ─── Connection Pool ─────────────────────────────────────────────────────────
//
// Pool manages a reusable pool of WebSocket connections for client-side usage.
// Use cases:
//   - Outbound WebSocket connections (e.g. API proxies, real-time dashboards)
//   - Connection reuse to reduce TCP handshake overhead
//   - Health checking and automatic reconnection

// PoolConfig configures a connection pool.
type PoolConfig struct {
	// MaxIdle is the maximum number of idle connections in the pool.
	// Default: 100.
	MaxIdle int

	// MaxActive is the maximum number of active connections.
	// Default: 1000.
	MaxActive int

	// MaxReconnect is the maximum number of reconnection attempts.
	// Default: 5.
	MaxReconnect int

	// ReconnectDelay is the initial delay between reconnection attempts.
	// Default: 1s.
	ReconnectDelay time.Duration

	// HealthCheckInterval is how often to check connection health.
	// Default: 30s.
	HealthCheckInterval time.Duration

	// Dialer creates the underlying WebSocket connection.
	// Required.
	Dialer func(ctx context.Context) (*ClientConn, error)
}

// ClientConn wraps a WebSocket connection with metadata.
type ClientConn struct {
	ID       string
	Conn     *gorilla.Conn
	Created  time.Time
	LastSeen time.Time
	Healthy  atomic.Bool
	Metadata map[string]any
}

// poolEntry holds an idle connection and its expiration time.
type poolEntry struct {
	conn    *ClientConn
	expires time.Time
}

// Pool manages a pool of reusable WebSocket connections.
type Pool struct {
	cfg    PoolConfig
	idle   []poolEntry
	active atomic.Int64
	mu     sync.Mutex

	// notify signals when an idle connection is available.
	notify chan struct{}

	ctx    context.Context
	cancel context.CancelFunc
	closed atomic.Bool
}

// NewPool creates a new connection pool.
func NewPool(cfg PoolConfig) *Pool {
	if cfg.MaxIdle == 0 {
		cfg.MaxIdle = 100
	}
	if cfg.MaxActive == 0 {
		cfg.MaxActive = 1000
	}
	if cfg.MaxReconnect == 0 {
		cfg.MaxReconnect = 5
	}
	if cfg.ReconnectDelay == 0 {
		cfg.ReconnectDelay = time.Second
	}
	if cfg.HealthCheckInterval == 0 {
		cfg.HealthCheckInterval = 30 * time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())
	p := &Pool{
		cfg:    cfg,
		idle:   make([]poolEntry, 0, cfg.MaxIdle),
		notify: make(chan struct{}, 1), // Buffered to avoid blocking.
		ctx:    ctx,
		cancel: cancel,
	}

	// Start background health check.
	go p.healthCheckLoop()

	return p
}

// Get acquires a connection from the pool.
// Returns a connection, or an error if the pool is closed or the max is reached.
func (p *Pool) Get(ctx context.Context) (*ClientConn, error) {
	if p.closed.Load() {
		return nil, errors.New("websocket pool: closed")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Check for context cancellation.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Try to get an idle connection.
	if len(p.idle) > 0 {
		entry := p.idle[len(p.idle)-1]
		p.idle = p.idle[:len(p.idle)-1]

		if entry.expires.Before(time.Now()) {
			entry.conn.Conn.Close()
			return nil, errors.New("websocket pool: connection expired")
		}

		p.active.Add(1)
		entry.conn.LastSeen = time.Now()
		return entry.conn, nil
	}

	// No idle connections - check if we can create a new one.
	if p.active.Load() >= int64(p.cfg.MaxActive) {
		return nil, errors.New("websocket pool: max active connections reached")
	}

	// Create a new connection.
	conn, err := p.cfg.Dialer(ctx)
	if err != nil {
		return nil, err
	}

	p.active.Add(1)
	conn.LastSeen = time.Now()
	return conn, nil
}

// Put returns a connection to the pool.
// If the pool is full, the connection is closed.
func (p *Pool) Put(conn *ClientConn) {
	if conn == nil {
		return
	}

	if p.closed.Load() {
		if conn.Conn != nil {
			conn.Conn.Close()
		}
		return
	}

	if !conn.Healthy.Load() {
		if conn.Conn != nil {
			conn.Conn.Close()
		}
		p.active.Add(-1)
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.idle) >= p.cfg.MaxIdle {
		if conn.Conn != nil {
			conn.Conn.Close()
		}
	} else {
		p.idle = append(p.idle, poolEntry{
			conn:    conn,
			expires: time.Now().Add(p.cfg.HealthCheckInterval * 2),
		})
	}
	p.active.Add(-1)

	// Signal one waiting getter.
	select {
	case p.notify <- struct{}{}:
	default:
	}
}

// Close closes all connections and stops the pool.
func (p *Pool) Close() error {
	if p.closed.CompareAndSwap(false, true) {
		p.cancel()
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	for _, entry := range p.idle {
		if entry.conn.Conn != nil {
			entry.conn.Conn.Close()
		}
	}
	p.idle = p.idle[:0]


	return nil
}

// PutMany adds multiple connections to the pool (for warmup).
func (p *Pool) PutMany(conns []*ClientConn) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, conn := range conns {
		if len(p.idle) >= p.cfg.MaxIdle {
			if conn.Conn != nil {
				conn.Conn.Close()
			}
		} else {
			p.idle = append(p.idle, poolEntry{
				conn:    conn,
				expires: time.Now().Add(p.cfg.HealthCheckInterval * 2),
			})
		}
	}
}

// Stats returns pool statistics.
func (p *Pool) Stats() PoolStats {
	return PoolStats{
		Active:    int(p.active.Load()),
		Idle:      p.idleLen(),
		MaxIdle:   p.cfg.MaxIdle,
		MaxActive: p.cfg.MaxActive,
	}
}

// PoolStats contains pool statistics.
type PoolStats struct {
	Active    int
	Idle      int
	MaxIdle   int
	MaxActive int
}

func (p *Pool) idleLen() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.idle)
}

// healthCheckLoop periodically checks idle connections.
func (p *Pool) healthCheckLoop() {
	ticker := time.NewTicker(p.cfg.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.healthCheck()
		}
	}
}

// healthCheck checks and removes expired connections.
func (p *Pool) healthCheck() {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	newIdle := make([]poolEntry, 0, len(p.idle))
	for _, entry := range p.idle {
		if entry.expires.Before(now) {
			entry.conn.Conn.Close()
		} else {
			newIdle = append(newIdle, entry)
		}
	}
	p.idle = newIdle
}

// ─── ReconnectingPool ────────────────────────────────────────────────────────
//
// ReconnectingPool wraps a Pool and automatically reconnects on failure.
// Use this for outbound WebSocket connections that must stay connected.

type ReconnectingPool struct {
	cfg  PoolConfig
	url  string
	conn atomic.Pointer[ClientConn]
	stop context.CancelFunc
	mu   sync.RWMutex
}

// NewReconnectingPool creates a pool that auto-reconnects.
func NewReconnectingPool(url string, cfg PoolConfig) *ReconnectingPool {
	return &ReconnectingPool{
		cfg: cfg,
		url: url,
	}
}

// Connect starts the reconnecting pool.
func (p *ReconnectingPool) Connect(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	p.stop = cancel

	return p.reconnect(ctx)
}

// reconnect attempts to connect with exponential backoff.
func (p *ReconnectingPool) reconnect(ctx context.Context) error {
	var delay time.Duration

	for attempt := 0; attempt < p.cfg.MaxReconnect; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}

		conn, err := p.cfg.Dialer(ctx)
		if err != nil {
			slog.Warn("websocket reconnect failed",
				"attempt", attempt+1,
				"err", err,
				"delay", delay)
			delay = p.nextDelay(delay)
			continue
		}

		p.conn.Store(conn)
		delay = p.cfg.ReconnectDelay // reset on success
		slog.Info("websocket reconnected", "url", p.url)
		return nil
	}

	return errors.New("websocket: max reconnect attempts reached")
}

// nextDelay calculates the next backoff delay.
func (p *ReconnectingPool) nextDelay(current time.Duration) time.Duration {
	if current == 0 {
		return p.cfg.ReconnectDelay
	}
	return current * 2
}

// Close stops the reconnecting pool.
func (p *ReconnectingPool) Close() error {
	if p.stop != nil {
		p.stop()
	}
	return nil
}

// Conn returns the current connection, or nil if not connected.
func (p *ReconnectingPool) Conn() *ClientConn {
	return p.conn.Load()
}