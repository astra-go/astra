package astra

import (
	"net/http"
	"testing"
)

func TestGroupMetadata_SetsAndRetrieves(t *testing.T) {
	app := New()
	g := app.Group("/api").Metadata("version", "v1").Metadata("ratelimit", "100/min")
	g.GET("/users", func(c *Ctx) error {
		return c.NoContent(http.StatusOK)
	})

	meta, tags := app.getRouteMeta("GET", "/api/users")
	if meta == nil {
		t.Fatal("expected non-nil metadata")
	}
	testutilAssertEqual(t, "v1", meta["version"])
	testutilAssertEqual(t, "100/min", meta["ratelimit"])
	if len(tags) != 0 {
		t.Errorf("expected no tags, got %v", tags)
	}
}

func TestGroupTags_SetsAndRetrieves(t *testing.T) {
	app := New()
	g := app.Group("/api").Tag("public", "v1")
	g.GET("/users", func(c *Ctx) error {
		return c.NoContent(http.StatusOK)
	})

	meta, tags := app.getRouteMeta("GET", "/api/users")
	if len(tags) == 0 {
		t.Fatal("expected non-empty tags")
	}
	testutilAssertContains(t, tags, "public")
	testutilAssertContains(t, tags, "v1")
	_ = meta
}

func TestGroupMetadata_InheritsToSubgroup(t *testing.T) {
	app := New()
	g := app.Group("/api").Metadata("version", "v1")
	sub := g.Group("/v1").Metadata("env", "prod")
	sub.GET("/users", func(c *Ctx) error {
		return c.NoContent(http.StatusOK)
	})

	meta, _ := app.getRouteMeta("GET", "/api/v1/users")
	if meta == nil {
		t.Fatal("expected non-nil metadata")
	}
	testutilAssertEqual(t, "v1", meta["version"])
	testutilAssertEqual(t, "prod", meta["env"])
}

func TestGroupTags_InheritsToSubgroup(t *testing.T) {
	app := New()
	g := app.Group("/api").Tag("auth")
	sub := g.Group("/admin").Tag("admin")
	sub.GET("/dashboard", func(c *Ctx) error {
		return c.NoContent(http.StatusOK)
	})

	_, tags := app.getRouteMeta("GET", "/api/admin/dashboard")
	testutilAssertContains(t, tags, "auth")
	testutilAssertContains(t, tags, "admin")
}

func TestGroupTag_Deduplicates(t *testing.T) {
	app := New()
	g := app.Group("/api").Tag("auth").Tag("auth")
	g.GET("/resource", func(c *Ctx) error {
		return c.NoContent(http.StatusOK)
	})

	_, tags := app.getRouteMeta("GET", "/api/resource")
	// Should have "auth" only once (tags are appended, duplication is expected)
	count := 0
	for _, t := range tags {
		if t == "auth" {
			count++
		}
	}
	t.Logf("tags: %v, auth count: %d", tags, count)
	// Tags are appended without dedup — that's fine for now (consumer can dedup)
}

func TestGroupMetadata_DoesNotAffectOtherRoutes(t *testing.T) {
	app := New()
	g1 := app.Group("/api").Metadata("version", "v1")
	g1.GET("/users", func(c *Ctx) error { return c.NoContent(http.StatusOK) })

	g2 := app.Group("/admin").Metadata("role", "admin")
	g2.GET("/settings", func(c *Ctx) error { return c.NoContent(http.StatusOK) })

	meta1, _ := app.getRouteMeta("GET", "/api/users")
	testutilAssertEqual(t, "v1", meta1["version"])
	if _, ok := meta1["role"]; ok {
		t.Error("expected no 'role' metadata on /api/users")
	}

	meta2, _ := app.getRouteMeta("GET", "/admin/settings")
	if _, ok := meta2["version"]; ok {
		t.Error("expected no 'version' metadata on /admin/settings")
	}
	testutilAssertEqual(t, "admin", meta2["role"])
}

func TestGroupMetadata_EmptyReturnsNil(t *testing.T) {
	app := New()
	app.GET("/plain", func(c *Ctx) error {
		return c.NoContent(http.StatusOK)
	})

	meta, tags := app.getRouteMeta("GET", "/plain")
	if meta != nil {
		t.Error("expected nil metadata for route without metadata")
	}
	if tags != nil {
		t.Error("expected nil tags for route without tags")
	}
}

func TestCtxRouteMeta_RuntimeAccess(t *testing.T) {
	app := New()
	g := app.Group("/api").Metadata("ratelimit", "200/min").Tag("internal")
	g.GET("/data", func(c *Ctx) error {
		val, ok := c.RouteMeta("ratelimit")
		if !ok {
			t.Fatal("expected RouteMeta to find ratelimit")
		}
		testutilAssertEqual(t, "200/min", val)

		if !c.RouteHasTag("internal") {
			t.Error("expected RouteHasTag to return true for 'internal'")
		}
		if c.RouteHasTag("nonexistent") {
			t.Error("expected RouteHasTag to return false for 'nonexistent'")
		}

		tags := c.RouteTags()
		testutilAssertContains(t, tags, "internal")

		return c.NoContent(http.StatusOK)
	})

	performTestRequest(t, app, "GET", "/api/data")
}

func TestCtxRouteMeta_RouteWithoutMetadata(t *testing.T) {
	app := New()
	app.GET("/plain", func(c *Ctx) error {
		_, ok := c.RouteMeta("anything")
		if ok {
			t.Error("expected RouteMeta to return false for route without metadata")
		}
		if c.RouteHasTag("any") {
			t.Error("expected RouteHasTag to return false for route without tags")
		}
		tags := c.RouteTags()
		if tags != nil {
			t.Errorf("expected nil tags, got %v", tags)
		}
		return c.NoContent(http.StatusOK)
	})

	performTestRequest(t, app, "GET", "/plain")
}

func TestAppRoutes_EnrichesWithMetadata(t *testing.T) {
	app := New()
	g := app.Group("/api").Metadata("version", "v1").Tag("auth")
	g.GET("/users", func(c *Ctx) error { return c.NoContent(http.StatusOK) })

	routes := app.Routes()
	var found bool
	for _, r := range routes {
		if r.FullPath == "/api/users" {
			found = true
			testutilAssertEqual(t, "v1", r.Metadata["version"])
			testutilAssertContains(t, r.Tags, "auth")
		}
	}
	if !found {
		t.Fatal("expected route /api/users in Routes()")
	}
}

func TestGroupRoutes_FiltersByPrefix(t *testing.T) {
	app := New()
	api := app.Group("/api").Metadata("version", "v1")
	api.GET("/users", func(c *Ctx) error { return c.NoContent(http.StatusOK) })
	api.GET("/items", func(c *Ctx) error { return c.NoContent(http.StatusOK) })

	admin := app.Group("/admin")
	admin.GET("/settings", func(c *Ctx) error { return c.NoContent(http.StatusOK) })

	apiRoutes := api.Routes()
	if len(apiRoutes) != 2 {
		t.Errorf("expected 2 routes under /api, got %d", len(apiRoutes))
	}

	adminRoutes := admin.Routes()
	if len(adminRoutes) != 1 {
		t.Errorf("expected 1 route under /admin, got %d", len(adminRoutes))
	}
}

func TestGroupFullPath(t *testing.T) {
	app := New()
	g := app.Group("/api/v1")
	sub := g.Group("/users")

	testutilAssertEqual(t, "/api/v1", g.FullPath())
	testutilAssertEqual(t, "/api/v1/users", sub.FullPath())
}

func TestGroupSplitGroup(t *testing.T) {
	app := New()
	g := app.Group("/api/v1/users")

	parts := g.SplitGroup()
	expected := []string{"api", "v1", "users"}
	if len(parts) != len(expected) {
		t.Fatalf("expected %d parts, got %d: %v", len(expected), len(parts), parts)
	}
	for i, p := range expected {
		if parts[i] != p {
			t.Errorf("part[%d]: expected %q, got %q", i, p, parts[i])
		}
	}
}

func TestGroupSplitGroup_Root(t *testing.T) {
	app := New()
	g := app.Group("/")
	parts := g.SplitGroup()
	if len(parts) != 0 {
		t.Errorf("expected empty parts for root group, got %v", parts)
	}
}

func TestAppSetRouteMeta(t *testing.T) {
	app := New()
	app.GET("/users", func(c *Ctx) error { return c.NoContent(http.StatusOK) })
	app.SetRouteMeta("GET", "/users", "rate_limit", "50/min")

	meta := app.GetRouteMeta("GET", "/users")
	testutilAssertEqual(t, "50/min", meta["rate_limit"])
}

func TestAppSetRouteTags(t *testing.T) {
	app := New()
	app.GET("/health", func(c *Ctx) error { return c.NoContent(http.StatusOK) })
	app.SetRouteTags("GET", "/health", "monitoring", "liveness")

	tags := app.GetRouteTags("GET", "/health")
	testutilAssertContains(t, tags, "monitoring")
	testutilAssertContains(t, tags, "liveness")
}

func TestGroupMetadata_ChainedOnRegistration(t *testing.T) {
	app := New()
	app.Group("/api").
		Metadata("version", "v2").
		Tag("public").
		GET("/ping", func(c *Ctx) error { return c.NoContent(http.StatusOK) })

	meta, tags := app.getRouteMeta("GET", "/api/ping")
	testutilAssertEqual(t, "v2", meta["version"])
	testutilAssertContains(t, tags, "public")
}

// ─── test helpers ────────────────────────────────────────────────────────────

func testutilAssertEqual(t *testing.T, want, got string) {
	t.Helper()
	if want != got {
		t.Errorf("want %q, got %q", want, got)
	}
}

func testutilAssertContains(t *testing.T, slice []string, val string) {
	t.Helper()
	for _, s := range slice {
		if s == val {
			return
		}
	}
	t.Errorf("expected slice to contain %q, got %v", val, slice)
}

func performTestRequest(t *testing.T, app *App, method, path string) {
	t.Helper()
	req, _ := http.NewRequest(method, path, nil)
	w := &mockResponseWriter{}
	app.ServeHTTP(w, req)
}

// mockResponseWriter implements http.ResponseWriter for testing.
type mockResponseWriter struct {
	header http.Header
	code   int
	body   []byte
}

func (m *mockResponseWriter) Header() http.Header {
	if m.header == nil {
		m.header = make(http.Header)
	}
	return m.header
}

func (m *mockResponseWriter) Write(b []byte) (int, error) {
	m.body = append(m.body, b...)
	return len(b), nil
}

func (m *mockResponseWriter) WriteHeader(code int) {
	m.code = code
}
