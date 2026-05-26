// Package orm — rw.go provides read/write splitting for GORM connections.
//
// Write operations (INSERT, UPDATE, DELETE, DDL, transactions) are always
// routed to the primary. Read operations are distributed across replicas
// using round-robin. When a replica is unhealthy it is removed from rotation
// and re-added automatically once it recovers.
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
// # Transaction safety
//
// Read always returns the primary when an active transaction is present in ctx
// (set by TxMiddleware or RunTx). This prevents phantom reads caused by
// replica lag inside a transaction.
package orm

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/astra-go/astra"
	"gorm.io/gorm"
)

const rwRouterKey = "gorm:rw_router"

// ReadWriteRouter routes write operations to the primary DB and read
// operations to replicas using round-robin with automatic health checking.
//
// The zero value is not usable; construct with NewReadWriteRouter.
type ReadWriteRouter struct {
	primary  *gorm.DB
	replicas []*gorm.DB

	mu      sync.RWMutex
	healthy []*gorm.DB // subset of replicas currently passing health checks

	counter uint64 // atomic round-robin index

	stopOnce sync.Once
	stopCh   chan struct{}
}

// NewReadWriteRouter creates a router with the given primary and zero or more
// replicas. When no replicas are provided all reads also go to primary.
//
// A background goroutine starts immediately to health-check replicas every
// 30 seconds. Call Close to stop it.
func NewReadWriteRouter(primary *gorm.DB, replicas ...*gorm.DB) *ReadWriteRouter {
	r := &ReadWriteRouter{
		primary:  primary,
		replicas: replicas,
		healthy:  make([]*gorm.DB, 0, len(replicas)),
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

// Write returns the primary *gorm.DB scoped to ctx.
// Always use Write for INSERT / UPDATE / DELETE / DDL.
func (r *ReadWriteRouter) Write(ctx context.Context) *gorm.DB {
	return r.primary.WithContext(ctx)
}

// Read returns a replica *gorm.DB scoped to ctx using round-robin selection.
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
	r.mu.RUnlock()

	if len(h) == 0 {
		return r.primary.WithContext(ctx)
	}

	idx := atomic.AddUint64(&r.counter, 1) % uint64(len(h))
	return h[idx].WithContext(ctx)
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

// Close stops the background health-check goroutine.
func (r *ReadWriteRouter) Close() {
	r.stopOnce.Do(func() { close(r.stopCh) })
}

// healthLoop periodically pings all replicas and updates the healthy list.
func (r *ReadWriteRouter) healthLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-r.stopCh:
			return
		case <-ticker.C:
			r.recheckReplicas()
		}
	}
}

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

// RWRouter retrieves the ReadWriteRouter injected by Middleware.
// Returns nil when the middleware was not registered.
func RWRouter(c *astra.Ctx) *ReadWriteRouter {
	v, _ := c.Get(rwRouterKey)
	rw, _ := v.(*ReadWriteRouter)
	return rw
}
