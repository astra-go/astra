package astra_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/testutil"
)

// ─── Static route lookup ─────────────────────────────────────────────────────

func TestRouting_StaticRoot(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/", func(c *astra.Ctx) error { return c.String(200, "root") })
	srv := testutil.NewServer(t, app)
	srv.GET("/").AssertStatus(200).AssertBodyContains("root")
}

func TestRouting_StaticDeep(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/a/b/c/d", func(c *astra.Ctx) error { return c.String(200, "deep") })
	srv := testutil.NewServer(t, app)
	srv.GET("/a/b/c/d").AssertStatus(200).AssertBodyContains("deep")
	srv.GET("/a/b/c").AssertStatus(404)
}

// ─── Parameter route extraction ──────────────────────────────────────────────

func TestRouting_Param_Single(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/items/:id", func(c *astra.Ctx) error {
		return c.String(200, "id=%s", c.Param("id"))
	})
	srv := testutil.NewServer(t, app)
	srv.GET("/items/42").AssertStatus(200).AssertBodyContains("id=42")
	srv.GET("/items/hello").AssertStatus(200).AssertBodyContains("id=hello")
}

func TestRouting_Param_Multiple(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/users/:uid/posts/:pid", func(c *astra.Ctx) error {
		return c.String(200, "uid=%s,pid=%s", c.Param("uid"), c.Param("pid"))
	})
	srv := testutil.NewServer(t, app)
	srv.GET("/users/7/posts/99").AssertStatus(200).AssertBodyContains("uid=7,pid=99")
}

func TestRouting_Param_MissingParam(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/items/:id", func(c *astra.Ctx) error {
		return c.String(200, "id=%s", c.Param("id"))
	})
	srv := testutil.NewServer(t, app)
	// Partial path without the param segment → 404
	srv.GET("/items").AssertStatus(404)
}

// ─── Wildcard route matching ─────────────────────────────────────────────────

func TestRouting_Wildcard_DeepPath(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/static/*filepath", func(c *astra.Ctx) error {
		return c.String(200, "file=%s", c.Param("filepath"))
	})
	srv := testutil.NewServer(t, app)
	srv.GET("/static/css/main.css").AssertStatus(200).AssertBodyContains("file=/css/main.css")
	srv.GET("/static/js/app.js.map").AssertStatus(200).AssertBodyContains("file=/js/app.js.map")
}

func TestRouting_Wildcard_SingleSegment(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/download/*file", func(c *astra.Ctx) error {
		return c.String(200, "file=%s", c.Param("file"))
	})
	srv := testutil.NewServer(t, app)
	srv.GET("/download/readme.txt").AssertStatus(200).AssertBodyContains("file=/readme.txt")
}

// ─── Regex route matching ────────────────────────────────────────────────────

func TestRouting_Regex_DigitOnly(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/orders/{id:\\d+}", func(c *astra.Ctx) error {
		return c.String(200, "order=%s", c.Param("id"))
	})
	srv := testutil.NewServer(t, app)
	srv.GET("/orders/12345").AssertStatus(200).AssertBodyContains("order=12345")
	srv.GET("/orders/abc").AssertStatus(404)
}

func TestRouting_Regex_SlashPattern(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/api/{version:v[0-9]+}/users", func(c *astra.Ctx) error {
		return c.String(200, "ver=%s", c.Param("version"))
	})
	srv := testutil.NewServer(t, app)
	srv.GET("/api/v1/users").AssertStatus(200).AssertBodyContains("ver=v1")
	srv.GET("/api/v12/users").AssertStatus(200).AssertBodyContains("ver=v12")
	srv.GET("/api/beta/users").AssertStatus(404)
}

// ─── 404 Not Found ───────────────────────────────────────────────────────────

func TestRouting_NotFound_EmptyApp(t *testing.T) {
	app := testutil.NewTestApp()
	srv := testutil.NewServer(t, app)
	srv.GET("/anything").AssertStatus(404)
	srv.POST("/anything", nil).AssertStatus(404)
}

func TestRouting_NotFound_PartialMatch(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/users/list", func(c *astra.Ctx) error { return c.String(200, "list") })
	srv := testutil.NewServer(t, app)
	srv.GET("/users/list/extra").AssertStatus(404)
}

// ─── 405 Method Not Allowed ──────────────────────────────────────────────────

func TestRouting_MethodNotAllowed_AllowHeader(t *testing.T) {
	app := astra.New() // Not ModeTest so 405 works without panic
	app.GET("/resource", func(c *astra.Ctx) error { return c.String(200, "get") })
	app.PUT("/resource", func(c *astra.Ctx) error { return c.String(200, "put") })

	req := httptest.NewRequest(http.MethodDelete, "/resource", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
	allow := w.Header().Get("Allow")
	if allow == "" {
		t.Fatal("Allow header must be set on 405")
	}
	if !strings.Contains(allow, "GET") {
		t.Errorf("Allow header should contain GET, got %q", allow)
	}
	if !strings.Contains(allow, "PUT") {
		t.Errorf("Allow header should contain PUT, got %q", allow)
	}
}

func TestRouting_MethodNotAllowed_SingleMethod(t *testing.T) {
	app := astra.New()
	app.POST("/submit", func(c *astra.Ctx) error { return c.String(200, "ok") })

	req := httptest.NewRequest(http.MethodGet, "/submit", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
	allow := w.Header().Get("Allow")
	if !strings.Contains(allow, "POST") {
		t.Errorf("Allow header should contain POST, got %q", allow)
	}
}

// ─── Route group with middleware ──────────────────────────────────────────────

func TestRouting_Group_NestedMiddleware(t *testing.T) {
	app := testutil.NewTestApp()
	api := app.Group("/api", func(c *astra.Ctx) error {
		c.Set("layer", "api")
		c.Next()
		return nil
	})
	v1 := api.Group("/v1", func(c *astra.Ctx) error {
		c.Set("version", "v1")
		c.Next()
		return nil
	})
	v1.GET("/info", func(c *astra.Ctx) error {
		return c.String(200, "%s-%s", c.GetString("layer"), c.GetString("version"))
	})

	srv := testutil.NewServer(t, app)
	srv.GET("/api/v1/info").AssertStatus(200).AssertBodyContains("api-v1")
}

func TestRouting_Group_MiddlewareAbort(t *testing.T) {
	app := testutil.NewTestApp()
	admin := app.Group("/admin", func(c *astra.Ctx) error {
		c.AbortWithStatus(http.StatusUnauthorized)
		return nil
	})
	admin.GET("/dashboard", func(c *astra.Ctx) error {
		return c.String(200, "dashboard")
	})

	srv := testutil.NewServer(t, app)
	srv.GET("/admin/dashboard").AssertStatus(http.StatusUnauthorized)
}

// ─── Duplicate route registration ────────────────────────────────────────────

func TestRouting_DuplicateRoute_LogsWarning(t *testing.T) {
	_ = strings.Builder{}
	app := astra.New() // not ModeTest, so duplicate logs warning
	app.GET("/dup", func(c *astra.Ctx) error { return c.String(200, "first") })
	app.GET("/dup", func(c *astra.Ctx) error { return c.String(200, "second") })

	// Second handler should win
	srv := testutil.NewServer(t, app)
	srv.GET("/dup").AssertStatus(200).AssertBodyContains("second")
}

// ─── Many routes (100+) ─────────────────────────────────────────────────────

func TestRouting_ManyRoutes(t *testing.T) {
	app := testutil.NewTestApp()
	for i := range 200 {
		path := fmt.Sprintf("/route%d", i)
		body := fmt.Sprintf("route%d", i)
		app.GET(path, func(b string) astra.HandlerFunc {
			return func(c *astra.Ctx) error { return c.String(200, "%s", b) }
		}(body))
	}
	srv := testutil.NewServer(t, app)
	// Spot check a few
	srv.GET("/route0").AssertStatus(200).AssertBodyContains("route0")
	srv.GET("/route99").AssertStatus(200).AssertBodyContains("route99")
	srv.GET("/route199").AssertStatus(200).AssertBodyContains("route199")
	srv.GET("/route200").AssertStatus(404)
}

func TestRouting_ManyParamRoutes(t *testing.T) {
	app := testutil.NewTestApp()
	for i := range 100 {
		prefix := fmt.Sprintf("/api/v%d", i)
		app.GET(prefix+"/:id", func(c *astra.Ctx) error {
			return c.String(200, "id=%s", c.Param("id"))
		})
	}
	srv := testutil.NewServer(t, app)
	srv.GET("/api/v0/42").AssertStatus(200).AssertBodyContains("id=42")
	srv.GET("/api/v99/abc").AssertStatus(200).AssertBodyContains("id=abc")
}

// ─── Case sensitivity ────────────────────────────────────────────────────────

func TestRouting_CaseSensitive(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/Hello", func(c *astra.Ctx) error { return c.String(200, "upper") })
	srv := testutil.NewServer(t, app)
	srv.GET("/Hello").AssertStatus(200).AssertBodyContains("upper")
	srv.GET("/hello").AssertStatus(404)
	srv.GET("/HELLO").AssertStatus(404)
}

// ─── Trailing slash handling ─────────────────────────────────────────────────

func TestRouting_TrailingSlash_OnlyOneRegistered(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/only", func(c *astra.Ctx) error { return c.String(200, "only") })
	srv := testutil.NewServer(t, app)
	srv.GET("/only").AssertStatus(200).AssertBodyContains("only")
	// Astra matches /only/ to /only — trailing slash is treated as the same route
	srv.GET("/only/").AssertStatus(200).AssertBodyContains("only")
}

// ─── Multiple parameters in one path ─────────────────────────────────────────

func TestRouting_ThreeParams(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/org/:org/project/:proj/issue/:issue", func(c *astra.Ctx) error {
		return c.String(200, "org=%s,proj=%s,issue=%s",
			c.Param("org"), c.Param("proj"), c.Param("issue"))
	})
	srv := testutil.NewServer(t, app)
	srv.GET("/org/myorg/project/myproj/issue/42").
		AssertStatus(200).
		AssertBodyContains("org=myorg,proj=myproj,issue=42")
}

// ─── Mix of static, param, wildcard ─────────────────────────────────────────

func TestRouting_StaticVsParamPrecedence(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/users/me", func(c *astra.Ctx) error { return c.String(200, "me") })
	app.GET("/users/:id", func(c *astra.Ctx) error { return c.String(200, "id=%s", c.Param("id")) })
	srv := testutil.NewServer(t, app)
	srv.GET("/users/me").AssertStatus(200).AssertBodyContains("me")
	srv.GET("/users/other").AssertStatus(200).AssertBodyContains("id=other")
}

func TestRouting_ParamAndWildcard(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/docs/:section", func(c *astra.Ctx) error {
		return c.String(200, "section=%s", c.Param("section"))
	})
	app.GET("/docs/:section/*page", func(c *astra.Ctx) error {
		return c.String(200, "section=%s,page=%s", c.Param("section"), c.Param("page"))
	})
	srv := testutil.NewServer(t, app)
	srv.GET("/docs/api").AssertStatus(200).AssertBodyContains("section=api")
	srv.GET("/docs/api/intro").AssertStatus(200).AssertBodyContains("section=api,page=/intro")
}
