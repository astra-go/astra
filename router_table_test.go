package astra_test

import (
	"testing"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/testutil"
)

// TestRouter_DispatchPriority registers a rich route set once and drives every
// dispatch path in a single table pass:
//
//   - static  > regex-constrained > :param > catch-all
//   - childIndex collision: three static siblings share the same first byte
//   - regex fallback to :param when the pattern does not match
//   - multiple regex patterns for the same segment followed by :param
//   - 404 for unknown paths and 405 for wrong method
func TestRouter_DispatchPriority(t *testing.T) {
	app := testutil.NewTestApp()

	// root
	app.GET("/", func(c *astra.Ctx) error { return c.String(200, "root") })

	// childIndex collision under /x:
	// "foo", "far", "faz" all share first byte 'f', so childIndex['f']
	// transitions to childIndexCollision and childMap is populated.
	app.GET("/x/foo", func(c *astra.Ctx) error { return c.String(200, "foo") })
	app.GET("/x/far", func(c *astra.Ctx) error { return c.String(200, "far") })
	app.GET("/x/faz", func(c *astra.Ctx) error { return c.String(200, "faz") })
	app.GET("/x/bar", func(c *astra.Ctx) error { return c.String(200, "bar") }) // different first byte

	// /users: static > regex > :param cascade
	app.GET("/users/list", func(c *astra.Ctx) error { return c.String(200, "list") })
	app.GET("/users/{id:[0-9]+}", func(c *astra.Ctx) error { return c.String(200, "num:%s", c.Param("id")) })
	app.GET("/users/:id", func(c *astra.Ctx) error { return c.String(200, "param:%s", c.Param("id")) })

	// catch-all
	app.GET("/files/*path", func(c *astra.Ctx) error { return c.String(200, "file:%s", c.Param("path")) })

	// /v: two regex alternatives plus :param fallback
	app.GET("/v/{ver:[0-9]+}", func(c *astra.Ctx) error { return c.String(200, "ver-num:%s", c.Param("ver")) })
	app.GET("/v/{ver:[a-z]+}", func(c *astra.Ctx) error { return c.String(200, "ver-lower:%s", c.Param("ver")) })
	app.GET("/v/:ver", func(c *astra.Ctx) error { return c.String(200, "ver-any:%s", c.Param("ver")) })

	// POST only — used for 405 check
	app.POST("/rpc", func(c *astra.Ctx) error { return c.String(201, "rpc") })

	srv := testutil.NewServer(t, app)

	cases := []struct {
		name   string
		method string
		path   string
		status int
		body   string // non-empty: AssertBodyContains
	}{
		// ─── root ─────────────────────────────────────────────────────────
		{"root", "GET", "/", 200, "root"},

		// ─── childIndex collision ──────────────────────────────────────────
		// Three children share first byte 'f'; childMap must resolve each one.
		{"collision-foo", "GET", "/x/foo", 200, "foo"},
		{"collision-far", "GET", "/x/far", 200, "far"},
		{"collision-faz", "GET", "/x/faz", 200, "faz"},
		// Control: different first byte must not be disturbed by the collision.
		{"no-collision-bar", "GET", "/x/bar", 200, "bar"},
		// Unknown path under /x → 404 (no :param registered).
		{"x-unknown", "GET", "/x/missing", 404, ""},

		// ─── static > regex > :param ───────────────────────────────────────
		{"static-beats-regex-param", "GET", "/users/list", 200, "list"},
		{"regex-beats-param-numeric", "GET", "/users/42", 200, "num:42"},
		{"regex-beats-param-single-digit", "GET", "/users/7", 200, "num:7"},
		// Regex [0-9]+ does not match alpha → falls through to :param.
		{"param-fallback-alpha", "GET", "/users/alice", 200, "param:alice"},
		{"param-fallback-mixed", "GET", "/users/abc123", 200, "param:abc123"},

		// ─── catch-all ────────────────────────────────────────────────────
		// Value includes the leading slash (path[pos-1:] from matchSegments).
		{"catch-all-flat", "GET", "/files/readme.txt", 200, "/readme.txt"},
		{"catch-all-deep", "GET", "/files/a/b/c.md", 200, "/a/b/c.md"},

		// ─── multiple regex patterns with :param fallback ─────────────────
		{"ver-num", "GET", "/v/5", 200, "ver-num:5"},
		{"ver-lower", "GET", "/v/beta", 200, "ver-lower:beta"},
		// Neither [0-9]+ nor [a-z]+ matches an upper-case string → :param wins.
		{"ver-param-upper", "GET", "/v/UPPER", 200, "ver-any:UPPER"},
		{"ver-param-mixed", "GET", "/v/Mix3d", 200, "ver-any:Mix3d"},

		// ─── error paths ──────────────────────────────────────────────────
		{"not-found", "GET", "/nonexistent", 404, ""},
		// PATCH has no registered tree at all; allowedMethods finds POST /rpc → 405.
		{"method-not-allowed", "PATCH", "/rpc", 405, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := srv.Do(tc.method, tc.path, nil)
			resp.AssertStatus(tc.status)
			if tc.body != "" {
				resp.AssertBodyContains(tc.body)
			}
		})
	}
}

// TestRouter_ChildIndexCollision_FourSiblings explicitly exercises the path
// where N=4 static children share the same first byte, forcing
// childIndex[byte] to childIndexCollision and populating childMap for every
// sibling added — including the fourth which is appended after the map already
// exists.  A :param node verifies fall-through for non-matching paths.
func TestRouter_ChildIndexCollision_FourSiblings(t *testing.T) {
	app := testutil.NewTestApp()

	// Four 'p'-prefixed static children — each registered individually so each
	// handler captures a distinct literal string.
	app.GET("/ns/page", func(c *astra.Ctx) error { return c.String(200, "page") })
	app.GET("/ns/post", func(c *astra.Ctx) error { return c.String(200, "post") })
	app.GET("/ns/profile", func(c *astra.Ctx) error { return c.String(200, "profile") })
	app.GET("/ns/photo", func(c *astra.Ctx) error { return c.String(200, "photo") })
	// :param catches paths whose static counterpart is absent from childMap.
	app.GET("/ns/:name", func(c *astra.Ctx) error { return c.String(200, "param:%s", c.Param("name")) })

	srv := testutil.NewServer(t, app)

	cases := []struct{ path, body string }{
		// All four 'p'-prefixed static paths must resolve via childMap.
		{"/ns/page", "page"},
		{"/ns/post", "post"},
		{"/ns/profile", "profile"},
		{"/ns/photo", "photo"},
		// Unknown 'p'-prefixed path: childMap lookup misses → falls to :param.
		{"/ns/pricing", "param:pricing"},
		// Non-'p' first byte: childIndex entry is absent/single → also :param.
		{"/ns/dashboard", "param:dashboard"},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			srv.GET(tc.path).AssertStatus(200).AssertBodyContains(tc.body)
		})
	}
}
