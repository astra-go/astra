// Cache example: read-through caching for a user profile service.
//
// Uses github.com/astra-go/astra/cache (in-memory LRU backend).
// To switch to Redis, replace cache.NewMemory() with:
//
//	import cacheredis "github.com/astra-go/astra/cache/redis"
//	c, _ := cacheredis.New(cacheredis.Config{Addr: "localhost:6379"})
//
// Routes:
//
//	GET    /users/:id     fetch user (cache miss → load from DB; cache hit → return)
//	PUT    /users/:id     update user (invalidates cache entry)
//	DELETE /cache/:id     manually evict a cache entry
//	GET    /cache/stats   hit / miss counters
package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/cache"
	"github.com/astra-go/astra/middleware"
)

const userTTL = 5 * time.Minute

// ─── Simulated DB (slow store) ─────────────────────────────────────────────────

type User struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type DB struct {
	mu    sync.RWMutex
	users map[int64]*User
}

func NewDB() *DB {
	db := &DB{users: make(map[int64]*User)}
	db.users[1] = &User{ID: 1, Name: "Alice", Email: "alice@example.com"}
	db.users[2] = &User{ID: 2, Name: "Bob", Email: "bob@example.com"}
	db.users[3] = &User{ID: 3, Name: "Carol", Email: "carol@example.com"}
	return db
}

func (d *DB) Find(id int64) (*User, bool) {
	time.Sleep(20 * time.Millisecond) // simulate latency
	d.mu.RLock()
	defer d.mu.RUnlock()
	u, ok := d.users[id]
	return u, ok
}

func (d *DB) Update(id int64, name, email string) (*User, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	u, ok := d.users[id]
	if !ok {
		return nil, false
	}
	if name != "" {
		u.Name = name
	}
	if email != "" {
		u.Email = email
	}
	return u, true
}

// ─── Cache stats ───────────────────────────────────────────────────────────────

type Stats struct {
	Hits   atomic.Int64
	Misses atomic.Int64
}

func (s *Stats) Map() astra.Map {
	h, m := s.Hits.Load(), s.Misses.Load()
	ratio := 0.0
	if total := h + m; total > 0 {
		ratio = float64(h) / float64(total)
	}
	return astra.Map{"hits": h, "misses": m, "ratio": fmt.Sprintf("%.2f", ratio)}
}

// ─── Handlers ──────────────────────────────────────────────────────────────────

type UserHandler struct {
	db    *DB
	cache cache.Cache
	stats *Stats
}

func (h *UserHandler) cacheKey(id int64) string { return fmt.Sprintf("user:%d", id) }

func (h *UserHandler) Get(c *astra.Ctx) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return astra.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	var u User
	ctx := context.Background()

	// Read-through: return cached value or fetch from DB.
	err = cache.GetOrSet(ctx, h.cache, h.cacheKey(id), &u, userTTL, func() (any, error) {
		h.stats.Misses.Add(1)
		dbUser, ok := h.db.Find(id)
		if !ok {
			return nil, fmt.Errorf("not found")
		}
		return dbUser, nil
	})
	if err != nil {
		if err.Error() == "not found" {
			return astra.NewHTTPError(http.StatusNotFound, fmt.Sprintf("user %d not found", id))
		}
		return err
	}
	h.stats.Hits.Add(1)
	return c.JSON(http.StatusOK, astra.Map{"data": u, "source": "cache"})
}

func (h *UserHandler) Update(c *astra.Ctx) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return astra.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	var req struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := c.BindJSON(&req); err != nil {
		return err
	}
	u, ok := h.db.Update(id, req.Name, req.Email)
	if !ok {
		return astra.NewHTTPError(http.StatusNotFound, fmt.Sprintf("user %d not found", id))
	}
	// Invalidate so the next GET reflects the update.
	h.cache.Delete(context.Background(), h.cacheKey(id))
	return c.JSON(http.StatusOK, astra.Map{"data": u})
}

// ─── Main ───────────────────────────────────────────────────────────────────────

func main() {
	c := cache.NewMemory()
	defer c.Close()

	h := &UserHandler{db: NewDB(), cache: c, stats: &Stats{}}

	app := astra.New(astra.WithShutdownTimeout(10))
	app.Use(
		middleware.RequestID(),
		middleware.Logger(),
		middleware.Recovery(),
		middleware.CORSPermissive(),
	)

	app.GET("/users/:id", h.Get)
	app.PUT("/users/:id", h.Update)

	app.DELETE("/cache/:id", func(cc *astra.Ctx) error {
		id, err := strconv.ParseInt(cc.Param("id"), 10, 64)
		if err != nil {
			return astra.NewHTTPError(http.StatusBadRequest, "invalid id")
		}
		c.Delete(context.Background(), h.cacheKey(id))
		return cc.JSON(http.StatusOK, astra.Map{"evicted": id})
	})

	app.GET("/cache/stats", func(cc *astra.Ctx) error {
		return cc.JSON(http.StatusOK, h.stats.Map())
	})

	fmt.Println("Cache server :8080")
	fmt.Println("  GET /users/:id       (first call: ~20ms DB; repeat: <1ms cache)")
	fmt.Println("  PUT /users/:id       (updates DB + invalidates cache)")
	fmt.Println("  DELETE /cache/:id    (manual eviction)")
	fmt.Println("  GET /cache/stats")
	if err := app.Run(":8080"); err != nil {
		panic(err)
	}
}
