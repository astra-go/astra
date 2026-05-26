// Package swagger mounts a Swagger UI and an OpenAPI JSON endpoint on an
// Astra application.
//
// # Typical workflow
//
//  1. Annotate your handlers with swaggo comments:
//
//	// @Summary Create user
//	// @Tags    users
//	// @Accept  json
//	// @Produce json
//	// @Param   body body CreateUserReq true "request"
//	// @Success 200 {object} User
//	// @Router  /users [post]
//	func createUser(c *astra.Ctx) error { ... }
//
//  2. Generate the spec with the swag CLI (install once: go install github.com/swaggo/swag/cmd/swag@latest):
//
//	swag init -g main.go -o docs
//
//     This produces docs/docs.go, docs/swagger.json, docs/swagger.yaml.
//
//  3. Import the generated package and register Swagger in main.go:
//
//	import (
//	    _ "myapp/docs"              // side-effect: registers SwaggerInfo
//	    "github.com/astra-go/astra/swagger"
//	)
//
//	// option A — Register function
//	swagger.Register(app, swagger.Config{})
//
//	// option B — Plugin (integrates with App.RegisterPlugin)
//	app.RegisterPlugin(swagger.New(swagger.Config{}))
//
// The UI is then available at http://localhost:8080/swagger/index.html.
// The raw spec is served at             http://localhost:8080/swagger/doc.json.
//
// # Without swaggo
//
// You can provide your own OpenAPI JSON bytes directly via Config.SpecJSON,
// bypassing the swaggo registry entirely:
//
//	spec, _ := os.ReadFile("openapi.json")
//	swagger.Register(app, swagger.Config{SpecJSON: spec})
//
// # UI source
//
// Swagger UI assets are loaded from the official unpkg CDN by default.
// Set Config.CDN to a custom base URL (or a self-hosted path) if you need
// to run air-gapped. The served HTML is a single self-contained template; no
// static files are embedded in the binary.
package swagger

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/astra-go/astra"
)

// ─── swaggo compatibility shim ────────────────────────────────────────────────

// swagInfo is the subset of swaggo's swag.Info that we need.
// We use it via interface rather than importing swaggo so that swaggo remains
// an optional dev-time dependency (go install) and not a runtime dep.
type swagInfo interface {
	ReadDoc() string
}

// global registry — populated by swaggo's init() via RegisterSwaggerInfo.
var registeredSpec []byte

// RegisterSwaggerInfo is called by generated docs/docs.go init() functions.
// Users who import _ "myapp/docs" do NOT need to call this directly.
//
// If you use a custom generator, call this once at startup instead.
func RegisterSwaggerInfo(info swagInfo) {
	registeredSpec = []byte(info.ReadDoc())
}

// ─── Config ───────────────────────────────────────────────────────────────────

// Config configures the Swagger endpoint.
type Config struct {
	// BasePath is the URL prefix under which both the UI and spec are served.
	// Default: "/swagger".
	BasePath string

	// Title is displayed in the browser tab.
	// Default: "Swagger UI".
	Title string

	// SpecJSON is the raw OpenAPI JSON bytes to serve.
	// When nil, the spec registered via RegisterSwaggerInfo (i.e. from the
	// generated docs package) is used.
	SpecJSON []byte

	// CDN is the base URL of the Swagger UI distribution.
	// Default: "https://unpkg.com/swagger-ui-dist@5".
	// Override for air-gapped environments or self-hosting.
	CDN string

	// DeepLinking enables deep-linking for tags and operations.
	// Default: true.
	DeepLinking *bool

	// PersistAuthorization persists authorization data across page reloads.
	// Default: true.
	PersistAuthorization *bool

	// DocExpansion controls how operations are initially rendered.
	// "list" (default) | "full" | "none".
	DocExpansion string
}

func (c *Config) setDefaults() {
	if c.BasePath == "" {
		c.BasePath = "/swagger"
	}
	c.BasePath = strings.TrimRight(c.BasePath, "/")
	if c.Title == "" {
		c.Title = "Swagger UI"
	}
	if c.CDN == "" {
		c.CDN = "https://unpkg.com/swagger-ui-dist@5"
	}
	if c.DeepLinking == nil {
		t := true
		c.DeepLinking = &t
	}
	if c.PersistAuthorization == nil {
		t := true
		c.PersistAuthorization = &t
	}
	if c.DocExpansion == "" {
		c.DocExpansion = "list"
	}
}

// spec returns the JSON bytes to serve, falling back to the globally
// registered spec and then to an empty OpenAPI 3 skeleton.
func (c *Config) spec() []byte {
	if len(c.SpecJSON) > 0 {
		return c.SpecJSON
	}
	if len(registeredSpec) > 0 {
		return registeredSpec
	}
	// Return a minimal valid OpenAPI 3 stub so the UI always renders.
	return []byte(`{"openapi":"3.0.0","info":{"title":"` + c.Title + `","version":"0.0.0"},"paths":{}}`)
}

// ─── Plugin ───────────────────────────────────────────────────────────────────

// swaggerPlugin implements astra.Plugin.
type swaggerPlugin struct{ cfg Config }

// New returns a swagger Plugin for use with App.RegisterPlugin.
func New(cfg Config) astra.Plugin {
	return &swaggerPlugin{cfg: cfg}
}

// Name implements astra.Plugin.
func (p *swaggerPlugin) Name() string { return "swagger" }

// Init mounts the swagger routes on the Astra app.
func (p *swaggerPlugin) Init(app *astra.App) error {
	Register(app, p.cfg)
	return nil
}

// ─── Register ─────────────────────────────────────────────────────────────────

// Register mounts the Swagger UI and spec endpoint on app.
//
// Routes added:
//
//	GET {basePath}/doc.json       — raw OpenAPI JSON
//	GET {basePath}/               — redirect → index.html
//	GET {basePath}/index.html     — Swagger UI
func Register(app *astra.App, cfg Config) {
	cfg.setDefaults()

	specJSON := cfg.spec()

	// ── spec endpoint ────────────────────────────────────────────────────────
	app.GET(cfg.BasePath+"/doc.json", func(c *astra.Ctx) error {
		c.Writer().Header().Set("Content-Type", "application/json; charset=utf-8")
		c.Writer().WriteHeader(http.StatusOK)
		_, err := c.Writer().Write(specJSON)
		return err
	})

	// ── UI index redirect ─────────────────────────────────────────────────────
	app.GET(cfg.BasePath+"/", func(c *astra.Ctx) error {
		http.Redirect(c.Writer(), c.Request(), cfg.BasePath+"/index.html", http.StatusMovedPermanently)
		return nil
	})

	// ── Swagger UI HTML ───────────────────────────────────────────────────────
	uiHTML := buildUIHTML(cfg)
	app.GET(cfg.BasePath+"/index.html", func(c *astra.Ctx) error {
		c.Writer().Header().Set("Content-Type", "text/html; charset=utf-8")
		c.Writer().WriteHeader(http.StatusOK)
		_, err := c.Writer().Write(uiHTML)
		return err
	})
}

// ─── UI HTML builder ──────────────────────────────────────────────────────────

var uiTmpl = template.Must(template.New("swagger-ui").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>{{.Title}}</title>
  <link rel="stylesheet" href="{{.CDN}}/swagger-ui.css">
  <style>
    html { box-sizing: border-box; overflow: -moz-scrollbars-vertical; overflow-y: scroll; }
    *, *:before, *:after { box-sizing: inherit; }
    body { margin: 0; background: #fafafa; }
    .topbar { display: none; }
  </style>
</head>
<body>
<div id="swagger-ui"></div>
<script src="{{.CDN}}/swagger-ui-bundle.js"></script>
<script src="{{.CDN}}/swagger-ui-standalone-preset.js"></script>
<script>
window.onload = function() {
  const ui = SwaggerUIBundle({
    url:                  "{{.SpecURL}}",
    dom_id:               '#swagger-ui',
    presets:              [SwaggerUIBundle.presets.apis, SwaggerUIStandalonePreset],
    layout:               "StandaloneLayout",
    deepLinking:          {{.DeepLinking}},
    persistAuthorization: {{.PersistAuthorization}},
    docExpansion:         "{{.DocExpansion}}",
    defaultModelsExpandDepth: 1,
    displayRequestDuration:   true,
    filter:               true,
    tryItOutEnabled:      true,
  });
  window.ui = ui;
};
</script>
</body>
</html>`))

type uiData struct {
	Title                string
	CDN                  string
	SpecURL              string
	DeepLinking          bool
	PersistAuthorization bool
	DocExpansion         string
}

func buildUIHTML(cfg Config) []byte {
	data := uiData{
		Title:                cfg.Title,
		CDN:                  cfg.CDN,
		SpecURL:              cfg.BasePath + "/doc.json",
		DeepLinking:          *cfg.DeepLinking,
		PersistAuthorization: *cfg.PersistAuthorization,
		DocExpansion:         cfg.DocExpansion,
	}
	var sb strings.Builder
	if err := uiTmpl.Execute(&sb, data); err != nil {
		// Template is static; this path is only reachable via programmer error.
		panic(fmt.Sprintf("swagger: render UI template: %v", err))
	}
	return []byte(sb.String())
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// MustJSON marshals v to JSON and panics on error.
// Convenience for building inline spec stubs in tests.
func MustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("swagger.MustJSON: %v", err))
	}
	return b
}
