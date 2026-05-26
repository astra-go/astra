// Package render provides server-side HTML template rendering for Astra.
//
// # Engine interface
//
// Any struct that implements Engine can be set as the App renderer via
// astra.WithRenderer. The built-in HTMLEngine is backed by Go's standard
// html/template package.
//
//	app := astra.New(
//	    astra.WithRenderer(render.New(render.Config{
//	        Root:    "templates",
//	        Layout:  "layouts/base.html",
//	        Reload:  true, // hot-reload in dev mode
//	    })),
//	)
//
// # Directory layout
//
//	templates/
//	├── layouts/
//	│   └── base.html          ← defines {{block "title" .}} {{block "content" .}}
//	├── partials/
//	│   ├── header.html        ← {{define "partials/header.html"}} … {{end}}
//	│   └── footer.html
//	└── pages/
//	    ├── index.html         ← {{define "title"}}Home{{end}} {{define "content"}}…{{end}}
//	    └── user/
//	        └── profile.html
//
// # Rendering with a layout
//
// Call c.Render with the page template path. The engine clones the pre-loaded
// layout+partials set, parses the page template into the clone, then executes
// the configured layout template.
//
//	func indexHandler(c *astra.Ctx) error {
//	    return c.Render(200, "pages/index.html", astra.Map{"Title": "Home"})
//	}
//
// # Rendering without a layout
//
// Set Config.Layout to "" and the page template is executed directly.
//
// # embed.FS
//
//	//go:embed templates
//	var tmplFS embed.FS
//
//	render.New(render.Config{
//	    FS:     tmplFS,
//	    Root:   "templates",
//	    Layout: "layouts/base.html",
//	})
//
// # Custom template functions
//
//	render.New(render.Config{
//	    FuncMap: template.FuncMap{
//	        "upper": strings.ToUpper,
//	        "fmtDate": func(t time.Time) string {
//	            return t.Format("2006-01-02")
//	        },
//	    },
//	})
package render

import (
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// ─── Engine interface ─────────────────────────────────────────────────────────

// Engine is the interface that template backends must implement.
// It is identical to astra.Renderer so that any Engine value satisfies both
// without a circular import.
type Engine interface {
	// Render writes the rendered output of the named template to w.
	// data is passed as the template's dot value (.).
	Render(w io.Writer, name string, data any) error
}

// ─── Config ───────────────────────────────────────────────────────────────────

// Config configures the HTMLEngine.
type Config struct {
	// Root is the root directory that contains all template files.
	// Relative to the working directory (or to FS if set).
	// Default: "templates".
	Root string

	// Extension is the file extension to recognise as a template.
	// Default: ".html".
	Extension string

	// Layout is the name of the default layout template (relative to Root),
	// e.g. "layouts/base.html".  When set, Render clones the layout+partials
	// set, injects the page template, and executes the layout.
	// Empty string = no layout (page template is executed directly).
	Layout string

	// Partials are glob patterns (relative to Root) whose matching files are
	// pre-loaded into every clone together with Layout.
	// Example: []string{"partials/*.html", "components/*.html"}
	Partials []string

	// FuncMap is merged into every template set.
	FuncMap template.FuncMap

	// Reload re-parses all templates on every Render call.
	// Useful during development; disable in production.
	Reload bool

	// FS is the filesystem to load templates from.
	// When nil, os.DirFS(Root) is used.
	FS fs.FS
}

func (c *Config) setDefaults() {
	if c.Root == "" {
		c.Root = "templates"
	}
	if c.Extension == "" {
		c.Extension = ".html"
	}
}

// ─── HTMLEngine ───────────────────────────────────────────────────────────────

// HTMLEngine renders templates using Go's html/template package.
// It is safe for concurrent use.
type HTMLEngine struct {
	cfg  Config
	fsys fs.FS

	mu   sync.RWMutex
	base *template.Template // pre-loaded layout + partials
}

// New creates and loads an HTMLEngine from the given Config.
// Returns an error if the initial template load fails.
func New(cfg Config) (*HTMLEngine, error) {
	cfg.setDefaults()

	var fsys fs.FS
	if cfg.FS != nil {
		// If a custom FS is provided and Root is set, sub into it.
		if cfg.Root != "" && cfg.Root != "." {
			sub, err := fs.Sub(cfg.FS, cfg.Root)
			if err != nil {
				return nil, fmt.Errorf("render: sub FS at %q: %w", cfg.Root, err)
			}
			fsys = sub
		} else {
			fsys = cfg.FS
		}
	} else {
		fsys = os.DirFS(cfg.Root)
	}

	e := &HTMLEngine{cfg: cfg, fsys: fsys}
	if err := e.load(); err != nil {
		return nil, err
	}
	return e, nil
}

// Must is like New but panics on error. Useful for package-level init.
func Must(cfg Config) *HTMLEngine {
	e, err := New(cfg)
	if err != nil {
		panic(fmt.Sprintf("render.Must: %v", err))
	}
	return e
}

// load (re)parses the layout and all partial templates into e.base.
func (e *HTMLEngine) load() error {
	root := template.New("").Funcs(builtinFuncs()).Funcs(e.cfg.FuncMap)

	loader := func(path string) error {
		b, err := fs.ReadFile(e.fsys, path)
		if err != nil {
			return fmt.Errorf("render: read %q: %w", path, err)
		}
		name := filepath.ToSlash(path) // normalise separators
		if _, err = root.New(name).Parse(string(b)); err != nil {
			return fmt.Errorf("render: parse %q: %w", path, err)
		}
		return nil
	}

	// Load layout.
	if e.cfg.Layout != "" {
		if err := loader(e.cfg.Layout); err != nil {
			return err
		}
	}

	// Load partials.
	for _, pattern := range e.cfg.Partials {
		matches, err := fs.Glob(e.fsys, pattern)
		if err != nil {
			return fmt.Errorf("render: glob %q: %w", pattern, err)
		}
		for _, m := range matches {
			if err := loader(m); err != nil {
				return err
			}
		}
	}

	// If no explicit partials configured, auto-load everything that isn't the
	// layout itself and lives under any "partials" or "components" sub-directory.
	if len(e.cfg.Partials) == 0 {
		_ = fs.WalkDir(e.fsys, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, e.cfg.Extension) {
				return nil
			}
			dir := filepath.ToSlash(filepath.Dir(path))
			if strings.Contains(dir, "partial") || strings.Contains(dir, "component") {
				_ = loader(path)
			}
			return nil
		})
	}

	e.mu.Lock()
	e.base = root
	e.mu.Unlock()
	return nil
}

// Render writes the rendered template to w.
//
// When Config.Layout is set:
//  1. The pre-loaded base set (layout + partials) is cloned.
//  2. The named page template file is parsed into the clone.
//  3. The layout template is executed (it uses {{block "content" .}} which the
//     page template overrides with {{define "content"}}…{{end}}).
//
// When Config.Layout is empty the named template is executed directly.
func (e *HTMLEngine) Render(w io.Writer, name string, data any) error {
	if e.cfg.Reload {
		if err := e.load(); err != nil {
			return err
		}
	}

	e.mu.RLock()
	base := e.base
	e.mu.RUnlock()

	if e.cfg.Layout == "" {
		// No layout: execute the named template directly.
		// First try as a pre-loaded named template.
		if t := base.Lookup(name); t != nil {
			return t.Execute(w, data)
		}
		// Fall back to reading from fs and parsing on the fly.
		return e.renderDirect(w, base, name, data)
	}

	// With layout: clone base, parse page, execute layout.
	t, err := base.Clone()
	if err != nil {
		return fmt.Errorf("render: clone base: %w", err)
	}

	// Parse the page template into the clone.
	b, err := fs.ReadFile(e.fsys, name)
	if err != nil {
		return fmt.Errorf("render: read page %q: %w", name, err)
	}
	pageName := filepath.ToSlash(name)
	if _, err = t.New(pageName).Parse(string(b)); err != nil {
		return fmt.Errorf("render: parse page %q: %w", name, err)
	}

	// Execute the layout template.
	return t.ExecuteTemplate(w, e.cfg.Layout, data)
}

// renderDirect reads, parses, and executes a template that was not pre-loaded.
func (e *HTMLEngine) renderDirect(w io.Writer, base *template.Template, name string, data any) error {
	b, err := fs.ReadFile(e.fsys, name)
	if err != nil {
		return fmt.Errorf("render: read %q: %w", name, err)
	}
	t, err := base.Clone()
	if err != nil {
		return fmt.Errorf("render: clone: %w", err)
	}
	pageName := filepath.ToSlash(name)
	t2, err := t.New(pageName).Parse(string(b))
	if err != nil {
		return fmt.Errorf("render: parse %q: %w", name, err)
	}
	return t2.Execute(w, data)
}

// AddFunc adds a template function after the engine has been created.
// It triggers a full reload. Not safe to call concurrently with Render.
func (e *HTMLEngine) AddFunc(name string, fn any) error {
	if e.cfg.FuncMap == nil {
		e.cfg.FuncMap = template.FuncMap{}
	}
	e.cfg.FuncMap[name] = fn
	return e.load()
}

// Reload forces a re-parse of all templates. Use in development when
// Reload: false is set but you want a manual refresh.
func (e *HTMLEngine) Reload() error { return e.load() }

// ─── built-in template functions ──────────────────────────────────────────────

// builtinFuncs returns a set of convenience functions available in all templates.
func builtinFuncs() template.FuncMap {
	return template.FuncMap{
		// safeHTML marks a string as trusted HTML (bypasses escaping).
		"safeHTML": func(s string) template.HTML { return template.HTML(s) },
		// safeURL marks a string as a trusted URL.
		"safeURL": func(s string) template.URL { return template.URL(s) },
		// safeAttr marks a string as a trusted HTML attribute value.
		"safeAttr": func(s string) template.HTMLAttr { return template.HTMLAttr(s) },
		// safeCSS marks a string as trusted CSS.
		"safeCSS": func(s string) template.CSS { return template.CSS(s) },
		// safeJS marks a string as trusted JavaScript.
		"safeJS": func(s string) template.JS { return template.JS(s) },
		// dict builds a map[string]any from alternating key-value pairs.
		// Useful for passing multiple values into a sub-template:
		//   {{template "card.html" (dict "Title" .Title "Body" .Body)}}
		"dict": func(pairs ...any) (map[string]any, error) {
			if len(pairs)%2 != 0 {
				return nil, fmt.Errorf("render: dict requires an even number of arguments")
			}
			m := make(map[string]any, len(pairs)/2)
			for i := 0; i < len(pairs); i += 2 {
				k, ok := pairs[i].(string)
				if !ok {
					return nil, fmt.Errorf("render: dict key must be a string, got %T", pairs[i])
				}
				m[k] = pairs[i+1]
			}
			return m, nil
		},
		// iterate returns a slice of ints [0, n) for range loops.
		"iterate": func(n int) []int {
			s := make([]int, n)
			for i := range s {
				s[i] = i
			}
			return s
		},
	}
}
