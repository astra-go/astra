package astra_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/astra-go/astra"
)

// FuzzRouterRegistration tests that route registration never panics
// regardless of the path pattern supplied.  It exercises the radix-tree
// insertNode + splitPath paths with arbitrary strings.
func FuzzRouterRegistration(f *testing.F) {
	seeds := []string{
		"/",
		"/users",
		"/users/:id",
		"/files/*filepath",
		"/items/{id:[0-9]+}",
		"/api/v1/resources/{name:[a-z]+}",
		"//double",
		"/trailing/",
		"/deep/a/b/c/d/e/f/g",
		"",
		"/with space",
		"/with%20encode",
		"/日本語/パス",
		"/emoji/🚀",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, path string) {
		app := astra.New()
		// Registration must not panic
		app.GET(path, func(c *astra.Ctx) error { return c.String(200, "ok") })
		app.POST(path, func(c *astra.Ctx) error { return c.String(201, "created") })

		// Dispatch must not panic
		req := httptest.NewRequest(http.MethodGet, "/noop", nil)
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)
	})
}

// FuzzRouterDispatch tests route lookup (matchSegments) with arbitrary
// request paths against a fixed set of registered routes.
func FuzzRouterDispatch(f *testing.F) {
	seeds := []string{
		"/",
		"/users",
		"/users/42",
		"/files/docs/readme.md",
		"/items/abc",
		"//",
		"/../escape",
		"/normal/path",
		"/with%20space",
		"",
		"/a/b/c",
		"/日本語",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, reqPath string) {
		// httptest.NewRequest panics on an empty target;
		// the router itself handles empty paths gracefully.
		if reqPath == "" {
			reqPath = "/"
		}

		app := astra.New()
		app.GET("/", func(c *astra.Ctx) error { return c.String(200, "root") })
		app.GET("/users/:id", func(c *astra.Ctx) error { return c.String(200, "user") })
		app.GET("/files/*filepath", func(c *astra.Ctx) error { return c.String(200, "file") })
		app.GET("/items/{id:[0-9]+}", func(c *astra.Ctx) error { return c.String(200, "item") })
		app.GET("/api/{ver:v[0-9]+}/{name:[a-z]+}", func(c *astra.Ctx) error { return c.String(200, "api") })

		req := httptest.NewRequest(http.MethodGet, reqPath, nil)
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)
		// Only invariant: must not panic, must return a valid HTTP status
		if rec.Code < 100 || rec.Code > 599 {
			t.Errorf("invalid status code: %d", rec.Code)
		}
	})
}
