// ORM example: GORM with SQLite (in-memory), product CRUD + soft-delete.
//
// Uses gorm.io/gorm and github.com/glebarez/sqlite (pure-Go driver,
// no CGO required) — both are in the root module's go.mod.
//
// Routes:
//   POST   /api/v1/products             create
//   GET    /api/v1/products             list (with pagination)
//   GET    /api/v1/products/:id         get by ID
//   PUT    /api/v1/products/:id         update
//   DELETE /api/v1/products/:id         soft-delete
//   POST   /api/v1/products/:id/restore restore a soft-deleted product
package main

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/middleware"
)

// ─── Model ─────────────────────────────────────────────────────────────────────

type Product struct {
	gorm.Model
	Name     string  `json:"name"  gorm:"not null"`
	Price    float64 `json:"price" gorm:"not null"`
	Stock    int     `json:"stock"`
	Category string  `json:"category"`
}

type CreateProductReq struct {
	Name     string  `json:"name"`
	Price    float64 `json:"price"`
	Stock    int     `json:"stock"`
	Category string  `json:"category"`
}

type UpdateProductReq struct {
	Name     string  `json:"name,omitempty"`
	Price    float64 `json:"price,omitempty"`
	Stock    int     `json:"stock,omitempty"`
	Category string  `json:"category,omitempty"`
}

// ─── Repo ───────────────────────────────────────────────────────────────────────

type ProductRepo struct{ db *gorm.DB }

func (r *ProductRepo) Create(req CreateProductReq) (*Product, error) {
	p := &Product{Name: req.Name, Price: req.Price, Stock: req.Stock, Category: req.Category}
	return p, r.db.Create(p).Error
}

func (r *ProductRepo) FindAll(page, size int) ([]Product, int64, error) {
	var (
		products []Product
		total    int64
	)
	r.db.Model(&Product{}).Count(&total)
	err := r.db.Offset((page-1)*size).Limit(size).Find(&products).Error
	return products, total, err
}

func (r *ProductRepo) FindByID(id uint) (*Product, error) {
	var p Product
	if err := r.db.First(&p, id).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *ProductRepo) Update(id uint, req UpdateProductReq) (*Product, error) {
	p, err := r.FindByID(id)
	if err != nil {
		return nil, err
	}
	updates := map[string]any{}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Price != 0 {
		updates["price"] = req.Price
	}
	if req.Stock != 0 {
		updates["stock"] = req.Stock
	}
	if req.Category != "" {
		updates["category"] = req.Category
	}
	if err := r.db.Model(p).Updates(updates).Error; err != nil {
		return nil, err
	}
	return p, nil
}

func (r *ProductRepo) Delete(id uint) error {
	return r.db.Delete(&Product{}, id).Error
}

func (r *ProductRepo) Restore(id uint) error {
	return r.db.Unscoped().Model(&Product{}).Where("id = ?", id).
		Update("deleted_at", nil).Error
}

// ─── Handlers ──────────────────────────────────────────────────────────────────

type ProductHandler struct{ repo *ProductRepo }

func (h *ProductHandler) List(c *astra.Ctx) error {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	products, total, err := h.repo.FindAll(page, size)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, astra.Map{
		"data":  products,
		"total": total,
		"page":  page,
		"size":  size,
	})
}

func (h *ProductHandler) Create(c *astra.Ctx) error {
	var req CreateProductReq
	if err := c.BindJSON(&req); err != nil {
		return err
	}
	if req.Name == "" {
		return astra.NewHTTPError(http.StatusBadRequest, "name is required")
	}
	if req.Price <= 0 {
		return astra.NewHTTPError(http.StatusBadRequest, "price must be positive")
	}
	p, err := h.repo.Create(req)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, astra.Map{"data": p})
}

func (h *ProductHandler) Get(c *astra.Ctx) error {
	id, err := parseID(c.Param("id"))
	if err != nil {
		return astra.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	p, err := h.repo.FindByID(id)
	if err != nil {
		return gormErr(err)
	}
	return c.JSON(http.StatusOK, astra.Map{"data": p})
}

func (h *ProductHandler) Update(c *astra.Ctx) error {
	id, err := parseID(c.Param("id"))
	if err != nil {
		return astra.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	var req UpdateProductReq
	if err := c.BindJSON(&req); err != nil {
		return err
	}
	p, err := h.repo.Update(id, req)
	if err != nil {
		return gormErr(err)
	}
	return c.JSON(http.StatusOK, astra.Map{"data": p})
}

func (h *ProductHandler) Delete(c *astra.Ctx) error {
	id, err := parseID(c.Param("id"))
	if err != nil {
		return astra.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	if err := h.repo.Delete(id); err != nil {
		return gormErr(err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *ProductHandler) Restore(c *astra.Ctx) error {
	id, err := parseID(c.Param("id"))
	if err != nil {
		return astra.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	if err := h.repo.Restore(id); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, astra.Map{"status": "restored"})
}

func parseID(s string) (uint, error) {
	n, err := strconv.ParseUint(s, 10, 64)
	return uint(n), err
}

func gormErr(err error) error {
	if err == gorm.ErrRecordNotFound {
		return astra.NewHTTPError(http.StatusNotFound, "record not found")
	}
	return err
}

// ─── Main ───────────────────────────────────────────────────────────────────────

func main() {
	// In-memory SQLite: no files, no external service — ideal for demos and tests.
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic(err)
	}
	if err := db.AutoMigrate(&Product{}); err != nil {
		panic(err)
	}

	// Seed
	db.Create(&Product{Name: "Widget A", Price: 9.99, Stock: 100, Category: "widgets"})
	db.Create(&Product{Name: "Widget B", Price: 19.99, Stock: 50, Category: "widgets"})
	db.Create(&Product{Name: "Gadget X", Price: 49.99, Stock: 25, Category: "gadgets"})

	h := &ProductHandler{repo: &ProductRepo{db: db}}

	app := astra.New(astra.WithShutdownTimeout(10))
	app.Use(
		middleware.RequestID(),
		middleware.Logger(),
		middleware.Recovery(),
		middleware.CORS(),
	)

	v1 := app.Group("/api/v1")
	v1.GET("/products", h.List)
	v1.POST("/products", h.Create)
	v1.GET("/products/:id", h.Get)
	v1.PUT("/products/:id", h.Update)
	v1.DELETE("/products/:id", h.Delete)
	v1.POST("/products/:id/restore", h.Restore)

	fmt.Println("ORM server :8080  (SQLite in-memory)")
	if err := app.Run(":8080"); err != nil {
		panic(err)
	}
}
