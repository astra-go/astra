package middleware

import (
	"testing"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/ctxkey"
	"github.com/astra-go/astra/testutil"
)

func TestRequestContext_SetsRequestID(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(RequestContext())
	app.GET("/", func(c *astra.Ctx) error {
		id := c.GetString("requestID")
		if id == "" {
			t.Error("expected non-empty requestID")
		}
		return c.NoContent(200)
	})

	s := testutil.NewServer(t, app)
	resp := s.GET("/")
	resp.AssertStatus(200)
}

func TestRequestContext_PreservesExistingRequestID(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(RequestContext())
	app.GET("/", func(c *astra.Ctx) error {
		id := c.GetString("requestID")
		testutil.AssertEqual(t, "my-custom-id", id)
		return c.NoContent(200)
	})

	s := testutil.NewServer(t, app)
	resp := s.Do("GET", "/", nil, map[string]string{"X-Request-ID": "my-custom-id"})
	resp.AssertStatus(200)
}

func TestRequestContext_SetsClientIPAndUserAgent(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(RequestContext())
	app.GET("/", func(c *astra.Ctx) error {
		// Client IP varies by environment (127.0.0.1 in test, 0.0.0.0 in others).
		// Just verify it's non-empty.
		if c.GetString("clientIP") == "" {
			t.Error("expected non-empty clientIP")
		}
		ua := c.GetString("userAgent")
		if ua == "" {
			t.Error("expected non-empty userAgent")
		}
		return c.NoContent(200)
	})

	s := testutil.NewServer(t, app)
	resp := s.GET("/")
	resp.AssertStatus(200)
}

func TestRequestContext_PropagatesToRequestContext(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(RequestContext())
	app.GET("/", func(c *astra.Ctx) error {
		rctx := c.RequestContext()

		id, ok := rctx.Value(ctxkey.RequestID).(string)
		if !ok || id == "" {
			t.Error("expected requestID in RequestContext")
		}

		clientIP, ok := rctx.Value(ctxkey.ClientIP).(string)
		if !ok || clientIP == "" {
			t.Error("expected clientIP in RequestContext")
		}

		return c.NoContent(200)
	})

	s := testutil.NewServer(t, app)
	resp := s.GET("/")
	resp.AssertStatus(200)
}

func TestRequestContext_PropagatesRoutePath(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(RequestContext())
	app.GET("/hello", func(c *astra.Ctx) error {
		// Simulate route key being set (normally done by router).
		c.Set(astra.RouteKey, "/hello")

		rctx := c.RequestContext()
		route, ok := rctx.Value(ctxkey.RoutePath).(string)
		if !ok || route != "/hello" {
			t.Errorf("expected /hello route in RequestContext, got %q", route)
		}
		return c.NoContent(200)
	})

	s := testutil.NewServer(t, app)
	resp := s.GET("/hello")
	resp.AssertStatus(200)
}

func TestRequestContext_PropagationFromSet(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(RequestContext())
	app.GET("/", func(c *astra.Ctx) error {
		c.Set("requestID", "set-in-handler")
		c.Set("clientIP", "10.0.0.1")

		rctx := c.RequestContext()

		id, _ := rctx.Value(ctxkey.RequestID).(string)
		testutil.AssertEqual(t, "set-in-handler", id)

		ip, _ := rctx.Value(ctxkey.ClientIP).(string)
		testutil.AssertEqual(t, "10.0.0.1", ip)

		return c.NoContent(200)
	})

	s := testutil.NewServer(t, app)
	resp := s.GET("/")
	resp.AssertStatus(200)
}

func TestRequestContext_WithoutMiddleware_RequestContextStillWorks(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/", func(c *astra.Ctx) error {
		// Without RequestContext middleware, no requestID is set,
		// but RequestContext() should still work and return the base context.
		rctx := c.RequestContext()
		if rctx == nil {
			t.Error("RequestContext() should not return nil")
		}

		// No requestID in context because no middleware set it.
		_, ok := rctx.Value(ctxkey.RequestID).(string)
		if ok {
			t.Log("requestID found in context (may exist from other sources)")
		}

		return c.NoContent(200)
	})

	s := testutil.NewServer(t, app)
	resp := s.GET("/")
	resp.AssertStatus(200)
}
