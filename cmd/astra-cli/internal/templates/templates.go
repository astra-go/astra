// Package templates provides all code-generation templates for astra-cli.
package templates

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/astra-go/astra/cmd/astra-cli/internal/tpldata"
)

// ─── Render helpers ───────────────────────────────────────────────────────────

func mustParse(name, src string) *template.Template {
	t, err := template.New(name).Parse(src)
	if err != nil {
		panic("template parse error: " + err.Error() + " in " + name)
	}
	return t
}

func render(name, src string, data any) string {
	buf := new(bytes.Buffer)
	if err := mustParse(name, src).Execute(buf, data); err != nil {
		panic("template execute error: " + err.Error())
	}
	return buf.String()
}

// ─── Project-level templates ─────────────────────────────────────────────────

// RenderMakefile generates the Makefile.
func RenderMakefile(d tpldata.Data) string {
	return render("makefile", makefileSrc, d)
}

// Gitignore returns the .gitignore content.
func Gitignore() string { return gitignoreSrc }

func makefileSrc string = `BINARY := {{.NameLower}}
IMAGE  := {{.NameLower}}:latest
GO     := go
PORT   := 8080

.PHONY: build run test lint clean tidy docker-build docker-run docker-stop

build:
	$(GO) build -o $(BINARY) ./cmd/server

run:
	$(GO) run ./cmd/server

test:
	$(GO) test ./... -v -race

lint:
	golangci-lint run ./...

clean:
	rm -f $(BINARY)

tidy:
	$(GO) mod tidy

docker-build:
	docker build -t $(IMAGE) .

docker-run:
	docker run --rm -p $(PORT):$(PORT) --name $(BINARY) $(IMAGE)

docker-stop:
	docker stop $(BINARY) || true
`

const gitignoreSrc = `# Binary
{{.NameLower}}
*.exe
*.exe~
*.dll
*.so
*.dylib
*.test
*.out
coverage.html

# Build output
/bin/

# IDE
.idea/
.vscode/
*.swp
*.swo

# Environment
.env
.env.local

# macOS
.DS_Store
`

// RenderConfigDev generates config/dev.yaml.
func RenderConfigDev(d tpldata.Data) string {
	return render("configDev", configDevSrc, d)
}

// RenderConfigProd generates config/prod.yaml.
func RenderConfigProd(d tpldata.Data) string {
	return render("configProd", configProdSrc, d)
}

const configDevSrc = `server:
  port: 8080
  mode: dev
  shutdown_timeout: 5

database:
  dsn: "host=localhost user=postgres password='' dbname={{.NameLower}}_dev sslmode=disable"
  max_open: 5
  max_idle: 2

cache:
  redis_addr: "localhost:6379"
  key_prefix: "{{.NameLower}}:dev:"

log:
  level: debug
  format: text
`

const configProdSrc = `server:
  port: 8080
  mode: prod
  shutdown_timeout: 30

database:
  dsn: "${DATABASE_DSN}"
  max_open: 25
  max_idle: 5

cache:
  redis_addr: "${REDIS_ADDR}"
  key_prefix: "{{.NameLower}}:"

log:
  level: info
  format: json
`

// RenderMainSimple generates the simple-layout main.go.
func RenderMainSimple(d tpldata.Data) string {
	return render("mainSimple", mainSimpleSrc, d)
}

// RenderMainDDD generates the DDD-layout cmd/server/main.go.
func RenderMainDDD(d tpldata.Data) string {
	return render("mainDDD", mainDDDSrc, d)
}

const mainSimpleSrc = `package main

import (
	"net/http"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/middleware"
)

func main() {
	app := astra.New(
		astra.WithMode(astra.ModeDev),
		astra.WithShutdownTimeout(30),
	)

	// Built-in middleware
	app.Use(
		middleware.RequestID(),
		middleware.Logger(),
		middleware.Recovery(),
		middleware.CORS("*"),
	)

	// Health probes — must NOT sit behind auth middleware
	app.GET("/health/live", func(c *astra.Ctx) error {
		return c.JSON(http.StatusOK, astra.Map{"status": "ok"})
	})
	app.GET("/health/ready", func(c *astra.Ctx) error {
		return c.JSON(http.StatusOK, astra.Map{"status": "ok"})
	})

	registerRoutes(app)

	app.Run(":8080")
}
`

const mainDDDSrc = `package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/middleware"
)

//go:generate wire

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	app := astra.New(
		astra.WithMode(astra.Mode(os.Getenv("APP_ENV"))),
		astra.WithShutdownTimeout(30),
	)

	app.Use(
		middleware.RequestID(),
		middleware.Logger(),
		middleware.Recovery(),
		middleware.CORS("*"),
	)

	app.GET("/health/live", func(c *astra.Ctx) error {
		return c.JSON(http.StatusOK, astra.Map{"status": "ok"})
	})
	app.GET("/health/ready", func(c *astra.Ctx) error {
		return c.JSON(http.StatusOK, astra.Map{"status": "ok"})
	})

	// Container initialisation (wire / manual DI)
	container := initContainer(app)

	// Wire up handlers
	v1 := app.Group("/api/v1")
	_ = container // TODO: use container to get handler instances
	// Example: handler.NewArticleHandler(container.MustGet(...)).Register(v1)

	<-ctx.Done()
	app.Stop(context.Background())
}
`

// RenderWireProvider generates the wire provider file.
func RenderWireProvider(d tpldata.Data) string {
	return render("wire", wireProviderSrc, d)
}

const wireProviderSrc = `//go:build wireinject

package main

import "github.com/google/wire"

// InitializeApp wires up the full application dependency graph.
//
// Build with:
//   wire gen .
//
// Or use the built-in DI container (no code generation required):
//   astra-cli init --out ./my-project
//   astra-cli generate container --dir ./cmd/server
func InitializeApp() (*App, error) {
	wire.Build(
		NewApp,
		// TODO: add your providers here
	)
	return nil, nil
}
`

// RenderContainer generates the manual DI container file.
func RenderContainer(d tpldata.Data) string {
	return render("container", containerSrc, d)
}

const containerSrc = `package main

import (
	"context"
	"database/sql"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/di"
)

// initContainer builds the application DI container and binds its lifecycle
// to app. Factories run lazily and at most once (singleton). Hooks registered
// via OnStop run in reverse order during graceful shutdown.
//
// Usage in main():
//
//	app := astra.New()
//	c   := initContainer(app)
//	svc := di.MustInvoke[*YourService](c)
//	handler.NewYourHandler(svc).Register(app.Group("/api/v1"))
//	app.Run(":8080")
func initContainer(app *astra.App) *di.Container {
	c := di.New()

	// ── Infrastructure ────────────────────────────────────────────────────────
	// di.Provide[*sql.DB](c, func(_ *di.Container) (*sql.DB, error) {
	// 	return sql.Open("postgres", os.Getenv("DATABASE_URL"))
	// })

	// ── Repositories ─────────────────────────────────────────────────────────
	// di.Provide[*UserRepo](c, func(c *di.Container) (*UserRepo, error) {
	// 	db, _ := di.Invoke[*sql.DB](c)
	// 	return NewUserRepo(db), nil
	// })

	// ── Services ─────────────────────────────────────────────────────────────
	// di.Provide[*UserService](c, func(c *di.Container) (*UserService, error) {
	// 	repo, _ := di.Invoke[*UserRepo](c)
	// 	svc := NewUserService(repo)
	// 	c.OnStop(func(ctx context.Context) error { return svc.Close(ctx) })
	// 	return svc, nil
	// })

	_ = context.Background // suppress unused import if no OnStop hooks yet

	c.BindApp(app)
	return c
}
`

// RenderErrorCodes generates pkg/errors/errors.go.
func RenderErrorCodes(d tpldata.Data) string {
	return render("errors", errorCodesSrc, d)
}

const errorCodesSrc = `package errors

import "github.com/astra-go/astra"

// Application-level error sentinels.
// Use astra.NewHTTPError(statusCode, message) for HTTP-aware errors.
var (
	// ErrNotFound is returned when a resource does not exist.
	ErrNotFound = astra.NewHTTPError(404, "resource not found")

	// ErrUnauthorized is returned when the caller is not authenticated.
	ErrUnauthorized = astra.NewHTTPError(401, "unauthorized")

	// ErrForbidden is returned when the caller lacks permission.
	ErrForbidden = astra.NewHTTPError(403, "forbidden")

	// ErrBadRequest is returned for invalid input.
	ErrBadRequest = astra.NewHTTPError(400, "bad request")

	// ErrConflict is returned when a resource already exists.
	ErrConflict = astra.NewHTTPError(409, "conflict")

	// ErrInternal is returned for unexpected server errors.
	ErrInternal = astra.NewHTTPError(500, "internal server error")
)
`

// ─── Service / Handler / Model / DTO templates ────────────────────────────────

// RenderService generates a service interface + stub implementation.
func RenderService(d tpldata.Data) string {
	return render("service", serviceSrc, d)
}

// RenderHandler generates an HTTP handler with List/Create/Get/Update/Delete.
func RenderHandler(d tpldata.Data) string {
	return render("handlerTpl", handlerSrc, d)
}

// RenderModel generates a GORM model.
func RenderModel(d tpldata.Data) string {
	return render("modelTpl", modelSrc, d)
}

// RenderCRUDModel generates a GORM model with column fields.
func RenderCRUDModel(d tpldata.Data) string {
	return render("crudModel", crudModelSrc, d)
}

// RenderCRUDRepo generates a repository file.
func RenderCRUDRepo(d tpldata.Data) string {
	return render("crudRepo", crudRepoSrc, d)
}

// RenderCRUDHandler generates a CRUD handler file.
func RenderCRUDHandler(d tpldata.Data) string {
	return render("crudHandler", crudHandlerSrc, d)
}

// RenderCRUDService generates a service layer file.
func RenderCRUDService(d tpldata.Data) string {
	return render("crudService", crudServiceSrc, d)
}

// RenderDTO generates a DTO file with request/response structs.
func RenderDTO(d tpldata.Data) string {
	return render("dto", dtoSrc, d)
}

const serviceSrc = `package service

import "context"

// ─── DTOs ─────────────────────────────────────────────────────────────────────

// Create{{.Name}}Request is the input for creating a {{.NameLower}}.
type Create{{.Name}}Request struct {
	// TODO: add fields
}

// Update{{.Name}}Request is the input for updating a {{.NameLower}}.
type Update{{.Name}}Request struct {
	// TODO: add fields
}

// {{.Name}}Response is the output DTO for {{.NameLower}} operations.
type {{.Name}}Response struct {
	ID int64 `json:"id"`
	// TODO: add fields
}

// ─── Interface ────────────────────────────────────────────────────────────────

// {{.Name}}Service defines the business-logic interface for {{.NameLower}} operations.
type {{.Name}}Service interface {
	List(ctx context.Context, page, limit int, keyword string) ([]*{{.Name}}Response, int64, error)
	Get(ctx context.Context, id int64) (*{{.Name}}Response, error)
	Create(ctx context.Context, req *Create{{.Name}}Request) (*{{.Name}}Response, error)
	Update(ctx context.Context, id int64, req *Update{{.Name}}Request) (*{{.Name}}Response, error)
	Delete(ctx context.Context, id int64) error
}

// ─── Implementation ───────────────────────────────────────────────────────────

// {{.Name}}ServiceImpl is the default implementation of {{.Name}}Service.
type {{.Name}}ServiceImpl struct {
	// TODO: inject repository
}

// New{{.Name}}Service creates a new {{.Name}}ServiceImpl.
func New{{.Name}}Service() *{{.Name}}ServiceImpl {
	return &{{.Name}}ServiceImpl{}
}

func (s *{{.Name}}ServiceImpl) List(ctx context.Context, page, limit int, keyword string) ([]*{{.Name}}Response, int64, error) {
	// TODO: implement
	return nil, 0, nil
}

func (s *{{.Name}}ServiceImpl) Get(ctx context.Context, id int64) (*{{.Name}}Response, error) {
	// TODO: implement
	return nil, nil
}

func (s *{{.Name}}ServiceImpl) Create(ctx context.Context, req *Create{{.Name}}Request) (*{{.Name}}Response, error) {
	// TODO: implement
	return nil, nil
}

func (s *{{.Name}}ServiceImpl) Update(ctx context.Context, id int64, req *Update{{.Name}}Request) (*{{.Name}}Response, error) {
	// TODO: implement
	return nil, nil
}

func (s *{{.Name}}ServiceImpl) Delete(ctx context.Context, id int64) error {
	// TODO: implement
	return nil
}
`

const handlerSrc = `package handler

import (
	"net/http"
	"strconv"

	"github.com/astra-go/astra"
)

// {{.Name}}Handler handles {{.NameLower}}-related HTTP requests.
type {{.Name}}Handler struct {
	// TODO: inject service
	// svc {{.Name}}Service
}

// New{{.Name}}Handler creates a new {{.Name}}Handler.
func New{{.Name}}Handler() *{{.Name}}Handler {
	return &{{.Name}}Handler{}
}

// Register mounts all {{.NameLower}} routes onto the given route group.
func (h *{{.Name}}Handler) Register(g *astra.Group) {
	g.GET("/{{.NameLower}}s",         h.List)
	g.POST("/{{.NameLower}}s",        h.Create)
	g.GET("/{{.NameLower}}s/:id",     h.Get)
	g.PUT("/{{.NameLower}}s/:id",     h.Update)
	g.DELETE("/{{.NameLower}}s/:id",  h.Delete)
}

// ─── DTOs ─────────────────────────────────────────────────────────────────────

// {{.Name}}ListQuery holds pagination and filter parameters.
type {{.Name}}ListQuery struct {
	Page    int    `form:"page"    validate:"min=1"`
	Limit   int    `form:"limit"   validate:"min=1,max=100"`
	Keyword string `form:"keyword"`
}

// Create{{.Name}}Request is the request body for creating a {{.NameLower}}.
type Create{{.Name}}Request struct {
	// TODO: add fields, e.g.:
	// Name string `json:"name" validate:"required,min=2,max=100"`
}

// Update{{.Name}}Request is the request body for updating a {{.NameLower}}.
type Update{{.Name}}Request struct {
	// TODO: add fields
}

// ─── Handlers ─────────────────────────────────────────────────────────────────

// List returns a paginated list of {{.NameLower}}s.
func (h *{{.Name}}Handler) List(c *astra.Ctx) error {
	var q {{.Name}}ListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		return err
	}
	if q.Page == 0 {
		q.Page = 1
	}
	if q.Limit == 0 {
		q.Limit = 20
	}
	ctx := c.Request.Context()
	_ = ctx // TODO: items, total, err := h.svc.List(ctx, q.Page, q.Limit, q.Keyword)
	return c.JSON(http.StatusOK, astra.Map{"data": []any{}, "total": 0, "page": q.Page, "limit": q.Limit})
}

// Create creates a new {{.NameLower}}.
func (h *{{.Name}}Handler) Create(c *astra.Ctx) error {
	var req Create{{.Name}}Request
	if err := c.ShouldBindJSON(&req); err != nil {
		return err
	}
	ctx := c.Request.Context()
	_ = ctx // TODO: item, err := h.svc.Create(ctx, req)
	return c.JSON(http.StatusCreated, astra.Map{"data": req})
}

// Get returns a {{.NameLower}} by ID.
func (h *{{.Name}}Handler) Get(c *astra.Ctx) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return astra.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	ctx := c.Request.Context()
	_ = ctx // TODO: item, err := h.svc.Get(ctx, id)
	return c.JSON(http.StatusOK, astra.Map{"id": id})
}

// Update updates a {{.NameLower}} by ID.
func (h *{{.Name}}Handler) Update(c *astra.Ctx) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return astra.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	var req Update{{.Name}}Request
	if err := c.ShouldBindJSON(&req); err != nil {
		return err
	}
	ctx := c.Request.Context()
	_ = ctx // TODO: item, err := h.svc.Update(ctx, id, req)
	_ = id
	return c.JSON(http.StatusOK, astra.Map{"data": req})
}

// Delete removes a {{.NameLower}} by ID.
func (h *{{.Name}}Handler) Delete(c *astra.Ctx) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return astra.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	ctx := c.Request.Context()
	_ = ctx // TODO: err := h.svc.Delete(ctx, id)
	_ = id
	return c.NoContent(http.StatusNoContent)
}
`

const modelSrc = `package model

import "time"

// {{.Name}} represents a {{.NameLower}} entity.
type {{.Name}} struct {
	ID        int64      `json:"id"                     gorm:"primaryKey;autoIncrement"`
	CreatedAt time.Time  `json:"created_at"              gorm:"autoCreateTime"`
	UpdatedAt time.Time  `json:"updated_at"              gorm:"autoUpdateTime"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"      gorm:"index"`
	// TODO: add domain fields
}

// TableName sets the GORM table name.
func ({{.Name}}) TableName() string { return "{{.NameLower}}s" }
`

const dtoSrc = `package dto

// Create{{.Name}}Request is the request body for creating a {{.NameLower}}.
type Create{{.Name}}Request struct {
	// TODO: add fields
}

// Update{{.Name}}Request is the request body for updating a {{.NameLower}}.
type Update{{.Name}}Request struct {
	// TODO: add fields
}

// {{.Name}}Response is the response DTO for a {{.NameLower}}.
type {{.Name}}Response struct {
	ID int64 `json:"id"`
	// TODO: add fields
}
`

// ─── Docker / CI templates ─────────────────────────────────────────────────────

// Dockerfile returns the Dockerfile content.
func Dockerfile() string { return dockerfileSrc }

const dockerfileSrc = `# ── Build stage ──────────────────────────────────────────────────────────────
FROM golang:1.23-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /app/server ./cmd/server

# ── Runtime stage ─────────────────────────────────────────────────────────────
FROM gcr.io/distroless/static:nonroot

WORKDIR /app
COPY --from=builder /app/server .

EXPOSE 8080
ENTRYPOINT ["/app/server"]
`

// RenderDockerCompose generates docker-compose.yml.
func RenderDockerCompose(d tpldata.Data) string {
	return render("dockercompose", dockerComposeSrc, d)
}

const dockerComposeSrc = `version: "3.9"

services:
  app:
    build: .
    ports:
      - "8080:8080"
    environment:
      - DATABASE_DSN=host=postgres user=postgres password={{.NameLower}}_dev_9kF2mP8x dbname={{.NameLower}} sslmode=disable
      - REDIS_ADDR=redis:6379
      - APP_ENV=prod
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
    restart: unless-stopped

  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: {{.NameLower}}
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: {{.NameLower}}_dev_9kF2mP8x
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres"]
      interval: 5s
      timeout: 5s
      retries: 5

  redis:
    image: redis:7-alpine
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 5

volumes:
  pgdata:
`

// RenderCIWorkflow generates .github/workflows/ci.yml.
func RenderCIWorkflow(d tpldata.Data) string {
	year := time.Now().Year()
	return render("ciworkflow", fmt.Sprintf(ciWorkflowSrc, year), d)
}

const ciWorkflowSrc = `name: CI

on:
  push:
    branches: ["main"]
  pull_request:
    branches: ["main"]

jobs:
  ci:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: "1.23"
          cache: true

      - name: Build
        run: go build ./...

      - name: Vet
        run: go vet ./...

      - name: Test
        run: go test ./... -race
`

// ─── OpenAPI endpoint template ─────────────────────────────────────────────────

// RenderEndpoint generates an OpenAPI-derived handler file.
func RenderEndpoint(ops []OpDef, pkg, apiTitle string) (string, error) {
	var buf bytes.Buffer

	fmt.Fprintf(&buf, "// Generated by astra-cli — API: %s\n\n", apiTitle)
	fmt.Fprintf(&buf, "package %s\n\n", pkg)
	fmt.Fprintf(&buf, `import "github.com/astra-go/astra"`+"\n\n")

	// Group by tag
	byTag := make(map[string][]opDef)
	for _, op := range ops {
		byTag[op.Tag] = append(byTag[op.Tag], op)
	}
	tags := make([]string, 0, len(byTag))
	for t := range byTag {
		tags = append(tags, t)
	}
	sort.Strings(tags)

	for _, tag := range tags {
		ops2 := byTag[tag]
		handlerName := tag + "Handler"

		fmt.Fprintf(&buf, "// ─── %s ────────────────────────────────────────────────────────────────\n\n", tag)
		fmt.Fprintf(&buf, "// %s handles %s API endpoints.\n", handlerName, tag)
		fmt.Fprintf(&buf, "type %s struct{}\n\n", handlerName)

		for _, op := range ops2 {
			fmt.Fprintf(&buf, "// %s handles %s %s.\n", op.FuncName, op.Method, op.Path)
			if op.Summary != "" {
				fmt.Fprintf(&buf, "// %s\n", op.Summary)
			}
			fmt.Fprintf(&buf, "func (h *%s) %s(c *astra.Ctx) error {\n", handlerName, op.FuncName)
			if op.Request != "" {
				fmt.Fprintf(&buf, "\tvar req %s\n", op.Request)
				fmt.Fprintf(&buf, "\tif err := c.ShouldBindJSON(&req); err != nil {\n")
				fmt.Fprintf(&buf, "\t\treturn err\n")
				fmt.Fprintf(&buf, "\t}\n")
			}
			fmt.Fprintf(&buf, "\t_ = c.Request.Context()\n")
			fmt.Fprintf(&buf, "\t// TODO: implement\n")
			if op.Response != "" {
				fmt.Fprintf(&buf, "\treturn c.JSON(200, %s{})\n", op.Response)
			} else {
				fmt.Fprintf(&buf, "\treturn nil\n")
			}
			fmt.Fprintf(&buf, "}\n\n")
		}

		fmt.Fprintf(&buf, "// Register mounts the %s routes on the given router group.\n", tag)
		fmt.Fprintf(&buf, "func (h *%s) Register(g *astra.Group) {\n", handlerName)
		for _, op := range ops2 {
			fmt.Fprintf(&buf, "\tg.%s(%q, h.%s)\n", op.Method, op.Path, op.FuncName)
		}
		fmt.Fprintf(&buf, "}\n\n")
	}

	return buf.String(), nil
}

// opDef holds OpenAPI operation metadata for handler generation.
type OpDef struct {
	Method   string
	Path     string
	FuncName string
	Summary  string
	Tag      string
	Request  string
	Response string
}
`