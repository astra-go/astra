# Render — Server-Side Template Rendering

Server-side template rendering engine based on Go standard library `html/template`, supports layouts, partials, hot reload, and `embed.FS`.

## Features

- **Layout System**: Main template + sub-template layout nesting
- **Partial Templates**: Template fragments in `partials/` directory can be referenced by any page
- **Hot Reload**: In development mode, file changes auto-reparse templates
- **embed.FS Support**: Pairs with Go 1.16+ `embed` for compile-time static asset packaging
- **Custom Functions**: `FuncMap` supports registering custom template functions

## Quick Start

```go
import (
    "github.com/astra-go/astra"
    "github.com/astra-go/astra/render"
)

// Method 1: Filesystem path
app := astra.New(astra.WithRenderer(render.New(render.Config{
    Root:    "templates",
    Layout:  "layouts/base.html",
    Ext:     ".html",
    Reload:  true, // Hot reload in development
})))

app.GET("/page", func(c *astra.Ctx) error {
    return c.Render(200, "pages/index.html", astra.Map{"title": "Home"})
})

// Method 2: embed.FS (recommended for production)
//go:embed templates
var tmplFS embed.FS

app := astra.New(astra.WithRenderer(render.New(render.Config{
    FS:     tmplFS,
    Root:   "templates",
    Layout: "layouts/base.html",
})))
```

## Directory Structure Example

```
templates/
├── layouts/
│   └── base.html          ← {{block "content" .}} {{end}}
├── partials/
│   ├── header.html        ← {{define "partials/header"}} ... {{end}}
│   └── footer.html
└── pages/
    ├── index.html         ← {{define "title"}}Home{{end}} {{define "content"}} ... {{end}}
    └── user/
        └── profile.html
```

### Using Partial Templates in Templates

```html
<!-- pages/index.html -->
{{template "partials/header.html" .}}
<div>{{.title}}</div>
{{template "partials/footer.html" .}}
```

## API

### New

```go
func New(cfg Config) Engine

type Config struct {
    Root    string           // Template root dir (mutually exclusive with FS)
    FS      embed.FS         // embed.FS (mutually exclusive with Root)
    Layout  string           // Main layout template path (empty=no layout)
    Ext     string           // File extension, default ".html"
    Reload  bool             // Reload on file change
    FuncMap template.FuncMap // Custom template functions
}
```

### c.Render

```go
func (c *astra.Ctx) Render(code int, name string, data any) error
```

`name` is template path relative to Root; `data` is template data.

## Custom Template Functions

```go
app := astra.New(astra.WithRenderer(render.New(render.Config{
    Root:   "templates",
    Layout: "layouts/base.html",
    FuncMap: template.FuncMap{
        "upper": strings.ToUpper,
        "formatDate": func(t time.Time) string {
            return t.Format("2006-01-02")
        },
        "truncate": func(s string, n int) string {
            if len(s) > n {
                return s[:n] + "..."
            }
            return s
        },
    },
})))
```

## Complete Example

```go
package main

import (
    "embed"
    "github.com/astra-go/astra"
    "github.com/astra-go/astra/render"
)

//go:embed templates
var tmplFS embed.FS

//go:embed templates/layouts
var _ embed.FS

func main() {
    app := astra.New(astra.WithRenderer(render.New(render.Config{
        FS:     tmplFS,
        Root:   "templates",
        Layout: "layouts/base.html",
        FuncMap: map[string]any{
            "upper": func(s string) string { return strings.ToUpper(s) },
        },
    })))

    app.GET("/", func(c *astra.Ctx) error {
        return c.Render(200, "pages/index.html", astra.Map{
            "title": "Welcome",
            "items": []string{"A", "B", "C"},
        })
    })

    app.Run(":8080")
}
```

## Module Dependencies

No external dependencies (uses Go standard library `html/template`).

## Notes

- `Root` and `FS` are mutually exclusive; `FS` is suitable for production to avoid runtime filesystem dependency
- `Reload: true` only for development; should be off in production to avoid performance overhead
- Template execution errors return HTTP 500; recommend validating template syntax during development
