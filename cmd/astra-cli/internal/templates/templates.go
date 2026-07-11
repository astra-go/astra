// Package templates provides all code-generation templates for astra-cli.
package templates

import (
	"bytes"
	"fmt"
	"sort"
	"text/template"

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

const makefileSrc = `BINARY := {{.NameLower}}
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

// RenderCRUDRepo generates a repository file.

// RenderCRUDHandler generates a CRUD handler file.

// RenderCRUDService generates a service layer file.

// RenderDTO generates a DTO file with request/response structs.
func RenderDTO(d tpldata.Data) string {
	return render("dto", dtoSrc, d)
}

const serviceSrc = "package service" +
"" +
"import \"context\"" +
"" +
"// ─── DTOs ─────────────────────────────────────────────────────────────────────" +
"" +
"// Create{{.Name}}Request is the input for creating a {{.NameLower}}." +
"type Create{{.Name}}Request struct {" +
"	// TODO: add fields" +
"}" +
"" +
"// Update{{.Name}}Request is the input for updating a {{.NameLower}}." +
"type Update{{.Name}}Request struct {" +
"	// TODO: add fields" +
"}" +
"" +
"// {{.Name}}Response is the output DTO for {{.NameLower}} operations." +
"type {{.Name}}Response struct {" +
"	ID int64 " +
"\x60" +
"json:\"id\"" +
"\x60" +
"	// TODO: add fields" +
"}" +
"" +
"// ─── Interface ────────────────────────────────────────────────────────────────" +
"" +
"// {{.Name}}Service defines the business-logic interface for {{.NameLower}} operations." +
"type {{.Name}}Service interface {" +
"	List(ctx context.Context, page, limit int, keyword string) ([]*{{.Name}}Response, int64, error)" +
"	Get(ctx context.Context, id int64) (*{{.Name}}Response, error)" +
"	Create(ctx context.Context, req *Create{{.Name}}Request) (*{{.Name}}Response, error)" +
"	Update(ctx context.Context, id int64, req *Update{{.Name}}Request) (*{{.Name}}Response, error)" +
"	Delete(ctx context.Context, id int64) error" +
"}" +
"" +
"// ─── Implementation ───────────────────────────────────────────────────────────" +
"" +
"// {{.Name}}ServiceImpl is the default implementation of {{.Name}}Service." +
"type {{.Name}}ServiceImpl struct {" +
"	// TODO: inject repository" +
"}" +
"" +
"// New{{.Name}}Service creates a new {{.Name}}ServiceImpl." +
"func New{{.Name}}Service() *{{.Name}}ServiceImpl {" +
"	return &{{.Name}}ServiceImpl{}" +
"}" +
"" +
"func (s *{{.Name}}ServiceImpl) List(ctx context.Context, page, limit int, keyword string) ([]*{{.Name}}Response, int64, error) {" +
"	// TODO: implement" +
"	return nil, 0, nil" +
"}" +
"" +
"func (s *{{.Name}}ServiceImpl) Get(ctx context.Context, id int64) (*{{.Name}}Response, error) {" +
"	// TODO: implement" +
"	return nil, nil" +
"}" +
"" +
"func (s *{{.Name}}ServiceImpl) Create(ctx context.Context, req *Create{{.Name}}Request) (*{{.Name}}Response, error) {" +
"	// TODO: implement" +
"	return nil, nil" +
"}" +
"" +
"func (s *{{.Name}}ServiceImpl) Update(ctx context.Context, id int64, req *Update{{.Name}}Request) (*{{.Name}}Response, error) {" +
"	// TODO: implement" +
"	return nil, nil" +
"}" +
"" +
"func (s *{{.Name}}ServiceImpl) Delete(ctx context.Context, id int64) error {" +
"	// TODO: implement" +
"	return nil" +
"}"

const handlerSrc = "package handler" +
"" +
"import (" +
"	\"net/http\"" +
"	\"strconv\"" +
"" +
"	\"github.com/astra-go/astra\"" +
")" +
"" +
"// {{.Name}}Handler handles {{.NameLower}}-related HTTP requests." +
"type {{.Name}}Handler struct {" +
"	// TODO: inject service" +
"	// svc {{.Name}}Service" +
"}" +
"" +
"// New{{.Name}}Handler creates a new {{.Name}}Handler." +
"func New{{.Name}}Handler() *{{.Name}}Handler {" +
"	return &{{.Name}}Handler{}" +
"}" +
"" +
"// Register mounts all {{.NameLower}} routes onto the given route group." +
"func (h *{{.Name}}Handler) Register(g *astra.Group) {" +
"	g.GET(\"/{{.NameLower}}s\",         h.List)" +
"	g.POST(\"/{{.NameLower}}s\",        h.Create)" +
"	g.GET(\"/{{.NameLower}}s/:id\",     h.Get)" +
"	g.PUT(\"/{{.NameLower}}s/:id\",     h.Update)" +
"	g.DELETE(\"/{{.NameLower}}s/:id\",  h.Delete)" +
"}" +
"" +
"// ─── DTOs ─────────────────────────────────────────────────────────────────────" +
"" +
"// {{.Name}}ListQuery holds pagination and filter parameters." +
"type {{.Name}}ListQuery struct {" +
"	Page    int    " +
"\x60" +
"form:\"page\"    validate:\"min=1\"" +
"\x60" +
"	Limit   int    " +
"\x60" +
"form:\"limit\"   validate:\"min=1,max=100\"" +
"\x60" +
"	Keyword string " +
"\x60" +
"form:\"keyword\"" +
"\x60" +
"}" +
"" +
"// Create{{.Name}}Request is the request body for creating a {{.NameLower}}." +
"type Create{{.Name}}Request struct {" +
"	// TODO: add fields, e.g.:" +
"	// Name string " +
"\x60" +
"json:\"name\" validate:\"required,min=2,max=100\"" +
"\x60" +
"}" +
"" +
"// Update{{.Name}}Request is the request body for updating a {{.NameLower}}." +
"type Update{{.Name}}Request struct {" +
"	// TODO: add fields" +
"}" +
"" +
"// ─── Handlers ─────────────────────────────────────────────────────────────────" +
"" +
"// List returns a paginated list of {{.NameLower}}s." +
"func (h *{{.Name}}Handler) List(c *astra.Ctx) error {" +
"	var q {{.Name}}ListQuery" +
"	if err := c.ShouldBindQuery(&q); err != nil {" +
"		return err" +
"	}" +
"	if q.Page == 0 {" +
"		q.Page = 1" +
"	}" +
"	if q.Limit == 0 {" +
"		q.Limit = 20" +
"	}" +
"	ctx := c.Request.Context()" +
"	_ = ctx // TODO: items, total, err := h.svc.List(ctx, q.Page, q.Limit, q.Keyword)" +
"	return c.JSON(http.StatusOK, astra.Map{\"data\": []any{}, \"total\": 0, \"page\": q.Page, \"limit\": q.Limit})" +
"}" +
"" +
"// Create creates a new {{.NameLower}}." +
"func (h *{{.Name}}Handler) Create(c *astra.Ctx) error {" +
"	var req Create{{.Name}}Request" +
"	if err := c.ShouldBindJSON(&req); err != nil {" +
"		return err" +
"	}" +
"	ctx := c.Request.Context()" +
"	_ = ctx // TODO: item, err := h.svc.Create(ctx, req)" +
"	return c.JSON(http.StatusCreated, astra.Map{\"data\": req})" +
"}" +
"" +
"// Get returns a {{.NameLower}} by ID." +
"func (h *{{.Name}}Handler) Get(c *astra.Ctx) error {" +
"	id, err := strconv.ParseInt(c.Param(\"id\"), 10, 64)" +
"	if err != nil {" +
"		return astra.NewHTTPError(http.StatusBadRequest, \"invalid id\")" +
"	}" +
"	ctx := c.Request.Context()" +
"	_ = ctx // TODO: item, err := h.svc.Get(ctx, id)" +
"	return c.JSON(http.StatusOK, astra.Map{\"id\": id})" +
"}" +
"" +
"// Update updates a {{.NameLower}} by ID." +
"func (h *{{.Name}}Handler) Update(c *astra.Ctx) error {" +
"	id, err := strconv.ParseInt(c.Param(\"id\"), 10, 64)" +
"	if err != nil {" +
"		return astra.NewHTTPError(http.StatusBadRequest, \"invalid id\")" +
"	}" +
"	var req Update{{.Name}}Request" +
"	if err := c.ShouldBindJSON(&req); err != nil {" +
"		return err" +
"	}" +
"	ctx := c.Request.Context()" +
"	_ = ctx // TODO: item, err := h.svc.Update(ctx, id, req)" +
"	_ = id" +
"	return c.JSON(http.StatusOK, astra.Map{\"data\": req})" +
"}" +
"" +
"// Delete removes a {{.NameLower}} by ID." +
"func (h *{{.Name}}Handler) Delete(c *astra.Ctx) error {" +
"	id, err := strconv.ParseInt(c.Param(\"id\"), 10, 64)" +
"	if err != nil {" +
"		return astra.NewHTTPError(http.StatusBadRequest, \"invalid id\")" +
"	}" +
"	ctx := c.Request.Context()" +
"	_ = ctx // TODO: err := h.svc.Delete(ctx, id)" +
"	_ = id" +
"	return c.NoContent(http.StatusNoContent)" +
"}"

const modelSrc = "package model" +
"" +
"import \"time\"" +
"" +
"// {{.Name}} represents a {{.NameLower}} entity." +
"type {{.Name}} struct {" +
"	ID        int64      " +
"\x60" +
"json:\"id\"                     gorm:\"primaryKey;autoIncrement\"" +
"\x60" +
"	CreatedAt time.Time  " +
"\x60" +
"json:\"created_at\"              gorm:\"autoCreateTime\"" +
"\x60" +
"	UpdatedAt time.Time  " +
"\x60" +
"json:\"updated_at\"              gorm:\"autoUpdateTime\"" +
"\x60" +
"	DeletedAt *time.Time " +
"\x60" +
"json:\"deleted_at,omitempty\"      gorm:\"index\"" +
"\x60" +
"	// TODO: add domain fields" +
"}" +
"" +
"// TableName sets the GORM table name." +
"func ({{.Name}}) TableName() string { return \"{{.NameLower}}s\" }"

const dtoSrc = "package dto" +
"" +
"// Create{{.Name}}Request is the request body for creating a {{.NameLower}}." +
"type Create{{.Name}}Request struct {" +
"	// TODO: add fields" +
"}" +
"" +
"// Update{{.Name}}Request is the request body for updating a {{.NameLower}}." +
"type Update{{.Name}}Request struct {" +
"	// TODO: add fields" +
"}" +
"" +
"// {{.Name}}Response is the response DTO for a {{.NameLower}}." +
"type {{.Name}}Response struct {" +
"	ID int64 " +
"\x60" +
"json:\"id\"" +
"\x60" +
"	// TODO: add fields" +
"}"

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
	return render("ciworkflow", ciWorkflowSrc, d)
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
	byTag := make(map[string][]OpDef)
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

// OpDef holds OpenAPI operation metadata for handler generation.
type OpDef struct {
	Method   string
	Path     string
	FuncName string
	Summary  string
	Tag      string
	Request  string
	Response string
}
// GoMod returns the go.mod template content.
func GoMod() string {
	return `module {{.NameLower}}

go 1.23

require (
	github.com/astra-go/astra v1.0.6
	github.com/spf13/cast v1.7.0
	github.com/spf13/viper v1.19.0
	gorm.io/driver/mysql v1.5.7
	gorm.io/gorm v1.25.12
)

require (
	github.com/fsnotify/fsnotify v1.8.0 // indirect
	github.com/go-sql-driver/mysql v1.8.1 // indirect
	github.com/hashicorp/hcl v1.0.3 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/magiconair/properties v1.8.9 // indirect
	github.com/mitchellh/mapstructure v1.1.2 // indirect
	github.com/pelletier/go-toml/v2 v2.2.3 // indirect
	github.com/sagikazarmark/locafero v0.4.0 // indirect
	github.com/sagikazarmark/slog-shim v0.1.0 // indirect
	github.com/sourcegraph/conc v0.3.0 // indirect
	github.com/spf13/afero v1.12.0 // indirect
	github.com/spf13/cast v1.7.0 // indirect
	github.com/spf13/pflag v1.0.6 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/exp v0.0.0-20240909161429-701f63a606c0 // indirect
	golang.org/x/sys v0.28.0 // indirect
	golang.org/x/text v0.21.0 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
`
}

// Makefile returns the Makefile content.
func Makefile() string { return RenderMakefile(tpldata.Data{Name: "app", NameLower: "app"}) }


// ─── Middleware template ──────────────────────────────────────────────────────

var middlewareSrc = "package middleware\n\nimport (\n\t\"net/http\"\n\t\"time\"\n\n\t\"github.com/astra-go/astra\"\n)\n\n// {{.Name}} creates a {{.NameLower}} middleware.\nfunc {{.Name}}() astra.MiddlewareFunc {\n\treturn func(next astra.HandlerFunc) astra.HandlerFunc {\n\t\treturn func(c *astra.Ctx) error {\n\t\t\t// TODO: implement {{.NameLower}} logic\n\t\t\treturn next(c)\n\t\t}\n\t}\n}\n"

// RenderMiddleware generates a middleware file.
func RenderMiddleware(d tpldata.Data) string {
	return render("middleware", middlewareSrc, d)
}

// ─── CRUD templates ───────────────────────────────────────────────────────────

var crudModelSrc = "package model\\n\\n" +
	"import (\\n\\t\"time\"\\n)\\n\\n" +
	"// {{.Name}} represents a {{.NameLower}} entity.\\n" +
	"type {{.Name}} struct {\\n" +
	"\\tID        int64      " + "\x60" + "json:\"id\" gorm:\"primaryKey;autoIncrement\" " + "\x60" + "\\n" +
	"\\tCreatedAt time.Time " + "\x60" + "json:\"created_at\" gorm:\"autoCreateTime\" " + "\x60" + "\\n" +
	"\\tUpdatedAt time.Time " + "\x60" + "json:\"updated_at\" gorm:\"autoUpdateTime\" " + "\x60" + "\\n" +
	"\\tDeletedAt *time.Time " + "\x60" + "json:\"deleted_at,omitempty\" gorm:\"index\" " + "\x60" + "\\n" +
	"\\t// TODO: add domain fields\\n" +
	"}\\n\\n" +
	"// TableName sets the GORM table name.\\n" +
	"func ({{.Name}}) TableName() string { return \"{{.NameLower}}s\" }\\n"

var crudRepoSrc = "package repository\n\nimport (\n\t\"context\"\n\n\t\"github.com/astra-go/astra\"\n\t\"github.com/astra-go/astra/examples/{{.NameLower}}/internal/model\"\n\t\"gorm.io/gorm\"\n)\n\n// {{.Name}}Repository defines the data-access interface.\ntype {{.Name}}Repository interface {\n\tCreate(ctx context.Context, m *model.{{.Name}}) error\n\tGetByID(ctx context.Context, id int64) (*model.{{.Name}}, error)\n\tUpdate(ctx context.Context, m *model.{{.Name}}) error\n\tDelete(ctx context.Context, id int64) error\n\tList(ctx context.Context, offset, limit int, keyword string) ([]*model.{{.Name}}, int64, error)\n}\n\ntype gorm{{.Name}}Repository struct {\n\tdb *gorm.DB\n}\n\nfunc New{{.Name}}Repository(db *gorm.DB) {{.Name}}Repository {\n\treturn &gorm{{.Name}}Repository{db: db}\n}\n\nfunc (r *gorm{{.Name}}Repository) Create(ctx context.Context, m *model.{{.Name}}) error {\n\treturn r.db.WithContext(ctx).Create(m).Error\n}\n\nfunc (r *gorm{{.Name}}Repository) GetByID(ctx context.Context, id int64) (*model.{{.Name}}, error) {\n\tvar m model.{{.Name}}\n\tif err := r.db.WithContext(ctx).First(&m, id).Error; err != nil {\n\t\treturn nil, err\n\t}\n\treturn &m, nil\n}\n\nfunc (r *gorm{{.Name}}Repository) Update(ctx context.Context, m *model.{{.Name}}) error {\n\treturn r.db.WithContext(ctx).Save(m).Error\n}\n\nfunc (r *gorm{{.Name}}Repository) Delete(ctx context.Context, id int64) error {\n\treturn r.db.WithContext(ctx).Delete(&model.{{.Name}}{}, id).Error\n}\n\nfunc (r *gorm{{.Name}}Repository) List(ctx context.Context, offset, limit int, keyword string) ([]*model.{{.Name}}, int64, error) {\n\tvar ms []*model.{{.Name}}\n\tvar total int64\n\tq := r.db.WithContext(ctx).Model(&model.{{.Name}}{})\n\tif keyword != \"\" {\n\t\tq = q.Where(\"name LIKE ?\", \"%\"+keyword+\"%\")\n\t}\n\tif err := q.Count(&total).Error; err != nil {\n\t\treturn nil, 0, err\n\t}\n\tif err := q.Offset(offset).Limit(limit).Find(&ms).Error; err != nil {\n\t\treturn nil, 0, err\n\t}\n\treturn ms, total, nil\n}\n"

var crudHandlerSrc = "package handler\n\nimport (\n\t\"net/http\"\n\t\"strconv\"\n\n\t\"github.com/astra-go/astra\"\n\t\"github.com/astra-go/astra/examples/{{.NameLower}}/internal/dto\"\n\t\"github.com/astra-go/astra/examples/{{.NameLower}}/internal/repository\"\n)\n\n// {{.Name}}Handler handles HTTP requests for {{.NameLower}}.\ntype {{.Name}}Handler struct {\n\trepo repository.{{.Name}}Repository\n}\n\nfunc New{{.Name}}Handler(repo repository.{{.Name}}Repository) *{{.Name}}Handler {\n\treturn &{{.Name}}Handler{repo: repo}\n}\n\nfunc (h *{{.Name}}Handler) Register(g *astra.RouterGroup) {\n\tg.GET(\"/{{.NameLower}}s\", h.List)\n\tg.POST(\"/{{.NameLower}}s\", h.Create)\n\tg.GET(\"/{{.NameLower}}s/:id\", h.Get)\n\tg.PUT(\"/{{.NameLower}}s/:id\", h.Update)\n\tg.DELETE(\"/{{.NameLower}}s/:id\", h.Delete)\n}\n\nfunc (h *{{.Name}}Handler) List(c *astra.Ctx) error {\n\tpage, _ := strconv.Atoi(c.DefaultQuery(\"page\", \"1\"))\n\tlimit, _ := strconv.Atoi(c.DefaultQuery(\"limit\", \"20\"))\n\tkeyword := c.Query(\"keyword\")\n\tif page < 1 { page = 1 }\n\tif limit < 1 || limit > 100 { limit = 20 }\n\toffset := (page - 1) * limit\n\tms, total, err := h.repo.List(c.Request.Context(), offset, limit, keyword)\n\tif err != nil { return err }\n\treturn c.JSON(http.StatusOK, astra.Map{\"data\": ms, \"total\": total, \"page\": page, \"limit\": limit})\n}\n\nfunc (h *{{.Name}}Handler) Get(c *astra.Ctx) error {\n\tid, err := strconv.ParseInt(c.Param(\"id\"), 10, 64)\n\tif err != nil { return astra.NewHTTPError(http.StatusBadRequest, \"invalid id\") }\n\tm, err := h.repo.GetByID(c.Request.Context(), id)\n\tif err != nil { return err }\n\treturn c.JSON(http.StatusOK, astra.Map{\"data\": m})\n}\n\nfunc (h *{{.Name}}Handler) Create(c *astra.Ctx) error {\n\tvar req dto.Create{{.Name}}Request\n\tif err := c.ShouldBindJSON(&req); err != nil { return err }\n\t_ = req\n\treturn c.NoContent(http.StatusCreated)\n}\n\nfunc (h *{{.Name}}Handler) Update(c *astra.Ctx) error {\n\tid, err := strconv.ParseInt(c.Param(\"id\"), 10, 64)\n\tif err != nil { return astra.NewHTTPError(http.StatusBadRequest, \"invalid id\") }\n\tvar req dto.Update{{.Name}}Request\n\tif err := c.ShouldBindJSON(&req); err != nil { return err }\n\t_ = id; _ = req\n\treturn c.NoContent(http.StatusNoContent)\n}\n\nfunc (h *{{.Name}}Handler) Delete(c *astra.Ctx) error {\n\tid, err := strconv.ParseInt(c.Param(\"id\"), 10, 64)\n\tif err != nil { return astra.NewHTTPError(http.StatusBadRequest, \"invalid id\") }\n\tif err := h.repo.Delete(c.Request.Context(), id); err != nil { return err }\n\treturn c.NoContent(http.StatusNoContent)\n}\n"

var crudServiceSrc = "package service\n\nimport (\n\t\"context\"\n\n\t\"github.com/astra-go/astra/examples/{{.NameLower}}/internal/dto\"\n\t\"github.com/astra-go/astra/examples/{{.NameLower}}/internal/model\"\n\t\"github.com/astra-go/astra/examples/{{.NameLower}}/internal/repository\"\n)\n\n// {{.Name}}Service defines the business-logic interface for {{.NameLower}}.\ntype {{.Name}}Service interface {\n\tList(ctx context.Context, page, limit int, keyword string) ([]*model.{{.Name}}, int64, error)\n\tGet(ctx context.Context, id int64) (*model.{{.Name}}, error)\n\tCreate(ctx context.Context, req *dto.Create{{.Name}}Request) (*model.{{.Name}}, error)\n\tUpdate(ctx context.Context, id int64, req *dto.Update{{.Name}}Request) (*model.{{.Name}}, error)\n\tDelete(ctx context.Context, id int64) error\n}\n\ntype {{.NameLower}}Service struct {\n\trepo repository.{{.Name}}Repository\n}\n\nfunc New{{.Name}}Service(repo repository.{{.Name}}Repository) {{.Name}}Service {\n\treturn &{{.NameLower}}Service{repo: repo}\n}\n\nfunc (s *{{.NameLower}}Service) List(ctx context.Context, page, limit int, keyword string) ([]*model.{{.Name}}, int64, error) {\n\treturn s.repo.List(ctx, (page-1)*limit, limit, keyword)\n}\nfunc (s *{{.NameLower}}Service) Get(ctx context.Context, id int64) (*model.{{.Name}}, error) {\n\treturn s.repo.GetByID(ctx, id)\n}\nfunc (s *{{.NameLower}}Service) Create(ctx context.Context, req *dto.Create{{.Name}}Request) (*model.{{.Name}}, error) {\n\tm := &model.{{.Name}}{}\n\tif err := s.repo.Create(ctx, m); err != nil { return nil, err }\n\treturn m, nil\n}\nfunc (s *{{.NameLower}}Service) Update(ctx context.Context, id int64, req *dto.Update{{.Name}}Request) (*model.{{.Name}}, error) {\n\tm, err := s.repo.GetByID(ctx, id)\n\tif err != nil { return nil, err }\n\t_ = m; _ = req\n\tif err := s.repo.Update(ctx, m); err != nil { return nil, err }\n\treturn m, nil\n}\nfunc (s *{{.NameLower}}Service) Delete(ctx context.Context, id int64) error {\n\treturn s.repo.Delete(ctx, id)\n}\n"

func RenderCRUDModel(d tpldata.Data) string {
	return render("crudModel", crudModelSrc, d)
}

func RenderCRUDRepo(d tpldata.Data) string {
	return render("crudRepo", crudRepoSrc, d)
}

func RenderCRUDHandler(d tpldata.Data) string {
	return render("crudHandler", crudHandlerSrc, d)
}

func RenderCRUDService(d tpldata.Data) string {
	return render("crudService", crudServiceSrc, d)
}
