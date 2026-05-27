package security_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/astra-go/astra"
	sec "github.com/astra-go/astra/middleware/security"
	"github.com/astra-go/astra/testutil"
)

func TestSlidingWindow_AllowsWithinLimit(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.SlidingWindowWithConfig(sec.SlidingWindowConfig{
		Limit:   5,
		Window:  time.Second,
		KeyFunc: func(_ *astra.Ctx) string { return "fixed" },
	}))
	app.GET("/", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	for i := 0; i < 5; i++ {
		s.GET("/").AssertStatus(http.StatusOK)
	}
}

func TestSlidingWindow_RejectsWhenExceeded(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.SlidingWindowWithConfig(sec.SlidingWindowConfig{
		Limit:   1,
		Window:  time.Hour,
		KeyFunc: func(_ *astra.Ctx) string { return "fixed" },
	}))
	app.GET("/", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	s.GET("/").AssertStatus(http.StatusOK)
	s.GET("/").AssertStatus(http.StatusTooManyRequests)
}

func TestSlidingWindow_IndependentKeys(t *testing.T) {
	key := "key-a"
	app := testutil.NewTestApp()
	app.Use(sec.SlidingWindowWithConfig(sec.SlidingWindowConfig{
		Limit:   1,
		Window:  time.Hour,
		KeyFunc: func(_ *astra.Ctx) string { return key },
	}))
	app.GET("/", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	s.GET("/").AssertStatus(http.StatusOK)

	key = "key-b"
	s.GET("/").AssertStatus(http.StatusOK)
}

func TestSlidingWindow_PerKeyLimits(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.SlidingWindowWithConfig(sec.SlidingWindowConfig{
		Limit:  10,
		Window: time.Hour,
		KeyFunc: func(c *astra.Ctx) string {
			return c.Header("X-API-Key")
		},
		PerKeyLimits: map[string]int64{
			"free-key": 1,
		},
	}))
	app.GET("/", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	s.GET("/", map[string]string{"X-API-Key": "free-key"}).AssertStatus(http.StatusOK)
	s.GET("/", map[string]string{"X-API-Key": "free-key"}).AssertStatus(http.StatusTooManyRequests)

	s.GET("/", map[string]string{"X-API-Key": "premium-key"}).AssertStatus(http.StatusOK)
}

func TestRouteQuota_AppliesPerRouteLimit(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.RouteQuotaMiddleware(sec.RouteQuotaConfig{
		Routes: []sec.RouteQuota{
			{Prefix: "/api/upload", Limit: 1, Window: time.Hour},
		},
		DefaultLimit:  100,
		DefaultWindow: time.Hour,
		KeyFunc:       func(_ *astra.Ctx) string { return "user" },
	}))
	app.POST("/api/upload", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	s.POST("/api/upload", nil).AssertStatus(http.StatusOK)
	s.POST("/api/upload", nil).AssertStatus(http.StatusTooManyRequests)
}

func TestRouteQuota_DefaultLimitAppliesWhenNoMatch(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.RouteQuotaMiddleware(sec.RouteQuotaConfig{
		Routes: []sec.RouteQuota{
			{Prefix: "/slow", Limit: 1, Window: time.Hour},
		},
		DefaultLimit:  1,
		DefaultWindow: time.Hour,
		KeyFunc:       func(_ *astra.Ctx) string { return "user" },
	}))
	app.GET("/other", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	s.GET("/other").AssertStatus(http.StatusOK)
	s.GET("/other").AssertStatus(http.StatusTooManyRequests)
}

func TestRouteQuota_PrefixBoundaryMatching(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.RouteQuotaMiddleware(sec.RouteQuotaConfig{
		Routes: []sec.RouteQuota{
			{Prefix: "/api", Limit: 1, Window: time.Hour},
		},
		DefaultLimit:  100,
		DefaultWindow: time.Hour,
		KeyFunc:       func(_ *astra.Ctx) string { return "user" },
	}))
	app.GET("/apiv2/resource", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	s.GET("/apiv2/resource").AssertStatus(http.StatusOK)
	s.GET("/apiv2/resource").AssertStatus(http.StatusOK)
}
