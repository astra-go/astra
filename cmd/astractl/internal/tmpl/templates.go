// Package tmpl provides all code-generation templates for astractl.
// Templates are initialised lazily (sync.Once) so a parse error surfaces at
// first use with a clear message rather than panicking at program startup.
package tmpl

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"text/template"
)

// templateDir, when non-empty, is checked before embedded templates.
// Set via SetDir before any generator runs.
var templateDir string

// SetDir configures a directory to load custom .tmpl files from.
// A file named <name>.tmpl in that directory overrides the embedded template.
// Must be called before any template accessor (e.g. before dispatching to a gen subcommand).
func SetDir(dir string) { templateDir = dir }

// tryFromDir attempts to load <templateDir>/<name>.tmpl from disk.
// Returns nil if templateDir is unset or the file does not exist.
func tryFromDir(name string) *template.Template {
	if templateDir == "" {
		return nil
	}
	path := filepath.Join(templateDir, name+".tmpl")
	src, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	t, err := template.New(name).Parse(string(src))
	if err != nil {
		fmt.Fprintf(os.Stderr, "[error] custom template %q parse error: %s\n", path, err)
		fmt.Fprintf(os.Stderr, "  hint: fix the template syntax in %s\n", path)
		os.Exit(1)
	}
	return t
}

var (
	onceMain      sync.Once
	onceRoutes    sync.Once
	onceMainDDD   sync.Once
	onceGoMod     sync.Once
	onceConfig    sync.Once
	onceConfigDev sync.Once
	onceProd      sync.Once
	onceGitignore sync.Once
	onceDockerfile     sync.Once
	onceDockerCompose  sync.Once
	onceMakefile       sync.Once
	onceHandler         sync.Once
	onceHandlerWithSvc  sync.Once
	onceService     sync.Once
	onceModel       sync.Once
	onceMiddleware  sync.Once
	onceRepo        sync.Once
	onceMigration   sync.Once
	onceWireProvider sync.Once
	onceDIContainer  sync.Once
	onceErrorCodes   sync.Once
	onceHandlerTest  sync.Once
	onceCIWorkflow   sync.Once

	tplMain           *template.Template
	tplRoutes         *template.Template
	tplMainDDD        *template.Template
	tplGoMod          *template.Template
	tplConfig         *template.Template
	tplConfigDev      *template.Template
	tplConfigProd     *template.Template
	tplGitignore      *template.Template
	tplDockerfile     *template.Template
	tplDockerCompose  *template.Template
	tplMakefile       *template.Template
	tplHandler         *template.Template
	tplHandlerWithSvc  *template.Template
	tplService     *template.Template
	tplModel       *template.Template
	tplMiddleware  *template.Template
	tplRepo        *template.Template
	tplMigration   *template.Template
	tplWireProvider *template.Template
	tplDIContainer  *template.Template
	tplErrorCodes   *template.Template
	tplHandlerTest  *template.Template
	tplCIWorkflow   *template.Template
)

func must(name, src string) *template.Template {
	t, err := template.New(name).Parse(src)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[error] internal: template %q failed to parse: %s\n", name, err)
		fmt.Fprintf(os.Stderr, "  hint: this is a bug in astractl — please report it at https://github.com/astra-go/astra/issues\n")
		os.Exit(1)
	}
	return t
}

func Main() *template.Template {
	if t := tryFromDir("main"); t != nil { return t }
	onceMain.Do(func() { tplMain = must("main", mainSrc) })
	return tplMain
}

func Routes() *template.Template {
	if t := tryFromDir("routes"); t != nil { return t }
	onceRoutes.Do(func() { tplRoutes = must("routes", routesSrc) })
	return tplRoutes
}

func MainDDD() *template.Template {
	if t := tryFromDir("mainDDD"); t != nil { return t }
	onceMainDDD.Do(func() { tplMainDDD = must("mainDDD", mainDDDSrc) })
	return tplMainDDD
}

func GoMod() *template.Template {
	if t := tryFromDir("gomod"); t != nil { return t }
	onceGoMod.Do(func() { tplGoMod = must("gomod", goModSrc) })
	return tplGoMod
}

func Config() *template.Template {
	if t := tryFromDir("config"); t != nil { return t }
	onceConfig.Do(func() { tplConfig = must("config", configSrc) })
	return tplConfig
}

func ConfigDev() *template.Template {
	if t := tryFromDir("configDev"); t != nil { return t }
	onceConfigDev.Do(func() { tplConfigDev = must("configDev", configDevSrc) })
	return tplConfigDev
}

func ConfigProd() *template.Template {
	if t := tryFromDir("configProd"); t != nil { return t }
	onceProd.Do(func() { tplConfigProd = must("configProd", configProdSrc) })
	return tplConfigProd
}

func Gitignore() *template.Template {
	if t := tryFromDir("gitignore"); t != nil { return t }
	onceGitignore.Do(func() { tplGitignore = must("gitignore", gitignoreSrc) })
	return tplGitignore
}

func Dockerfile() *template.Template {
	if t := tryFromDir("dockerfile"); t != nil { return t }
	onceDockerfile.Do(func() { tplDockerfile = must("dockerfile", dockerfileSrc) })
	return tplDockerfile
}

func DockerCompose() *template.Template {
	if t := tryFromDir("dockercompose"); t != nil { return t }
	onceDockerCompose.Do(func() { tplDockerCompose = must("dockercompose", dockerComposeSrc) })
	return tplDockerCompose
}

func Makefile() *template.Template {
	if t := tryFromDir("makefile"); t != nil { return t }
	onceMakefile.Do(func() { tplMakefile = must("makefile", makefileSrc) })
	return tplMakefile
}

func Handler() *template.Template {
	if t := tryFromDir("handler"); t != nil { return t }
	onceHandler.Do(func() { tplHandler = must("handler", handlerSrc) })
	return tplHandler
}

func HandlerWithService() *template.Template {
	if t := tryFromDir("handlerWithService"); t != nil { return t }
	onceHandlerWithSvc.Do(func() { tplHandlerWithSvc = must("handlerWithService", handlerWithServiceSrc) })
	return tplHandlerWithSvc
}

func Service() *template.Template {
	if t := tryFromDir("service"); t != nil { return t }
	onceService.Do(func() { tplService = must("service", serviceSrc) })
	return tplService
}

func Model() *template.Template {
	if t := tryFromDir("model"); t != nil { return t }
	onceModel.Do(func() { tplModel = must("model", modelSrc) })
	return tplModel
}

func Middleware() *template.Template {
	if t := tryFromDir("middleware"); t != nil { return t }
	onceMiddleware.Do(func() { tplMiddleware = must("middleware", middlewareSrc) })
	return tplMiddleware
}

func Repo() *template.Template {
	if t := tryFromDir("repo"); t != nil { return t }
	onceRepo.Do(func() { tplRepo = must("repo", repoSrc) })
	return tplRepo
}

func Migration() *template.Template {
	if t := tryFromDir("migration"); t != nil { return t }
	onceMigration.Do(func() { tplMigration = must("migration", migrationSrc) })
	return tplMigration
}

func WireProvider() *template.Template {
	if t := tryFromDir("wire"); t != nil { return t }
	onceWireProvider.Do(func() { tplWireProvider = must("wire", wireProviderSrc) })
	return tplWireProvider
}

func DIContainer() *template.Template {
	if t := tryFromDir("diContainer"); t != nil { return t }
	onceDIContainer.Do(func() { tplDIContainer = must("diContainer", diContainerSrc) })
	return tplDIContainer
}

func ErrorCodes() *template.Template {
	if t := tryFromDir("errors"); t != nil { return t }
	onceErrorCodes.Do(func() { tplErrorCodes = must("errors", errorCodesSrc) })
	return tplErrorCodes
}

func HandlerTest() *template.Template {
	if t := tryFromDir("handlerTest"); t != nil { return t }
	onceHandlerTest.Do(func() { tplHandlerTest = must("handlerTest", handlerTestSrc) })
	return tplHandlerTest
}

func CIWorkflow() *template.Template {
	if t := tryFromDir("ciWorkflow"); t != nil { return t }
	onceCIWorkflow.Do(func() { tplCIWorkflow = must("ciWorkflow", ciWorkflowSrc) })
	return tplCIWorkflow
}

// ─── Template source strings ──────────────────────────────────────────────────

const mainSrc = `package main

import (
	"net/http"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/middleware"
)

func main() {
	app := astra.New(
		astra.WithMode(astra.ModeProd),
		astra.WithShutdownTimeout(30),
	)

	app.Use(
		middleware.RequestID(),
		middleware.Logger(),
		middleware.Recovery(),
		middleware.CORS(),
	)

	// Kubernetes health probes — must NOT sit behind auth middleware.
	app.GET("/health/live", func(c *astra.Context) error {
		return c.JSON(http.StatusOK, astra.Map{"status": "ok"})
	})
	app.GET("/health/ready", func(c *astra.Context) error {
		// TODO: check downstream dependencies (DB, Redis) and return 503 if unhealthy.
		return c.JSON(http.StatusOK, astra.Map{"status": "ok"})
	})

	registerRoutes(app)

	app.Run(":8080")
}
`

const routesSrc = `package main

import "github.com/astra-go/astra"

func registerRoutes(app *astra.App) {
	v1 := app.Group("/api/v1")
	_ = v1
	// TODO: register your routes here
	// Example:
	//   h := handler.NewUserHandler(db)
	//   h.Register(v1)
}
`

const mainDDDSrc = `package main

import (
	"net/http"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/middleware"
)

func main() {
	app := astra.New(
		astra.WithMode(astra.ModeProd),
		astra.WithShutdownTimeout(30),
	)

	app.Use(
		middleware.RequestID(),
		middleware.Logger(),
		middleware.Recovery(),
		middleware.CORS(),
	)

	// Kubernetes health probes — must NOT sit behind auth middleware.
	app.GET("/health/live", func(c *astra.Context) error {
		return c.JSON(http.StatusOK, astra.Map{"status": "ok"})
	})
	app.GET("/health/ready", func(c *astra.Context) error {
		// TODO: check DB/Redis connectivity; return 503 on failure.
		return c.JSON(http.StatusOK, astra.Map{"status": "ok"})
	})

	v1 := app.Group("/api/v1")
	_ = v1 // TODO: wire handlers — e.g. handler.NewUserHandler(...).Register(v1)

	app.Run(":8080")
}
`

const goModSrc = `module {{.Module}}

go 1.23

require (
	github.com/astra-go/astra v1.0.0
	gopkg.in/yaml.v3 v3.0.1
)
`

const configSrc = `server:
  port: 8080
  mode: prod          # dev | staging | prod | test
  shutdown_timeout: 10

database:
  dsn: "host=localhost user=postgres password='' dbname={{.NameLower}} sslmode=disable"
  max_open: 25
  max_idle: 5

cache:
  redis_addr: "localhost:6379"
  key_prefix: "{{.NameLower}}:"

log:
  level: info
  format: json
`

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

const gitignoreSrc = `# Binary
/{{.NameLower}}
*.exe
*.exe~
*.dll
*.so
*.dylib

# Test binary
*.test

# Coverage
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

const dockerfileSrc = `# ── Build stage ──────────────────────────────────────────────────────────────
FROM golang:1.23-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /app/server .

# ── Runtime stage ─────────────────────────────────────────────────────────────
FROM gcr.io/distroless/static:nonroot

WORKDIR /app
COPY --from=builder /app/server .

EXPOSE 8080
ENTRYPOINT ["/app/server"]
`

const dockerComposeSrc = `version: "3.9"

services:
  app:
    build: .
    ports:
      - "8080:8080"
    environment:
      - DATABASE_DSN=host=postgres user=postgres password=postgres dbname={{.NameLower}} sslmode=disable
      - REDIS_ADDR=redis:6379
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
      POSTGRES_PASSWORD: postgres
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

const makefileSrc = `BINARY := {{.NameLower}}
IMAGE  := {{.NameLower}}:latest
GO     := go
PORT   := 8080

.PHONY: build run test lint clean tidy docker-build docker-run docker-stop

build:
	$(GO) build -o $(BINARY) .

run:
	$(GO) run .

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

const handlerSrc = `package {{.Pkg}}

import (
	"net/http"
	"strconv"

	"github.com/astra-go/astra"
)

// {{.Name}}Handler handles {{.NameLower}}-related HTTP requests.
type {{.Name}}Handler struct {
	// TODO: inject service/repository dependencies
	// svc {{.Name}}Service
}

// New{{.Name}}Handler creates a new {{.Name}}Handler.
func New{{.Name}}Handler( /* svc {{.Name}}Service */ ) *{{.Name}}Handler {
	return &{{.Name}}Handler{}
}

// Register mounts all {{.NameLower}} routes onto the given route group.
func (h *{{.Name}}Handler) Register(g *astra.Group) {
	g.GET("/{{.NameLower}}s",        h.List)
	g.POST("/{{.NameLower}}s",       h.Create)
	g.GET("/{{.NameLower}}s/:id",    h.Get)
	g.PUT("/{{.NameLower}}s/:id",    h.Update)
	g.DELETE("/{{.NameLower}}s/:id", h.Delete)
}

// ─── DTOs ─────────────────────────────────────────────────────────────────────

// {{.Name}}ListQuery holds pagination and filter parameters.
type {{.Name}}ListQuery struct {
	Page    int    ` + "`" + `form:"page"    validate:"min=1"` + "`" + `
	Limit   int    ` + "`" + `form:"limit"   validate:"min=1,max=100"` + "`" + `
	Keyword string ` + "`" + `form:"keyword"` + "`" + `
}

// Create{{.Name}}Request is the request body for creating a {{.NameLower}}.
type Create{{.Name}}Request struct {
	// TODO: add fields, e.g.:
	// Name string ` + "`" + `json:"name" validate:"required,min=2,max=100"` + "`" + `
}

// Update{{.Name}}Request is the request body for updating a {{.NameLower}}.
type Update{{.Name}}Request struct {
	// TODO: add fields
}

// ─── Handlers ─────────────────────────────────────────────────────────────────

// List returns a paginated list of {{.NameLower}}s.
func (h *{{.Name}}Handler) List(c *astra.Context) error {
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
func (h *{{.Name}}Handler) Create(c *astra.Context) error {
	var req Create{{.Name}}Request
	if err := c.ShouldBindJSON(&req); err != nil {
		return err
	}
	ctx := c.Request.Context()
	_ = ctx // TODO: item, err := h.svc.Create(ctx, req)
	return c.JSON(http.StatusCreated, astra.Map{"data": req})
}

// Get returns a {{.NameLower}} by ID.
func (h *{{.Name}}Handler) Get(c *astra.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return astra.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	ctx := c.Request.Context()
	_ = ctx // TODO: item, err := h.svc.Get(ctx, id)
	return c.JSON(http.StatusOK, astra.Map{"id": id})
}

// Update updates a {{.NameLower}} by ID.
func (h *{{.Name}}Handler) Update(c *astra.Context) error {
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
func (h *{{.Name}}Handler) Delete(c *astra.Context) error {
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

const handlerWithServiceSrc = `package {{.Pkg}}

import (
	"context"
	"net/http"
	"strconv"

	"github.com/astra-go/astra"
)

// ─── DTOs ─────────────────────────────────────────────────────────────────────

// {{.Name}}ListQuery holds pagination and filter parameters.
type {{.Name}}ListQuery struct {
	Page    int    ` + "`" + `form:"page"    validate:"min=1"` + "`" + `
	Limit   int    ` + "`" + `form:"limit"   validate:"min=1,max=100"` + "`" + `
	Keyword string ` + "`" + `form:"keyword"` + "`" + `
}

// Create{{.Name}}Request is the request body for creating a {{.NameLower}}.
type Create{{.Name}}Request struct {
	// TODO: add fields, e.g.:
	// Name string ` + "`" + `json:"name" validate:"required,min=2,max=100"` + "`" + `
}

// Update{{.Name}}Request is the request body for updating a {{.NameLower}}.
type Update{{.Name}}Request struct {
	// TODO: add fields
}

// {{.Name}}Response is the response DTO for {{.NameLower}} operations.
type {{.Name}}Response struct {
	ID int64 ` + "`" + `json:"id"` + "`" + `
	// TODO: add fields matching your model
}

// ─── Service interface ────────────────────────────────────────────────────────

// {{.Name}}Service defines the business-logic interface for {{.NameLower}} operations.
// Implement this interface in your service layer (e.g. service/{{.NameLower}}_service.go).
type {{.Name}}Service interface {
	List(ctx context.Context, page, limit int, keyword string) ([]*{{.Name}}Response, int64, error)
	Get(ctx context.Context, id int64) (*{{.Name}}Response, error)
	Create(ctx context.Context, req *Create{{.Name}}Request) (*{{.Name}}Response, error)
	Update(ctx context.Context, id int64, req *Update{{.Name}}Request) (*{{.Name}}Response, error)
	Delete(ctx context.Context, id int64) error
}

// ─── Handler ──────────────────────────────────────────────────────────────────

// {{.Name}}Handler handles {{.NameLower}}-related HTTP requests.
type {{.Name}}Handler struct {
	svc {{.Name}}Service
}

// New{{.Name}}Handler creates a new {{.Name}}Handler.
func New{{.Name}}Handler(svc {{.Name}}Service) *{{.Name}}Handler {
	return &{{.Name}}Handler{svc: svc}
}

// Register mounts all {{.NameLower}} routes onto the given route group.
func (h *{{.Name}}Handler) Register(g *astra.Group) {
	g.GET("/{{.NameLower}}s",        h.List)
	g.POST("/{{.NameLower}}s",       h.Create)
	g.GET("/{{.NameLower}}s/:id",    h.Get)
	g.PUT("/{{.NameLower}}s/:id",    h.Update)
	g.DELETE("/{{.NameLower}}s/:id", h.Delete)
}

// ─── Handlers ─────────────────────────────────────────────────────────────────

// List returns a paginated list of {{.NameLower}}s.
func (h *{{.Name}}Handler) List(c *astra.Context) error {
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
	items, total, err := h.svc.List(c.Request.Context(), q.Page, q.Limit, q.Keyword)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, astra.Map{"data": items, "total": total, "page": q.Page, "limit": q.Limit})
}

// Create creates a new {{.NameLower}}.
func (h *{{.Name}}Handler) Create(c *astra.Context) error {
	var req Create{{.Name}}Request
	if err := c.ShouldBindJSON(&req); err != nil {
		return err
	}
	item, err := h.svc.Create(c.Request.Context(), &req)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, astra.Map{"data": item})
}

// Get returns a {{.NameLower}} by ID.
func (h *{{.Name}}Handler) Get(c *astra.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return astra.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	item, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, astra.Map{"data": item})
}

// Update updates a {{.NameLower}} by ID.
func (h *{{.Name}}Handler) Update(c *astra.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return astra.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	var req Update{{.Name}}Request
	if err := c.ShouldBindJSON(&req); err != nil {
		return err
	}
	item, err := h.svc.Update(c.Request.Context(), id, &req)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, astra.Map{"data": item})
}

// Delete removes a {{.NameLower}} by ID.
func (h *{{.Name}}Handler) Delete(c *astra.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return astra.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}
`

const serviceSrc = `package {{.Pkg}}

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
	ID int64 ` + "`" + `json:"id"` + "`" + `
	// TODO: add fields matching your model
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
	// TODO: inject repository, e.g.:
	// repo *repository.{{.Name}}Repo
}

// New{{.Name}}Service creates a new {{.Name}}ServiceImpl.
func New{{.Name}}Service( /* repo *repository.{{.Name}}Repo */ ) *{{.Name}}ServiceImpl {
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

const modelSrc = `package {{.Pkg}}

import "time"

// {{.Name}} represents a {{.NameLower}} entity.
type {{.Name}} struct {
	ID        int64      ` + "`" + `json:"id"                    gorm:"primaryKey;autoIncrement"` + "`" + `
	CreatedAt time.Time  ` + "`" + `json:"created_at"             gorm:"autoCreateTime"` + "`" + `
	UpdatedAt time.Time  ` + "`" + `json:"updated_at"             gorm:"autoUpdateTime"` + "`" + `
	DeletedAt *time.Time ` + "`" + `json:"deleted_at,omitempty"   gorm:"index"` + "`" + `
	// TODO: add domain fields
}

// TableName sets the GORM table name.
func ({{.Name}}) TableName() string { return "{{.NameLower}}s" }
`

const middlewareSrc = `package {{.Pkg}}

import "github.com/astra-go/astra"

// {{.Name}} is a custom Astra middleware.
func {{.Name}}(/* options */) astra.MiddlewareFunc {
	return func(c *astra.Context) error {
		// ── Before handler ────────────────────────────────────────────────────
		// c.Set("myKey", "myValue")

		c.Next()

		// ── After handler ─────────────────────────────────────────────────────
		// status := c.Writer.Status()

		return nil
	}
}
`

const repoSrc = `package {{.Pkg}}

import (
	"context"

	"github.com/astra-go/astra/orm"
	"gorm.io/gorm"
)

// {{.Name}}Repo handles database operations for {{.Name}} entities.
type {{.Name}}Repo struct {
	*orm.Repository[{{.Name}}Model]
}

// New{{.Name}}Repo creates a new {{.Name}}Repo.
func New{{.Name}}Repo(db *gorm.DB) *{{.Name}}Repo {
	return &{{.Name}}Repo{Repository: orm.NewRepository[{{.Name}}Model](db)}
}

// TODO: replace {{.Name}}Model with your actual model import.
type {{.Name}}Model struct {
	ID int64 ` + "`" + `gorm:"primaryKey"` + "`" + `
}

// FindActive returns all active {{.NameLower}}s.
// orm.FromCtx propagates any transaction from ctx automatically.
func (r *{{.Name}}Repo) FindActive(ctx context.Context) ([]{{.Name}}Model, error) {
	return r.WithCtx(ctx).FindWhere("deleted_at IS NULL")
}
`

const migrationSrc = `package migrations

import (
	"database/sql"

	"github.com/astra-go/astra/migrate"
)

// Migration{{.ID}} — {{.Description}}
var Migration{{.ID}} = &migrate.Migration{
	ID: "{{.IDStr}}",
	Up: func(db *sql.DB) error {
		_, err := db.Exec(` + "`" + `
			-- TODO: add your SQL here
			-- CREATE TABLE IF NOT EXISTS example (
			--     id BIGSERIAL PRIMARY KEY,
			--     name TEXT NOT NULL
			-- )
		` + "`" + `)
		return err
	},
	Down: func(db *sql.DB) error {
		_, err := db.Exec(` + "`" + `
			-- TODO: reverse the migration
			-- DROP TABLE IF EXISTS example
		` + "`" + `)
		return err
	},
}
`

const wireProviderSrc = `//go:build wireinject

package main

import "github.com/google/wire"

// InitializeApp wires up the full application dependency graph.
// Run 'wire gen .' in this directory after adding providers.
//
// Tip: Astra also ships a built-in DI container (no code generation required):
//
//	go get github.com/astra-go/astra
//	astractl gen container --dir ./cmd/server
//
// Example:
//
//	func InitializeApp() (*App, error) {
//	    wire.Build(
//	        NewApp,
//	        handler.NewUserHandler,
//	        service.NewUserService,
//	        repository.NewUserRepo,
//	        provideDB,
//	    )
//	    return nil, nil
//	}
func InitializeApp() (string, error) {
	wire.Build(wire.Value("app"))
	return "", nil
}
`

const diContainerSrc = `package main

import (
	"context"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/di"
)

// initContainer builds the application DI container and binds its lifecycle to app.
// Factories run lazily and at most once (singleton). Hooks registered via OnStop run
// in reverse order during graceful shutdown.
//
// Usage in main():
//
//	app := astra.New()
//	c   := initContainer(app)
//	svc := di.MustInvoke[*UserService](c)
//	handler.NewUserHandler(svc).Register(app.Group("/api/v1"))
//	app.Run(":8080")
func initContainer(app *astra.App) *di.Container {
	c := di.New()

	// ── Infrastructure ────────────────────────────────────────────────────────
	// di.Provide[*sql.DB](c, func(_ *di.Container) (*sql.DB, error) {
	// 	return sql.Open("postgres", os.Getenv("DATABASE_URL"))
	// })

	// ── Repositories ─────────────────────────────────────────────────────────
	// di.Provide[*UserRepo](c, func(c *di.Container) (*UserRepo, error) {
	// 	db, err := di.Invoke[*sql.DB](c)
	// 	return NewUserRepo(db), err
	// })

	// ── Services ─────────────────────────────────────────────────────────────
	// di.Provide[*UserService](c, func(c *di.Container) (*UserService, error) {
	// 	repo, err := di.Invoke[*UserRepo](c)
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	svc := NewUserService(repo)
	// 	c.OnStop(func(ctx context.Context) error { return svc.Close(ctx) })
	// 	return svc, nil
	// })

	_ = context.Background // suppress unused import if no OnStop hooks yet

	c.BindApp(app) // wire Start/Stop into Astra's graceful shutdown
	return c
}
`

const errorCodesSrc = `package {{.Pkg}}

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

const handlerTestSrc = `package {{.Pkg}}_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/astra-go/astra"
)

func setup{{.Name}}App(t *testing.T) *astra.App {
	t.Helper()
	app := astra.New()
	// TODO: wire up handler
	// h := New{{.Name}}Handler(/* deps */)
	// h.Register(app.Group("/api/v1"))
	return app
}

func TestList{{.Name}}s(t *testing.T) {
	app := setup{{.Name}}App(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/{{.NameLower}}s?page=1&limit=10", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 got %d", rec.Code)
	}
}

func TestCreate{{.Name}}(t *testing.T) {
	app := setup{{.Name}}App(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/{{.NameLower}}s", nil)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	_ = rec // TODO: assert expected status and body
}

func TestGet{{.Name}}(t *testing.T) {
	app := setup{{.Name}}App(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/{{.NameLower}}s/1", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	_ = rec // TODO: assert expected status and body
}
`

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

// ─── env / IDE templates ──────────────────────────────────────────────────────

var (
	onceEditorConfig    sync.Once
	onceVSCodeSettings  sync.Once
	onceVSCodeLaunch    sync.Once
	onceVSCodeExtensions sync.Once
	onceGolangCI        sync.Once
	onceDevContainer    sync.Once
	onceGitHookPreCommit sync.Once

	tplEditorConfig     *template.Template
	tplVSCodeSettings   *template.Template
	tplVSCodeLaunch     *template.Template
	tplVSCodeExtensions *template.Template
	tplGolangCI         *template.Template
	tplDevContainer     *template.Template
	tplGitHookPreCommit *template.Template
)

func EditorConfig() *template.Template {
	if t := tryFromDir("editorconfig"); t != nil { return t }
	onceEditorConfig.Do(func() { tplEditorConfig = must("editorconfig", editorConfigSrc) })
	return tplEditorConfig
}

func VSCodeSettings() *template.Template {
	if t := tryFromDir("vscode_settings"); t != nil { return t }
	onceVSCodeSettings.Do(func() { tplVSCodeSettings = must("vscode_settings", vscodeSettingsSrc) })
	return tplVSCodeSettings
}

func VSCodeLaunch() *template.Template {
	if t := tryFromDir("vscode_launch"); t != nil { return t }
	onceVSCodeLaunch.Do(func() { tplVSCodeLaunch = must("vscode_launch", vscodeLaunchSrc) })
	return tplVSCodeLaunch
}

func VSCodeExtensions() *template.Template {
	if t := tryFromDir("vscode_extensions"); t != nil { return t }
	onceVSCodeExtensions.Do(func() { tplVSCodeExtensions = must("vscode_extensions", vscodeExtensionsSrc) })
	return tplVSCodeExtensions
}

func GolangCI() *template.Template {
	if t := tryFromDir("golangci"); t != nil { return t }
	onceGolangCI.Do(func() { tplGolangCI = must("golangci", golangCISrc) })
	return tplGolangCI
}

func DevContainer() *template.Template {
	if t := tryFromDir("devcontainer"); t != nil { return t }
	onceDevContainer.Do(func() { tplDevContainer = must("devcontainer", devContainerSrc) })
	return tplDevContainer
}

func GitHookPreCommit() *template.Template {
	if t := tryFromDir("githooks_pre_commit"); t != nil { return t }
	onceGitHookPreCommit.Do(func() { tplGitHookPreCommit = must("githooks_pre_commit", gitHookPreCommitSrc) })
	return tplGitHookPreCommit
}

const editorConfigSrc = `root = true

[*]
charset = utf-8
end_of_line = lf
insert_final_newline = true
trim_trailing_whitespace = true

[*.go]
indent_style = tab

[*.{yaml,yml,json,toml}]
indent_style = space
indent_size = 2

[*.md]
indent_style = space
indent_size = 2
trim_trailing_whitespace = false

[Makefile]
indent_style = tab
`

const vscodeSettingsSrc = `{
  "go.toolsManagement.autoUpdate": true,
  "editor.formatOnSave": true,
  "[go]": {
    "editor.defaultFormatter": "golang.go",
    "editor.codeActionsOnSave": {
      "source.organizeImports": "explicit"
    }
  },
  "gopls": {
    "ui.semanticTokens": true,
    "formatting.gofumpt": true
  },
  "go.lintTool": "golangci-lint",
  "go.lintFlags": ["--fast"],
  "files.exclude": {
    "**/vendor": true,
    "**/.git": true,
    "**/bin": true
  },
  "files.watcherExclude": {
    "**/vendor/**": true,
    "**/bin/**": true
  }
}
`

const vscodeLaunchSrc = `{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "Launch {{.Name}}",
      "type": "go",
      "request": "launch",
      "mode": "auto",
      "program": "${workspaceFolder}",
      "env": {
        "APP_ENV": "dev"
      },
      "args": []
    },
    {
      "name": "Test current file",
      "type": "go",
      "request": "launch",
      "mode": "test",
      "program": "${file}"
    }
  ]
}
`

const vscodeExtensionsSrc = `{
  "recommendations": [
    "golang.go",
    "ms-azuretools.vscode-docker",
    "redhat.vscode-yaml",
    "foxundermoon.shell-format",
    "timonwong.shellcheck",
    "davidanson.vscode-markdownlint"
  ]
}
`

const golangCISrc = `run:
  timeout: 5m
  go: '{{.GoVersion}}'

linters:
  enable:
    - errcheck
    - govet
    - staticcheck
    - unused
    - gosimple
    - ineffassign
    - typecheck
    - misspell
    - gofmt
    - goimports

linters-settings:
  errcheck:
    check-type-assertions: true
  govet:
    enable-all: true
    disable:
      - fieldalignment

issues:
  exclude-use-default: false
  max-issues-per-linter: 50
  max-same-issues: 3

severity:
  default-severity: warning
`

const devContainerSrc = `{
  "name": "{{.Name}} Dev Container",
  "image": "mcr.microsoft.com/devcontainers/go:{{.GoVersion}}",
  "features": {
    "ghcr.io/devcontainers/features/docker-in-docker:2": {}
  },
  "postCreateCommand": "go mod tidy",
  "customizations": {
    "vscode": {
      "extensions": [
        "golang.go",
        "ms-azuretools.vscode-docker"
      ],
      "settings": {
        "go.toolsManagement.autoUpdate": true,
        "editor.formatOnSave": true,
        "[go]": {
          "editor.defaultFormatter": "golang.go"
        }
      }
    }
  },
  "remoteEnv": {
    "APP_ENV": "dev"
  }
}
`

const gitHookPreCommitSrc = `#!/bin/sh
# Pre-commit hook: run go vet and golangci-lint on staged Go files.
set -e

STAGED=$(git diff --cached --name-only --diff-filter=ACM | grep '\.go$' || true)
[ -z "$STAGED" ] && exit 0

echo "▶  go vet ./..."
go vet ./...

if command -v golangci-lint >/dev/null 2>&1; then
  echo "▶  golangci-lint run (fast)"
  golangci-lint run --fast ./...
fi
`
