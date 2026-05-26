package astra_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/testutil"
)

// ─── Routing ──────────────────────────────────────────────────────────────────

func TestRouting_BasicMethods(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/get", func(c *astra.Ctx) error { return c.String(200, "GET") })
	app.POST("/post", func(c *astra.Ctx) error { return c.String(201, "POST") })
	app.PUT("/put", func(c *astra.Ctx) error { return c.String(200, "PUT") })
	app.DELETE("/del", func(c *astra.Ctx) error { return c.String(200, "DELETE") })
	app.PATCH("/patch", func(c *astra.Ctx) error { return c.String(200, "PATCH") })

	srv := testutil.NewServer(t, app)
	srv.GET("/get").AssertStatus(200).AssertBodyContains("GET")
	srv.POST("/post", nil).AssertStatus(201).AssertBodyContains("POST")
	srv.PUT("/put", nil).AssertStatus(200).AssertBodyContains("PUT")
	srv.DELETE("/del").AssertStatus(200).AssertBodyContains("DELETE")
	srv.PATCH("/patch", nil).AssertStatus(200).AssertBodyContains("PATCH")
}

func TestRouting_PathParam(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/users/:id", func(c *astra.Ctx) error {
		return c.JSON(200, astra.Map{"id": c.Param("id")})
	})

	srv := testutil.NewServer(t, app)
	var body struct {
		ID string `json:"id"`
	}
	srv.GET("/users/42").AssertStatus(200).AssertJSON(&body)
	testutil.AssertEqual(t, "42", body.ID)
}

func TestRouting_Wildcard(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/files/*filepath", func(c *astra.Ctx) error {
		return c.String(200, "%s", c.Param("filepath"))
	})

	srv := testutil.NewServer(t, app)
	srv.GET("/files/a/b/c").AssertStatus(200).AssertBodyContains("a/b/c")
}

func TestRouting_Group(t *testing.T) {
	app := testutil.NewTestApp()
	v1 := app.Group("/api/v1")
	v1.GET("/ping", func(c *astra.Ctx) error { return c.String(200, "pong") })

	srv := testutil.NewServer(t, app)
	srv.GET("/api/v1/ping").AssertStatus(200).AssertBodyContains("pong")
}

func TestRouting_NotFound(t *testing.T) {
	app := testutil.NewTestApp()
	srv := testutil.NewServer(t, app)
	srv.GET("/nonexistent").AssertStatus(404)
}

// ─── Context ──────────────────────────────────────────────────────────────────

func TestContext_JSON(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/json", func(c *astra.Ctx) error {
		return c.JSON(200, astra.Map{"key": "value", "num": 42})
	})

	srv := testutil.NewServer(t, app)
	resp := srv.GET("/json")
	resp.AssertStatus(200).
		AssertHeader("Content-Type", "application/json; charset=utf-8").
		AssertBodyContains(`"key"`)

	var m map[string]any
	resp.AssertJSON(&m)
	testutil.AssertEqual(t, "value", m["key"].(string))
}

func TestContext_ContentLength(t *testing.T) {
	// JSON responses must set Content-Length (not use chunked encoding)
	app := testutil.NewTestApp()
	app.GET("/cl", func(c *astra.Ctx) error {
		return c.JSON(200, astra.Map{"x": 1})
	})
	srv := testutil.NewServer(t, app)
	resp := srv.GET("/cl")
	resp.AssertStatus(200)
	if resp.Header("Content-Length") == "" {
		t.Error("Content-Length header must be set on JSON responses")
	}
}

func TestContext_String(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/str", func(c *astra.Ctx) error {
		return c.String(200, "hello %s", "world")
	})
	srv := testutil.NewServer(t, app)
	srv.GET("/str").AssertStatus(200).AssertBodyContains("hello world").
		AssertHeaderContains("Content-Type", "text/plain")
}

func TestContext_NoContent(t *testing.T) {
	app := testutil.NewTestApp()
	app.DELETE("/resource", func(c *astra.Ctx) error {
		return c.NoContent(http.StatusNoContent)
	})
	srv := testutil.NewServer(t, app)
	srv.DELETE("/resource").AssertStatus(http.StatusNoContent)
}

func TestContext_QueryParam(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/search", func(c *astra.Ctx) error {
		q := c.DefaultQuery("q", "default")
		return c.String(200, "%s", q)
	})
	srv := testutil.NewServer(t, app)
	srv.GET("/search?q=astra").AssertStatus(200).AssertBodyContains("astra")
	srv.GET("/search").AssertStatus(200).AssertBodyContains("default")
}

func TestContext_SetGet(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(func(c *astra.Ctx) error {
		c.Set("user", "alice")
		c.Next()
		return nil
	})
	app.GET("/me", func(c *astra.Ctx) error {
		user := c.GetString("user")
		return c.String(200, "%s", user)
	})
	srv := testutil.NewServer(t, app)
	srv.GET("/me").AssertStatus(200).AssertBodyContains("alice")
}

func TestContext_BindJSON(t *testing.T) {
	type req struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}
	app := testutil.NewTestApp()
	app.POST("/echo", func(c *astra.Ctx) error {
		var r req
		if err := c.ShouldBindJSON(&r); err != nil {
			return err
		}
		return c.JSON(200, r)
	})
	srv := testutil.NewServer(t, app)
	var resp req
	srv.POST("/echo", req{Name: "test", Value: 99}).
		AssertStatus(200).
		AssertJSON(&resp)
	testutil.AssertEqual(t, "test", resp.Name)
	testutil.AssertEqual(t, 99, resp.Value)
}

func TestContext_BindJSON_Invalid(t *testing.T) {
	app := testutil.NewTestApp()
	app.POST("/parse", func(c *astra.Ctx) error {
		var v map[string]any
		return c.ShouldBindJSON(&v)
	})
	srv := testutil.NewServer(t, app)
	// Send non-JSON body
	srv.Do("POST", "/parse", "not json").AssertStatus(400)
}

func TestContext_BindJSON_MaxBodySize(t *testing.T) {
	app := testutil.NewTestApp(astra.WithMaxJSONBodySize(16))
	app.POST("/echo", func(c *astra.Ctx) error {
		var v map[string]any
		if err := c.BindJSON(&v); err != nil {
			return err
		}
		return c.JSON(200, v)
	})
	srv := testutil.NewServer(t, app)
	// {"k":"v"} = 9 bytes — fits within 16-byte limit.
	srv.Do("POST", "/echo", map[string]string{"k": "v"}).AssertStatus(200)
	// {"key":"value_exceeds"} = 23 bytes — truncated by limit → JSON parse error → 400.
	srv.Do("POST", "/echo", map[string]string{"key": "value_exceeds"}).AssertStatus(400)
}

func TestContext_Redirect(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/old", func(c *astra.Ctx) error {
		return c.Redirect(http.StatusMovedPermanently, "/new")
	})
	app.GET("/new", func(c *astra.Ctx) error {
		return c.String(200, "new")
	})
	srv := testutil.NewServer(t, app)
	// Default client follows redirects
	srv.GET("/new").AssertStatus(200)
}

// ─── Middleware ───────────────────────────────────────────────────────────────

func TestMiddleware_Order(t *testing.T) {
	app := testutil.NewTestApp()
	var log []string
	app.Use(func(c *astra.Ctx) error {
		log = append(log, "m1-before")
		c.Next()
		log = append(log, "m1-after")
		return nil
	})
	app.Use(func(c *astra.Ctx) error {
		log = append(log, "m2-before")
		c.Next()
		log = append(log, "m2-after")
		return nil
	})
	app.GET("/order", func(c *astra.Ctx) error {
		log = append(log, "handler")
		return c.NoContent(200)
	})
	srv := testutil.NewServer(t, app)
	srv.GET("/order").AssertStatus(200)

	want := []string{"m1-before", "m2-before", "handler", "m2-after", "m1-after"}
	if len(log) != len(want) {
		t.Fatalf("execution order: want %v, got %v", want, log)
	}
	for i := range want {
		if log[i] != want[i] {
			t.Errorf("step %d: want %q, got %q", i, want[i], log[i])
		}
	}
}

func TestMiddleware_Abort(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(func(c *astra.Ctx) error {
		c.AbortWithStatus(http.StatusForbidden)
		return nil
	})
	app.GET("/secret", func(c *astra.Ctx) error {
		return c.String(200, "should not reach")
	})
	srv := testutil.NewServer(t, app)
	srv.GET("/secret").
		AssertStatus(http.StatusForbidden).
		AssertBodyNotContains("should not reach")
}

func TestMiddleware_GroupScoped(t *testing.T) {
	app := testutil.NewTestApp()
	authed := app.Group("/admin", func(c *astra.Ctx) error {
		token := c.Header("X-Token")
		if token != "secret" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return nil
		}
		c.Next()
		return nil
	})
	authed.GET("/data", func(c *astra.Ctx) error {
		return c.String(200, "admin data")
	})
	app.GET("/public", func(c *astra.Ctx) error {
		return c.String(200, "public")
	})

	srv := testutil.NewServer(t, app)
	srv.GET("/admin/data").AssertStatus(http.StatusUnauthorized)
	srv.GET("/admin/data", map[string]string{"X-Token": "secret"}).
		AssertStatus(200).AssertBodyContains("admin data")
	srv.GET("/public").AssertStatus(200)
}

// ─── Error handling ───────────────────────────────────────────────────────────

func TestErrorHandler_HTTPError(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/err", func(c *astra.Ctx) error {
		return astra.NewHTTPError(http.StatusTeapot, "I'm a teapot")
	})
	srv := testutil.NewServer(t, app)
	srv.GET("/err").AssertStatus(http.StatusTeapot).AssertBodyContains("teapot")
}

func TestErrorHandler_AppError(t *testing.T) {
	app := testutil.NewTestApp()
	userNotFound := &astra.AppError{
		Code:       "USER_NOT_FOUND",
		HTTPStatus: http.StatusNotFound,
		Message:    "user not found",
	}
	app.GET("/user", func(c *astra.Ctx) error {
		return userNotFound
	})
	srv := testutil.NewServer(t, app)
	var body struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	srv.GET("/user").
		AssertStatus(http.StatusNotFound).
		AssertJSON(&body)
	testutil.AssertEqual(t, "USER_NOT_FOUND", body.Code)
	testutil.AssertEqual(t, "user not found", body.Message)
}

func TestErrorHandler_ProdMode_Masks5xx(t *testing.T) {
	app := astra.New(astra.WithMode(astra.ModeProd))
	app.GET("/boom", func(c *astra.Ctx) error {
		return errors.New("internal secret details")
	})
	srv := testutil.NewServer(t, app)
	srv.GET("/boom").
		AssertStatus(http.StatusInternalServerError).
		AssertBodyNotContains("internal secret details")
}

// ─── Plugin ───────────────────────────────────────────────────────────────────

type echoPlugin struct{ called bool }

func (p *echoPlugin) Name() string { return "echo" }
func (p *echoPlugin) Init(app *astra.App) error {
	p.called = true
	app.GET("/plugin/echo", func(c *astra.Ctx) error {
		return c.String(200, "from plugin")
	})
	return nil
}

func TestPlugin_RegisterAndInit(t *testing.T) {
	app := testutil.NewTestApp()
	p := &echoPlugin{}
	if err := app.RegisterPlugin(p); err != nil {
		t.Fatalf("RegisterPlugin: %v", err)
	}
	if !p.called {
		t.Fatal("plugin Init was not called")
	}
	srv := testutil.NewServer(t, app)
	srv.GET("/plugin/echo").AssertStatus(200).AssertBodyContains("from plugin")
}

type failPlugin struct{}

func (failPlugin) Name() string        { return "fail" }
func (failPlugin) Init(*astra.App) error { return errors.New("init failed") }

func TestPlugin_InitError(t *testing.T) {
	app := testutil.NewTestApp()
	err := app.RegisterPlugin(failPlugin{})
	testutil.AssertError(t, err)
	if !strings.Contains(err.Error(), "fail") {
		t.Errorf("error should mention plugin name: %v", err)
	}
}

// ─── Serializer ───────────────────────────────────────────────────────────────

type customSerializer struct{ called bool }

func (s *customSerializer) Marshal(v any) ([]byte, error)            { s.called = true; return json.Marshal(v) }
func (s *customSerializer) Unmarshal(data []byte, v any) error       { return json.Unmarshal(data, v) }

func TestSerializer_Custom(t *testing.T) {
	ser := &customSerializer{}
	app := testutil.NewTestApp(astra.WithSerializer(ser))
	app.GET("/s", func(c *astra.Ctx) error {
		return c.JSON(200, astra.Map{"ok": true})
	})
	srv := testutil.NewServer(t, app)
	srv.GET("/s").AssertStatus(200)
	if !ser.called {
		t.Error("custom serializer Marshal was not called")
	}
}

// ─── ClientIP ────────────────────────────────────────────────────────────────

// ─── Regex routing ───────────────────────────────────────────────────────────

func TestRouting_Regex_MatchesDigitID(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/users/{id:[0-9]+}", func(c *astra.Ctx) error {
		return c.JSON(200, astra.Map{"id": c.Param("id")})
	})

	srv := testutil.NewServer(t, app)
	var body struct {
		ID string `json:"id"`
	}
	// numeric id → matches regex
	srv.GET("/users/123").AssertStatus(200).AssertJSON(&body)
	testutil.AssertEqual(t, "123", body.ID)
	// non-numeric id → 404
	srv.GET("/users/abc").AssertStatus(404)
}

func TestRouting_Regex_PriorityOverParam(t *testing.T) {
	app := testutil.NewTestApp()
	// regex route registered first; bare :id also registered
	app.GET("/items/{id:[0-9]+}", func(c *astra.Ctx) error {
		return c.String(200, "numeric:%s", c.Param("id"))
	})
	app.GET("/items/:id", func(c *astra.Ctx) error {
		return c.String(200, "param:%s", c.Param("id"))
	})

	srv := testutil.NewServer(t, app)
	// numeric → must hit regex route
	srv.GET("/items/42").AssertStatus(200).AssertBodyContains("numeric:42")
	// alpha → falls back to param route
	srv.GET("/items/foo").AssertStatus(200).AssertBodyContains("param:foo")
}

func TestRouting_Regex_MultiplePatterns(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/v/{ver:[0-9]+}", func(c *astra.Ctx) error {
		return c.String(200, "num:%s", c.Param("ver"))
	})
	app.GET("/v/{ver:[a-z]+}", func(c *astra.Ctx) error {
		return c.String(200, "alpha:%s", c.Param("ver"))
	})

	srv := testutil.NewServer(t, app)
	srv.GET("/v/3").AssertStatus(200).AssertBodyContains("num:3")
	srv.GET("/v/beta").AssertStatus(200).AssertBodyContains("alpha:beta")
	srv.GET("/v/UPPER").AssertStatus(404)
}

func TestRouting_Regex_NestedAfterRegexSegment(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/api/{version:v[0-9]+}/users", func(c *astra.Ctx) error {
		return c.String(200, "ver:%s", c.Param("version"))
	})

	srv := testutil.NewServer(t, app)
	srv.GET("/api/v2/users").AssertStatus(200).AssertBodyContains("ver:v2")
	srv.GET("/api/beta/users").AssertStatus(404)
}

func TestRouting_Regex_InvalidPatternPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for invalid regex pattern")
		}
	}()
	app := testutil.NewTestApp()
	app.GET("/bad/{id:[invalid}", func(c *astra.Ctx) error { return nil })
}

func TestClientIP_RemoteAddr(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/ip", func(c *astra.Ctx) error {
		return c.String(200, "%s", c.ClientIP())
	})
	srv := testutil.NewServer(t, app)
	resp := srv.GET("/ip")
	resp.AssertStatus(200)
	// The test server connects from 127.0.0.1
	resp.AssertBodyContains("127.0.0.1")
}

// clientIPApp is a small helper: it builds an app with the given trusted proxies
// and a single GET /ip route that responds with c.ClientIP().
func clientIPApp(proxies ...string) *astra.App {
	app := testutil.NewTestApp(astra.WithTrustedProxies(proxies))
	app.GET("/ip", func(c *astra.Ctx) error {
		return c.String(200, "%s", c.ClientIP())
	})
	return app
}

// TestClientIP_NoTrustedProxy_XFFIgnored verifies that when no proxies are
// configured the XFF header is completely ignored and RemoteAddr is returned.
func TestClientIP_NoTrustedProxy_XFFIgnored(t *testing.T) {
	srv := testutil.NewServer(t, clientIPApp( /* no trusted proxies */ ))
	srv.GET("/ip", map[string]string{"X-Forwarded-For": "9.9.9.9"}).
		AssertStatus(200).
		AssertBodyContains("127.0.0.1")
}

// TestClientIP_XFF_ForgeryPrevented is the core regression test.
// An attacker prepends a fake IP as the leftmost XFF entry.
// The old left-to-right code would return 1.1.1.1 (forged).
// The new right-to-left code must return 2.2.2.2 (real caller).
func TestClientIP_XFF_ForgeryPrevented(t *testing.T) {
	// 127.0.0.1 is the direct peer (test server), so it is trusted.
	srv := testutil.NewServer(t, clientIPApp("127.0.0.1"))
	// Attacker-forged leftmost entry, real client IP on the right.
	srv.GET("/ip", map[string]string{"X-Forwarded-For": "1.1.1.1, 2.2.2.2"}).
		AssertStatus(200).
		AssertBodyContains("2.2.2.2")
}

// TestClientIP_XFF_MultiHopChain verifies right-to-left traversal skips
// intermediate trusted proxies and surfaces the real client.
func TestClientIP_XFF_MultiHopChain(t *testing.T) {
	// 127.0.0.1 is the test-server peer; 10.0.0.1 is an internal proxy hop.
	srv := testutil.NewServer(t, clientIPApp("127.0.0.1", "10.0.0.1"))
	// chain: real_client → 10.0.0.1 (internal proxy) → 127.0.0.1 (edge proxy)
	srv.GET("/ip", map[string]string{"X-Forwarded-For": "203.0.113.5, 10.0.0.1"}).
		AssertStatus(200).
		AssertBodyContains("203.0.113.5")
}

// TestClientIP_XFF_AllTrustedFallback verifies that when every XFF entry is a
// known proxy the function falls through to RemoteAddr.
func TestClientIP_XFF_AllTrustedFallback(t *testing.T) {
	srv := testutil.NewServer(t, clientIPApp("127.0.0.1", "10.0.0.1", "10.0.0.2"))
	srv.GET("/ip", map[string]string{"X-Forwarded-For": "10.0.0.1, 10.0.0.2"}).
		AssertStatus(200).
		AssertBodyContains("127.0.0.1") // fallback to RemoteAddr
}

// TestClientIP_XFF_CIDR verifies that CIDR ranges in TrustedProxies work
// correctly during right-to-left traversal.
func TestClientIP_XFF_CIDR(t *testing.T) {
	srv := testutil.NewServer(t, clientIPApp("127.0.0.0/8", "10.0.0.0/8"))
	// 10.0.0.1 is inside 10.0.0.0/8 → trusted; 203.0.113.5 is not → returned.
	srv.GET("/ip", map[string]string{"X-Forwarded-For": "203.0.113.5, 10.0.0.1"}).
		AssertStatus(200).
		AssertBodyContains("203.0.113.5")
}

// TestClientIP_XFF_MalformedEntrySkipped ensures that a malformed entry in the
// middle of XFF does not halt traversal; the first valid non-trusted IP wins.
func TestClientIP_XFF_MalformedEntrySkipped(t *testing.T) {
	srv := testutil.NewServer(t, clientIPApp("127.0.0.1"))
	srv.GET("/ip", map[string]string{"X-Forwarded-For": "203.0.113.7, not-an-ip"}).
		AssertStatus(200).
		AssertBodyContains("203.0.113.7")
}

// TestClientIP_XRealIp_Trusted verifies that X-Real-Ip is honoured when the
// direct peer is trusted and no XFF header is present.
func TestClientIP_XRealIp_Trusted(t *testing.T) {
	srv := testutil.NewServer(t, clientIPApp("127.0.0.1"))
	srv.GET("/ip", map[string]string{"X-Real-Ip": "203.0.113.9"}).
		AssertStatus(200).
		AssertBodyContains("203.0.113.9")
}

// TestClientIP_XRealIp_UntrustedPeerIgnored verifies that X-Real-Ip is not
// honoured when the direct peer is not in TrustedProxies.
func TestClientIP_XRealIp_UntrustedPeerIgnored(t *testing.T) {
	srv := testutil.NewServer(t, clientIPApp( /* no trusted proxies */ ))
	srv.GET("/ip", map[string]string{"X-Real-Ip": "203.0.113.9"}).
		AssertStatus(200).
		AssertBodyContains("127.0.0.1") // RemoteAddr wins
}

// ─── Route conflict detection ─────────────────────────────────────────────────

// TestRouting_Conflict_LogsWarning verifies that registering the same
// method+path twice emits a slog.Warn message and keeps the new handler.
func TestRouting_Conflict_LogsWarning(t *testing.T) {
	// Capture slog output into a buffer.
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(slog.Default()) }) // restore after test

	app := astra.New()
	app.GET("/users/:id", func(c *astra.Ctx) error { return c.String(200, "handler1") })
	app.GET("/users/:id", func(c *astra.Ctx) error { return c.String(200, "handler2") }) // conflict

	out := buf.String()
	if !strings.Contains(out, "route conflict") {
		t.Errorf("expected route conflict warning in log, got: %q", out)
	}
	if !strings.Contains(out, "/users/:id") {
		t.Errorf("expected path in warning, got: %q", out)
	}

	// New handler must win.
	srv := testutil.NewServer(t, app)
	srv.GET("/users/42").AssertStatus(200).AssertBodyContains("handler2")
}

// TestRouting_Conflict_RootPath verifies that "/" can be overridden and emits a warning.
func TestRouting_Conflict_RootPath(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(slog.Default()) })

	app := astra.New()
	app.GET("/", func(c *astra.Ctx) error { return c.String(200, "first") })
	app.GET("/", func(c *astra.Ctx) error { return c.String(200, "second") })

	if !strings.Contains(buf.String(), "route conflict") {
		t.Errorf("expected route conflict warning for '/', got: %q", buf.String())
	}

	srv := testutil.NewServer(t, app)
	srv.GET("/").AssertStatus(200).AssertBodyContains("second")
}

// TestRouting_Conflict_StaticPath verifies conflict detection for plain static paths.
func TestRouting_Conflict_StaticPath(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(slog.Default()) })

	app := astra.New()
	app.GET("/health", func(c *astra.Ctx) error { return c.String(200, "v1") })
	app.GET("/health", func(c *astra.Ctx) error { return c.String(200, "v2") })

	if !strings.Contains(buf.String(), "route conflict") {
		t.Errorf("expected route conflict warning for static path, got: %q", buf.String())
	}

	srv := testutil.NewServer(t, app)
	srv.GET("/health").AssertStatus(200).AssertBodyContains("v2")
}

// TestRouting_NoConflict_DifferentMethods verifies that registering the same
// path under different HTTP methods does NOT produce a warning.
func TestRouting_NoConflict_DifferentMethods(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(slog.Default()) })

	app := astra.New()
	app.GET("/items", func(c *astra.Ctx) error { return c.String(200, "get") })
	app.POST("/items", func(c *astra.Ctx) error { return c.String(201, "post") })

	if strings.Contains(buf.String(), "route conflict") {
		t.Errorf("unexpected route conflict warning for different methods: %q", buf.String())
	}
}

// ─── Slim mode tests ──────────────────────────────────────────────────────────

func TestNewSlim_RoutingWorks(t *testing.T) {
	app := astra.NewSlim()
	app.GET("/ping", func(c *astra.Ctx) error { return c.String(200, "pong") })

	srv := testutil.NewServer(t, app)
	srv.GET("/ping").AssertStatus(200).AssertBodyContains("pong")
}

func TestNewSlim_OnStart_ReturnsErrSlimMode(t *testing.T) {
	app := astra.NewSlim()
	err := app.OnStart(func(ctx context.Context) error { return nil })
	if !errors.Is(err, astra.ErrSlimMode) {
		t.Fatalf("expected ErrSlimMode, got %v", err)
	}
}

func TestNewSlim_OnStop_ReturnsErrSlimMode(t *testing.T) {
	app := astra.NewSlim()
	err := app.OnStop(func(ctx context.Context) error { return nil })
	if !errors.Is(err, astra.ErrSlimMode) {
		t.Fatalf("expected ErrSlimMode, got %v", err)
	}
}

func TestNewSlim_RegisterPlugin_ReturnsErrSlimMode(t *testing.T) {
	app := astra.NewSlim()
	err := app.RegisterPlugin()
	if !errors.Is(err, astra.ErrSlimMode) {
		t.Fatalf("expected ErrSlimMode, got %v", err)
	}
}

func TestNewSlim_Register_ReturnsErrSlimMode(t *testing.T) {
	app := astra.NewSlim()
	err := app.Register(astra.NewModuleFunc("noop", func(*astra.App) error { return nil }))
	if !errors.Is(err, astra.ErrSlimMode) {
		t.Fatalf("expected ErrSlimMode, got %v", err)
	}
}

func TestNew_OnStart_OnStop_StillWork(t *testing.T) {
	app := astra.New()
	startCalled := false
	if err := app.OnStart(func(ctx context.Context) error {
		startCalled = true
		return nil
	}); err != nil {
		t.Fatalf("New().OnStart returned error: %v", err)
	}
	stopCalled := false
	if err := app.OnStop(func(ctx context.Context) error {
		stopCalled = true
		return nil
	}); err != nil {
		t.Fatalf("New().OnStop returned error: %v", err)
	}
	_ = startCalled
	_ = stopCalled
}

// ─── 405 Allow-header tests ─────────────────────────────────────────────��─────

func TestMethodNotAllowed_AllowHeader(t *testing.T) {
	app := astra.New()
	app.GET("/res/:id", func(c *astra.Ctx) error { return nil })
	app.POST("/res/:id", func(c *astra.Ctx) error { return nil })
	app.DELETE("/res/:id", func(c *astra.Ctx) error { return nil })

	req := httptest.NewRequest(http.MethodPatch, "/res/1", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
	allow := w.Header().Get("Allow")
	if allow == "" {
		t.Fatal("Allow header must be present on 405 (RFC 9110 §15.5.6)")
	}
	for _, m := range []string{"GET", "POST", "DELETE"} {
		if !containsMethod(allow, m) {
			t.Errorf("Allow %q missing %s", allow, m)
		}
	}
	if containsMethod(allow, http.MethodPatch) {
		t.Errorf("Allow %q must not include the request method PATCH", allow)
	}
}

func TestMethodNotAllowed_AllowHeaderOrder(t *testing.T) {
	app := astra.New()
	// Register in reverse order; Allow header should still follow methodOrder.
	app.DELETE("/r/:id", func(c *astra.Ctx) error { return nil })
	app.POST("/r/:id", func(c *astra.Ctx) error { return nil })
	app.GET("/r/:id", func(c *astra.Ctx) error { return nil })

	req := httptest.NewRequest(http.MethodPatch, "/r/1", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	allow := w.Header().Get("Allow")
	if allow != "GET, POST, DELETE" {
		t.Errorf("expected Allow: GET, POST, DELETE; got %q", allow)
	}
}

func TestNotFound_NoAllowHeader(t *testing.T) {
	app := astra.New()
	app.GET("/exists", func(c *astra.Ctx) error { return nil })

	req := httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
	if allow := w.Header().Get("Allow"); allow != "" {
		t.Errorf("404 must not set Allow header, got %q", allow)
	}
}

// containsMethod reports whether the comma-separated Allow header value
// contains the given HTTP method token.
func containsMethod(allow, method string) bool {
	start := 0
	for i := 0; i <= len(allow); i++ {
		if i == len(allow) || allow[i] == ',' {
			tok := allow[start:i]
			for len(tok) > 0 && tok[0] == ' ' {
				tok = tok[1:]
			}
			if tok == method {
				return true
			}
			start = i + 1
		}
	}
	return false
}
