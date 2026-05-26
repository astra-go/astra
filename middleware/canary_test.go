package middleware_test

// ─── Canary middleware ─────────────────────────────────────────────────────────

import (
	"net/http"
	"testing"

	"github.com/astra-go/astra"
	sec "github.com/astra-go/astra/middleware/security"
	"github.com/astra-go/astra/testutil"
)

func newCanaryApp(rules []sec.CanaryRule) (*testutil.Server, *astra.App) {
	app := testutil.NewTestApp()
	app.Use(sec.Canary(rules))
	app.GET("/", func(c *astra.Ctx) error {
		v, _ := c.Get("canary_version")
		return c.JSON(http.StatusOK, map[string]any{"version": v})
	})
	return nil, app
}

// helper: run a GET with optional headers and read "version" from JSON body
func getCanaryVersion(t *testing.T, app *astra.App, headers map[string]string) string {
	t.Helper()
	s := testutil.NewServer(t, app)
	resp := s.GET("/", headers)
	var body struct {
		Version string `json:"version"`
	}
	resp.AssertJSON(&body)
	return body.Version
}

func TestCanary_HeaderExact_Match(t *testing.T) {
	_, app := newCanaryApp([]sec.CanaryRule{
		{Header: "X-Canary", Version: "v2"},
	})
	v := getCanaryVersion(t, app, map[string]string{"X-Canary": "anything"})
	testutil.AssertEqual(t, "v2", v)
}

func TestCanary_HeaderExact_NoMatch_EmptyVersion(t *testing.T) {
	_, app := newCanaryApp([]sec.CanaryRule{
		{Header: "X-Canary", Version: "v2"},
	})
	v := getCanaryVersion(t, app, nil)
	testutil.AssertEqual(t, "", v)
}

func TestCanary_HeaderRegex_Match(t *testing.T) {
	_, app := newCanaryApp([]sec.CanaryRule{
		{Header: "X-Canary", HeaderRE: "^true$", Version: "v2"},
	})
	v := getCanaryVersion(t, app, map[string]string{"X-Canary": "true"})
	testutil.AssertEqual(t, "v2", v)
}

func TestCanary_HeaderRegex_NoMatch(t *testing.T) {
	_, app := newCanaryApp([]sec.CanaryRule{
		{Header: "X-Canary", HeaderRE: "^true$", Version: "v2"},
	})
	v := getCanaryVersion(t, app, map[string]string{"X-Canary": "false"})
	testutil.AssertEqual(t, "", v)
}

func TestCanary_Cookie_Match(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.Canary([]sec.CanaryRule{
		{Cookie: "beta", Version: "v2"},
	}))
	app.GET("/", func(c *astra.Ctx) error {
		v, _ := c.Get("canary_version")
		return c.JSON(http.StatusOK, map[string]any{"version": v})
	})

	s := testutil.NewServer(t, app)
	resp := s.Do("GET", "/", nil, map[string]string{"Cookie": "beta=1"})
	var body struct {
		Version string `json:"version"`
	}
	resp.AssertJSON(&body)
	testutil.AssertEqual(t, "v2", body.Version)
}

func TestCanary_Cookie_Absent_NoMatch(t *testing.T) {
	_, app := newCanaryApp([]sec.CanaryRule{
		{Cookie: "beta", Version: "v2"},
	})
	v := getCanaryVersion(t, app, nil)
	testutil.AssertEqual(t, "", v)
}

func TestCanary_CookieRegex_Match(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.Canary([]sec.CanaryRule{
		{Cookie: "env", CookieRE: "^(staging|canary)$", Version: "v2"},
	}))
	app.GET("/", func(c *astra.Ctx) error {
		v, _ := c.Get("canary_version")
		return c.JSON(http.StatusOK, map[string]any{"version": v})
	})

	s := testutil.NewServer(t, app)
	resp := s.Do("GET", "/", nil, map[string]string{"Cookie": "env=canary"})
	var body struct{ Version string `json:"version"` }
	resp.AssertJSON(&body)
	testutil.AssertEqual(t, "v2", body.Version)
}

func TestCanary_HashRouting_DeterministicByUserID(t *testing.T) {
	// "user-0" → fnv hash % 10 should deterministically be in one bucket.
	// We just verify the same user always gets the same version.
	app := testutil.NewTestApp()
	app.Use(func(c *astra.Ctx) error {
		c.Set("user_id", "user-42")
		c.Next()
		return nil
	})
	app.Use(sec.Canary([]sec.CanaryRule{
		{UserIDKey: "user_id", Modulo: 2, Remainder: 0, Version: "v2"},
		{UserIDKey: "user_id", Modulo: 2, Remainder: 1, Version: "v1"},
	}))
	app.GET("/", func(c *astra.Ctx) error {
		v, _ := c.Get("canary_version")
		return c.JSON(http.StatusOK, map[string]any{"version": v})
	})

	s := testutil.NewServer(t, app)

	// First request — determine which version "user-42" maps to.
	resp1 := s.GET("/")
	var body1 struct{ Version string `json:"version"` }
	resp1.AssertJSON(&body1)
	firstVersion := body1.Version

	if firstVersion != "v1" && firstVersion != "v2" {
		t.Fatalf("unexpected version %q", firstVersion)
	}

	// Same user must always produce the same version.
	for i := 0; i < 5; i++ {
		resp := s.GET("/")
		var body struct{ Version string `json:"version"` }
		resp.AssertJSON(&body)
		testutil.AssertEqual(t, firstVersion, body.Version)
	}
}

func TestCanary_NoUserIDInContext_NoMatch(t *testing.T) {
	// UserIDKey set but no user_id in context → rule should not match.
	_, app := newCanaryApp([]sec.CanaryRule{
		{UserIDKey: "user_id", Modulo: 10, Remainder: 0, Version: "v2"},
	})
	v := getCanaryVersion(t, app, nil)
	testutil.AssertEqual(t, "", v)
}

func TestCanary_EmptyRules_StableVersion(t *testing.T) {
	_, app := newCanaryApp(nil)
	v := getCanaryVersion(t, app, nil)
	testutil.AssertEqual(t, "", v)
}

func TestCanary_FirstMatchWins(t *testing.T) {
	_, app := newCanaryApp([]sec.CanaryRule{
		{Header: "X-Env", HeaderRE: "^prod$", Version: "stable"},
		{Header: "X-Env", Version: "v2"}, // broader rule — should NOT win
	})
	v := getCanaryVersion(t, app, map[string]string{"X-Env": "prod"})
	testutil.AssertEqual(t, "stable", v)
}

func TestCanary_NoCondition_NeverMatches(t *testing.T) {
	// A rule with no conditions (all zero) must never match.
	_, app := newCanaryApp([]sec.CanaryRule{
		{Version: "v2"}, // no Header, Cookie, or UserIDKey
	})
	v := getCanaryVersion(t, app, nil)
	testutil.AssertEqual(t, "", v)
}
