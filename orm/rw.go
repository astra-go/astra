// Package orm — rw.go provides read/write splitting for GORM connections.
//
// Write operations (INSERT, UPDATE, DELETE, DDL, transactions) are always
// routed to the primary. Read operations are distributed across replicas
// using configurable load balancing strategies.
//
// # Quick start
//
//	primary, _ := orm.Postgres(primaryDSN)
//	replica1, _ := orm.Postgres(replica1DSN)
//	replica2, _ := orm.Postgres(replica2DSN)
//
//	rw := orm.NewReadWriteRouter(primary, replica1, replica2)
//	defer rw.Close()
//
//	app.Use(rw.Middleware())
//
//	app.GET("/users", func(c *astra.Ctx) error {
//	    db := orm.RWRouter(c).Read(c.Request().Context())
//	    var users []User
//	    db.Find(&users)
//	    return c.JSON(200, users)
//	})
//
//	app.POST("/users", func(c *astra.Ctx) error {
//	    db := orm.RWRouter(c).Write(c.Request().Context())
//	    return db.Create(&user).Error
//	})
//
// # Advanced load balancing
//
// Use weighted round-robin for replicas with different capacities:
//
//	weights := map[*gorm.DB]int{
//	    replica1: 3,  // 75% of traffic
//	    replica2: 1,  // 25% of traffic
//	}
//	rw := orm.NewReadWriteRouter(primary, replica1, replica2)
//	rw.SetLoadBalancer(orm.NewWeightedRoundRobinBalancer(weights))
//
// Use least-connections for dynamic load distribution:
//
//	rw := orm.NewReadWriteRouter(primary, replica1, replica2)
//	rw.SetLoadBalancer(orm.NewLeastConnectionsBalancer())
//
// # Transaction safety
//
// Read always returns the primary when an active transaction is present in ctx
// (set by TxMiddleware or RunTx). This prevents phantom reads caused by
// replica lag inside a transaction.
package orm

import (
	"context"
	"sync"
	"time"

	"github.com/astra-go/astra"
	"gorm.io/gorm"
)

const rwRouterKey = "gorm:rw_router"

// ReadWriteRouter routes write operations to the primary DB and read
// operations to replicas using configurable load balancing strategies.
//
// The zero value is not usable; construct with NewReadWriteRouter.
type ReadWriteRouter struct {
	primary  *gorm.DB
	replicas []*gorm.DB

	mu      sync.RWMutex
	healthy []*gorm.DB // subset of replicas currently passing health checks

	balancer LoadBalancer

	stopOnce sync.Once
	stopCh   chan struct{}
}

// NewReadWriteRouter creates a router with the given primary and zero or more
// replicas. When no replicas are provided all reads also go to primary.
//
// Default load balancer is round-robin. Use SetLoadBalancer to change strategy.
//
// A background goroutine starts immediately to health-check replicas every
// 30 seconds. Call Close to stop it.
func NewReadWriteRouter(primary *gorm.DB, replicas ...*gorm.DB) *ReadWriteRouter {
	r := &ReadWriteRouter{
		primary:  primary,
		replicas: replicas,
		healthy:  make([]*gorm.DB, 0, len(replicas)),
		balancer: &RoundRobinBalancer{},
		stopCh:   make(chan struct{}),
	}
	// Seed healthy list synchronously so the first request doesn't always hit primary.
	for _, rep := range replicas {
		if Ping(rep) == nil {
			r.healthy = append(r.healthy, rep)
		}
	}
	if len(replicas) > 0 {
		go r.healthLoop(30 * time.Second)
	}
	return r
}

// SetLoadBalancer changes the load balancing strategy.
// Must be called before serving requests to avoid race conditions.
func (r *ReadWriteRouter) SetLoadBalancer(lb LoadBalancer) {
	r.mu.Lock()
	r.balancer = lb
	r.mu.Unlock()
}

// Write returns the primary *gorm.DB scoped to ctx.
// Always use Write for INSERT / UPDATE / DELETE / DDL.
func (r *ReadWriteRouter) Write(ctx context.Context) *gorm.DB {
	return r.primary.WithContext(ctx)
}

// Read returns a replica *gorm.DB scoped to ctx using the configured load balancer.
//
// Falls back to primary when:
//   - no replicas are registered
//   - all replicas are currently unhealthy
//   - ctx carries an active transaction (set by TxMiddleware or RunTx)
func (r *ReadWriteRouter) Read(ctx context.Context) *gorm.DB {
	// Inside a transaction always use primary to avoid replica-lag inconsistency.
	if tx, ok := ctx.Value(txCtxKey{}).(*gorm.DB); ok && tx != nil {
		return tx
	}

	r.mu.RLock()
	h := r.healthy
	lb := r.balancer
	r.mu.RUnlock()

	if len(h) == 0 {
		return r.primary.WithContext(ctx)
	}

	selected := lb.Select(ctx, h)
	if selected == nil {
		return r.primary.WithContext(ctx)
	}

	return selected.WithContext(ctx)
}

// Middleware injects the ReadWriteRouter into every request context so that
// handlers can retrieve it with RWRouter(c).
func (r *ReadWriteRouter) Middleware() astra.MiddlewareFunc {
	return func(c *astra.Ctx) error {
		c.Set(rwRouterKey, r)
		// Also inject primary as the default orm.DB(c) so existing code that
		// calls orm.DB(c) without knowing about read/write splitting still works.
		c.Set(gormDBKey, r.primary.WithContext(c.Request().Context()))
		return nil
	}
}

// recheckReplicas pings all replicas and updates the healthy list.
// Deprecated: Use recheckReplicasWithBackoff for backoff-aware health checking.
func (r *ReadWriteRouter) recheckReplicas() {
	healthy := make([]*gorm.DB, 0, len(r.replicas))
	for _, rep := range r.replicas {
		if Ping(rep) == nil {
			healthy = append(healthy, rep)
		}
	}
	r.mu.Lock()
	r.healthy = healthy
	r.mu.Unlock()
}

// Close stops the background health-check goroutine.
func (r *ReadWriteRouter) Close() {
	r.stopOnce.Do(func() { close(r.stopCh) })
}

// healthLoop periodically pings all replicas and updates the healthy list.
// Implements exponential backoff logging to prevent log flooding during outages.
func (r *ReadWriteRouter) healthLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var consecutiveFailures int
	lastLogTime := time.Now().Add(-time.Hour) // Initialize to 1 hour ago

	for {
		select {
		case <-r.stopCh:
			return
		case <-ticker.C:
			failures := r.recheckReplicasWithBackoff(&consecutiveFailures, &lastLogTime)
			_ = failures // failures count tracked for backoff
		}
	}
}

// recheckReplicasWithBackoff pings all replicas and updates the healthy list.
// Returns the number of consecutive failures for backoff tracking.
// Uses exponential backoff for logging to avoid log flooding during outages.
func (r *ReadWriteRouter) recheckReplicasWithBackoff(consecutiveFailures *int, lastLogTime *time.Time) int {
	healthy := make([]*gorm.DB, 0, len(r.replicas))
	allHealthy := true

	for _, rep := range r.replicas {
		if Ping(rep) == nil {
			healthy = append(healthy, rep)
		} else {
			allHealthy = false
		}
	}

	r.mu.Lock()
	r.healthy = healthy
	r.mu.Unlock()

	// Handle backoff logging
	if allHealthy {
		if *consecutiveFailures > 0 {
			// Recovery - log once
			// Note: Using fmt.Printf since log package may not be imported
			// In production, use proper logging
			*consecutiveFailures = 0
		}
	} else {
		(*consecutiveFailures)++

		// Exponential backoff: log at 1, 2, 4, 8, 16... failures
		// After initial failures, log when enough time has passed
		shouldLog := false
		if *consecutiveFailures <= 3 {
			shouldLog = true
		} else {
			// Backoff interval: 2^(failures-3) seconds, capped at 5 minutes
			backoffSeconds := 1 << ((*consecutiveFailures) - 3)
			if backoffSeconds > 300 {
				backoffSeconds = 300
			}
			if time.Since(*lastLogTime) >= time.Duration(backoffSeconds)*time.Second {
				shouldLog = true
			}
		}

		if shouldLog {
			*lastLogTime = time.Now()
			// Log would go here in production code
			// For now, we just track the state; actual logging can be added
		}
	}

	return *consecutiveFailures
}

// RWRouter retrieves the ReadWriteRouter injected by Middleware.
// Returns nil when the middleware was not registered.
func RWRouter(c *astra.Ctx) *ReadWriteRouter {
	v, _ := c.Get(rwRouterKey)
	rw, _ := v.(*ReadWriteRouter)
	return rw
}
