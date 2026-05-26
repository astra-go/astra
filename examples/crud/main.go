// CRUD example: full REST API with in-memory storage and input validation.
package main

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/middleware"
)

// ─── Domain ───────────────────────────────────────────────────────────────────

type User struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

type CreateReq struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

type UpdateReq struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
	Role  string `json:"role,omitempty"`
}

// ─── Store ────────────────────────────────────────────────────────────────────

type Store struct {
	mu      sync.RWMutex
	users   map[int64]*User
	counter int64
}

func NewStore() *Store { return &Store{users: make(map[int64]*User)} }

func (s *Store) Create(r CreateReq) *User {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counter++
	role := r.Role
	if role == "" {
		role = "user"
	}
	u := &User{ID: s.counter, Name: r.Name, Email: r.Email, Role: role, CreatedAt: time.Now()}
	s.users[u.ID] = u
	return u
}

func (s *Store) Get(id int64) (*User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.users[id]
	return u, ok
}

func (s *Store) List(page, size int) ([]*User, int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	all := make([]*User, 0, len(s.users))
	for _, u := range s.users {
		all = append(all, u)
	}
	total := len(all)
	start := (page - 1) * size
	if start >= total {
		return []*User{}, total
	}
	end := start + size
	if end > total {
		end = total
	}
	return all[start:end], total
}

func (s *Store) Update(id int64, r UpdateReq) (*User, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[id]
	if !ok {
		return nil, false
	}
	if r.Name != "" {
		u.Name = r.Name
	}
	if r.Email != "" {
		u.Email = r.Email
	}
	if r.Role != "" {
		u.Role = r.Role
	}
	return u, true
}

func (s *Store) Delete(id int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[id]; !ok {
		return false
	}
	delete(s.users, id)
	return true
}

// ─── Handlers ─────────────────────────────────────────────────────────────────

type Handler struct{ store *Store }

func (h *Handler) List(c *astra.Ctx) error {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	users, total := h.store.List(page, size)
	return c.JSON(http.StatusOK, astra.Map{
		"data":  users,
		"total": total,
		"page":  page,
		"size":  size,
	})
}

func (h *Handler) Create(c *astra.Ctx) error {
	var req CreateReq
	if err := c.BindJSON(&req); err != nil {
		return err
	}
	if req.Name == "" {
		return astra.NewHTTPError(http.StatusBadRequest, "name is required")
	}
	if req.Email == "" {
		return astra.NewHTTPError(http.StatusBadRequest, "email is required")
	}
	return c.JSON(http.StatusCreated, astra.Map{"data": h.store.Create(req)})
}

func (h *Handler) Get(c *astra.Ctx) error {
	id, err := parseID(c.Param("id"))
	if err != nil {
		return astra.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	u, ok := h.store.Get(id)
	if !ok {
		return astra.NewHTTPError(http.StatusNotFound, fmt.Sprintf("user %d not found", id))
	}
	return c.JSON(http.StatusOK, astra.Map{"data": u})
}

func (h *Handler) Update(c *astra.Ctx) error {
	id, err := parseID(c.Param("id"))
	if err != nil {
		return astra.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	var req UpdateReq
	if err := c.BindJSON(&req); err != nil {
		return err
	}
	u, ok := h.store.Update(id, req)
	if !ok {
		return astra.NewHTTPError(http.StatusNotFound, fmt.Sprintf("user %d not found", id))
	}
	return c.JSON(http.StatusOK, astra.Map{"data": u})
}

func (h *Handler) Delete(c *astra.Ctx) error {
	id, err := parseID(c.Param("id"))
	if err != nil {
		return astra.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	if !h.store.Delete(id) {
		return astra.NewHTTPError(http.StatusNotFound, fmt.Sprintf("user %d not found", id))
	}
	return c.NoContent(http.StatusNoContent)
}

func parseID(s string) (int64, error) { return strconv.ParseInt(s, 10, 64) }

// ─── Main ─────────────────────────────────────────────────────────────────────

func main() {
	app := astra.New(astra.WithShutdownTimeout(10))
	app.Use(
		middleware.RequestID(),
		middleware.Logger(),
		middleware.Recovery(),
		middleware.CORS(),
	)

	app.GET("/health", func(c *astra.Ctx) error {
		return c.JSON(http.StatusOK, astra.Map{"status": "ok"})
	})

	store := NewStore()
	// seed
	store.Create(CreateReq{Name: "Alice", Email: "alice@example.com", Role: "admin"})
	store.Create(CreateReq{Name: "Bob", Email: "bob@example.com"})

	h := &Handler{store: store}

	v1 := app.Group("/api/v1")
	v1.Use(middleware.RateLimit(100, 20))

	v1.GET("/users", h.List)
	v1.POST("/users", h.Create)
	v1.GET("/users/:id", h.Get)
	v1.PUT("/users/:id", h.Update)
	v1.DELETE("/users/:id", h.Delete)

	fmt.Println("CRUD server :8080")
	if err := app.Run(":8080"); err != nil {
		panic(err)
	}
}
