package render_test

import (
	"bytes"
	"html/template"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/astra-go/astra/render"
	"github.com/astra-go/astra/testutil"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

// baseFS returns a minimal in-memory filesystem for most tests.
func baseFS() fstest.MapFS {
	return fstest.MapFS{
		"pages/hello.html": {Data: []byte(`Hello, {{.Name}}!`)},
		"layouts/base.html": {Data: []byte(
			`BASE:{{block "content" .}}default{{end}}`,
		)},
		"pages/with_layout.html": {Data: []byte(
			`{{define "content"}}CONTENT:{{.Value}}{{end}}`,
		)},
		"partials/nav.html": {Data: []byte(
			`{{define "partials/nav.html"}}NAV{{end}}`,
		)},
	}
}

// engine creates an HTMLEngine from a MapFS with root "." (no sub-dir).
func engine(t *testing.T, fsys fstest.MapFS, cfg render.Config) *render.HTMLEngine {
	t.Helper()
	cfg.FS = fsys
	if cfg.Root == "" {
		cfg.Root = "."
	}
	eng, err := render.New(cfg)
	if err != nil {
		t.Fatalf("render.New: %v", err)
	}
	return eng
}

// ─── New / Must ───────────────────────────────────────────────────────────────

func TestNew_ValidConfig_NoError(t *testing.T) {
	_, err := render.New(render.Config{FS: baseFS(), Root: "."})
	testutil.AssertNoError(t, err)
}

func TestNew_MissingLayout_ReturnsError(t *testing.T) {
	_, err := render.New(render.Config{
		FS:     fstest.MapFS{},
		Root:   ".",
		Layout: "nonexistent.html",
	})
	testutil.AssertError(t, err)
}

func TestMust_PanicsOnError(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("Must should panic when layout file does not exist")
		}
	}()
	render.Must(render.Config{
		FS:     fstest.MapFS{},
		Root:   ".",
		Layout: "missing.html",
	})
}

// ─── Render — no layout ───────────────────────────────────────────────────────

func TestRender_DirectTemplate(t *testing.T) {
	eng := engine(t, baseFS(), render.Config{})

	var buf bytes.Buffer
	err := eng.Render(&buf, "pages/hello.html", map[string]any{"Name": "Astra"})
	testutil.AssertNoError(t, err)

	if !strings.Contains(buf.String(), "Hello, Astra!") {
		t.Errorf("expected 'Hello, Astra!', got %q", buf.String())
	}
}

func TestRender_MissingTemplate_ReturnsError(t *testing.T) {
	eng := engine(t, baseFS(), render.Config{})

	err := eng.Render(&bytes.Buffer{}, "pages/missing.html", nil)
	testutil.AssertError(t, err)
}

func TestRender_TemplateError_Propagated(t *testing.T) {
	fsys := fstest.MapFS{
		// Calling a nil function causes a template execution error.
		"bad.html": {Data: []byte(`{{call .Fn}}`)},
	}
	eng := engine(t, fsys, render.Config{})

	err := eng.Render(&bytes.Buffer{}, "bad.html", map[string]any{
		"Fn": (func())(nil), // nil func → reflect.Call panics → template returns error
	})
	testutil.AssertError(t, err)
}

// ─── Render — with layout ─────────────────────────────────────────────────────

func TestRender_WithLayout_WrapsContent(t *testing.T) {
	eng := engine(t, baseFS(), render.Config{Layout: "layouts/base.html"})

	var buf bytes.Buffer
	err := eng.Render(&buf, "pages/with_layout.html", map[string]any{"Value": "world"})
	testutil.AssertNoError(t, err)

	got := buf.String()
	if !strings.Contains(got, "BASE:") {
		t.Errorf("layout prefix 'BASE:' missing, got %q", got)
	}
	if !strings.Contains(got, "CONTENT:world") {
		t.Errorf("content 'CONTENT:world' missing, got %q", got)
	}
}

func TestRender_WithLayout_MissingPage_ReturnsError(t *testing.T) {
	eng := engine(t, baseFS(), render.Config{Layout: "layouts/base.html"})

	err := eng.Render(&bytes.Buffer{}, "pages/nonexistent.html", nil)
	testutil.AssertError(t, err)
}

// ─── Partials ─────────────────────────────────────────────────────────────────

func TestRender_Partials_AutoLoadedFromPartialDir(t *testing.T) {
	fsys := fstest.MapFS{
		"partials/nav.html": {Data: []byte(`{{define "partials/nav.html"}}NAV{{end}}`)},
		"pages/page.html":   {Data: []byte(`{{template "partials/nav.html" .}}`)},
	}
	eng := engine(t, fsys, render.Config{})

	var buf bytes.Buffer
	err := eng.Render(&buf, "pages/page.html", nil)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "NAV", buf.String())
}

func TestRender_Partials_ExplicitGlob(t *testing.T) {
	fsys := fstest.MapFS{
		"shared/widget.html": {Data: []byte(`{{define "shared/widget.html"}}WIDGET{{end}}`)},
		"pages/page.html":    {Data: []byte(`{{template "shared/widget.html" .}}`)},
	}
	eng := engine(t, fsys, render.Config{
		Partials: []string{"shared/*.html"},
	})

	var buf bytes.Buffer
	err := eng.Render(&buf, "pages/page.html", nil)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "WIDGET", buf.String())
}

// ─── FuncMap ──────────────────────────────────────────────────────────────────

func TestRender_CustomFuncMap(t *testing.T) {
	fsys := fstest.MapFS{
		"tmpl.html": {Data: []byte(`{{upper .Name}}`)},
	}
	eng := engine(t, fsys, render.Config{
		FuncMap: template.FuncMap{
			"upper": strings.ToUpper,
		},
	})

	var buf bytes.Buffer
	err := eng.Render(&buf, "tmpl.html", map[string]any{"Name": "astra"})
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "ASTRA", buf.String())
}

func TestRender_Builtin_SafeHTML(t *testing.T) {
	fsys := fstest.MapFS{
		"tmpl.html": {Data: []byte(`{{safeHTML .Raw}}`)},
	}
	eng := engine(t, fsys, render.Config{})

	var buf bytes.Buffer
	err := eng.Render(&buf, "tmpl.html", map[string]any{"Raw": "<b>bold</b>"})
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "<b>bold</b>", buf.String())
}

func TestRender_Builtin_Dict(t *testing.T) {
	fsys := fstest.MapFS{
		"tmpl.html": {Data: []byte(`{{with (dict "k" "v")}}{{.k}}{{end}}`)},
	}
	eng := engine(t, fsys, render.Config{})

	var buf bytes.Buffer
	err := eng.Render(&buf, "tmpl.html", nil)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "v", buf.String())
}

func TestRender_Builtin_Iterate(t *testing.T) {
	fsys := fstest.MapFS{
		"tmpl.html": {Data: []byte(`{{range iterate 3}}{{.}}{{end}}`)},
	}
	eng := engine(t, fsys, render.Config{})

	var buf bytes.Buffer
	err := eng.Render(&buf, "tmpl.html", nil)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "012", buf.String())
}

// ─── AddFunc ──────────────────────────────────────────────────────────────────

func TestAddFunc_AvailableAfterAddition(t *testing.T) {
	fsys := fstest.MapFS{
		"tmpl.html": {Data: []byte(`{{greet .Name}}`)},
	}
	eng := engine(t, fsys, render.Config{})

	err := eng.AddFunc("greet", func(s string) string { return "Hi " + s })
	testutil.AssertNoError(t, err)

	var buf bytes.Buffer
	err = eng.Render(&buf, "tmpl.html", map[string]any{"Name": "Alice"})
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "Hi Alice", buf.String())
}

// ─── Reload ───────────────────────────────────────────────────────────────────

func TestRender_Reload_PicksUpUpdatedTemplate(t *testing.T) {
	fsys := fstest.MapFS{
		"tmpl.html": {Data: []byte(`v1`)},
	}
	eng := engine(t, fsys, render.Config{Reload: true})

	// Update the in-memory FS before second render.
	fsys["tmpl.html"] = &fstest.MapFile{Data: []byte(`v2`)}

	var buf bytes.Buffer
	err := eng.Render(&buf, "tmpl.html", nil)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "v2", buf.String())
}

func TestReload_ManualReload(t *testing.T) {
	fsys := fstest.MapFS{
		"tmpl.html": {Data: []byte(`v1`)},
	}
	// Reload: false — manual reload only.
	eng := engine(t, fsys, render.Config{Reload: false})

	fsys["tmpl.html"] = &fstest.MapFile{Data: []byte(`v2`)}

	testutil.AssertNoError(t, eng.Reload())

	var buf bytes.Buffer
	err := eng.Render(&buf, "tmpl.html", nil)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "v2", buf.String())
}

// ─── Concurrent safety ────────────────────────────────────────────────────────

func TestRender_ConcurrentAccess(t *testing.T) {
	fsys := fstest.MapFS{
		"tmpl.html": {Data: []byte(`Hello`)},
	}
	eng := engine(t, fsys, render.Config{})

	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var buf bytes.Buffer
			if err := eng.Render(&buf, "tmpl.html", nil); err != nil {
				t.Errorf("concurrent Render: %v", err)
			}
		}()
	}
	wg.Wait()
}
