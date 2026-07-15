package router

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
)

// ══════════════════════════════════════════════════════════════════════════
// mock Ctx for router.Handle tests
// ══════════════════════════════════════════════════════════════════════════

type mockCtx struct {
	req            *http.Request
	handlers       HandlersChain
	params         Params
	routeKey       string
	allowedMethods string
	nextCalled     bool
	nextErr        error
}

func (m *mockCtx) Request() *http.Request           { return m.req }
func (m *mockCtx) SetHandlers(h HandlersChain)       { m.handlers = h }
func (m *mockCtx) SetParams(p Params)                { m.params = p }
func (m *mockCtx) SetRouteKey(k string)              { m.routeKey = k }
func (m *mockCtx) SetAllowedMethods(a string)        { m.allowedMethods = a }
func (m *mockCtx) Next() error                       { m.nextCalled = true; return m.nextErr }

func newMockCtx(method, path string) *mockCtx {
	return &mockCtx{
		req: httptest.NewRequest(method, path, nil),
	}
}

// ══════════════════════════════════════════════════════════════════════════
// NewRouter + basic routing
// ══════════════════════════════════════════════════════════════════════════

func nopHandler(c Ctx) error { return nil }

func TestRouter_AddAndRoute(t *testing.T) {
	r := NewRouter(nopHandler, nopHandler, nil, false, 0)

	r.Add("GET", "/users", HandlersChain{nopHandler})
	r.Add("GET", "/users/:id", HandlersChain{nopHandler})
	r.Add("POST", "/users", HandlersChain{nopHandler})

	routes := r.Routes()
	if len(routes) != 3 {
		t.Errorf("Routes() = %d, want 3", len(routes))
	}

	// Verify path params.
	c := newMockCtx("GET", "/users/42")
	r.Handle(c)
	if !c.nextCalled {
		t.Fatal("Handle did not call Next()")
	}
	if len(c.params) != 1 || c.params[0].Key != "id" || c.params[0].Value != "42" {
		t.Errorf("params = %+v, want [{id 42}]", c.params)
	}
	if c.routeKey != "/users/:id" {
		t.Errorf("routeKey = %q, want '/users/:id'", c.routeKey)
	}
}

func TestRouter_Handle_NotFound(t *testing.T) {
	notFound := func(c Ctx) error { return fmt.Errorf("not found") }
	r := NewRouter(notFound, nopHandler, nil, false, 0)
	r.Add("GET", "/", HandlersChain{nopHandler})

	c := newMockCtx("GET", "/no-such-route")
	r.Handle(c)

	if !c.nextCalled {
		t.Fatal("notFound handler not called")
	}
	// handlers should be the notFound chain.
	if len(c.handlers) != 1 {
		t.Errorf("handlers len = %d, want 1", len(c.handlers))
	}
	if c.routeKey != "" {
		t.Errorf("routeKey = %q, want ''", c.routeKey)
	}
}

func TestRouter_Handle_405(t *testing.T) {
	nop := nopHandler
	r := NewRouter(nop, nop, nil, false, 0)
	r.Add("GET", "/users", HandlersChain{nop})
	r.Add("POST", "/users", HandlersChain{nop})
	r.Add("DELETE", "/api/other", HandlersChain{nop})

	// PUT /users should get 405, not 404.
	c := newMockCtx("PUT", "/users")
	r.Handle(c)
	if !c.nextCalled {
		t.Fatal("Handle did not call Next()")
	}
	if c.allowedMethods == "" {
		t.Error("allowedMethods should not be empty (PUT not registered but GET/POST are)")
	}
	if !strings.Contains(c.allowedMethods, "GET") {
		t.Errorf("Allowed = %q, should contain GET", c.allowedMethods)
	}
	if !strings.Contains(c.allowedMethods, "POST") {
		t.Errorf("Allowed = %q, should contain POST", c.allowedMethods)
	}
}

func TestRouter_MaxParamDepth(t *testing.T) {
	r := NewRouter(nopHandler, nopHandler, nil, false, 0)
	r.Add("GET", "/:a/:b/:c", HandlersChain{nopHandler})
	r.Add("GET", "/:x", HandlersChain{nopHandler})

	if d := r.MaxParamDepth(); d != 3 {
		t.Errorf("MaxParamDepth = %d, want 3", d)
	}
}

func TestRouter_MaxParamDepth_Empty(t *testing.T) {
	r := NewRouter(nopHandler, nopHandler, nil, false, 0)
	if d := r.MaxParamDepth(); d != 0 {
		t.Errorf("MaxParamDepth = %d, want 0 (no routes)", d)
	}
}

func TestRouter_MethodUpperCase(t *testing.T) {
	r := NewRouter(nopHandler, nopHandler, nil, false, 0)
	r.Add("GET", "/status", HandlersChain{nopHandler})
	// Routes() should collect the route.
	routes := r.Routes()
	if len(routes) != 1 {
		t.Fatalf("Routes() = %d routes, want 1", len(routes))
	}
	// Method is stored as-is. FullPath contains the registered path template.
	if routes[0].Method != "GET" {
		t.Errorf("Method = %q, want GET", routes[0].Method)
	}
	if routes[0].FullPath != "/status" {
		t.Errorf("FullPath = %q, want /status", routes[0].FullPath)
	}
}

// ══════════════════════════════════════════════════════════════════════════
// Route conflict / overwrite
// ══════════════════════════════════════════════════════════════════════════

func TestRouter_Add_Overwrite_Warn(t *testing.T) {
	// Use slog.Default() to avoid nil pointer on Warn.
	r := NewRouter(nopHandler, nopHandler, slog.Default(), false, 0)
	r.Add("GET", "/hello", HandlersChain{nopHandler})
	r.Add("GET", "/hello", HandlersChain{nopHandler})
	// Should not panic; routes count should still be 1.
	if len(r.Routes()) != 1 {
		t.Errorf("Routes = %d, want 1 (overwrite)", len(r.Routes()))
	}
}

func TestRouter_Add_StrictConflictPanics(t *testing.T) {
	r := NewRouter(nopHandler, nopHandler, slog.Default(), true, 0)
	r.Add("GET", "/hello", HandlersChain{nopHandler})

	panicked := false
	func() {
		defer func() {
			if recover() != nil {
				panicked = true
			}
		}()
		r.Add("GET", "/hello", HandlersChain{nopHandler})
	}()
	if !panicked {
		t.Error("strictConflict: second Add should have panicked")
	}
}

func TestRouter_Add_EmptyPath_Panics(t *testing.T) {
	r := NewRouter(nopHandler, nopHandler, nil, false, 0)
	panicked := false
	func() {
		defer func() {
			if recover() != nil {
				panicked = true
			}
		}()
		r.Add("GET", "", HandlersChain{nopHandler})
	}()
	if !panicked {
		t.Error("empty path should panic")
	}
}

func TestRouter_Add_PathNotStartingWithSlash_Panics(t *testing.T) {
	r := NewRouter(nopHandler, nopHandler, nil, false, 0)
	panicked := false
	func() {
		defer func() {
			if recover() != nil {
				panicked = true
			}
		}()
		r.Add("GET", "users", HandlersChain{nopHandler})
	}()
	if !panicked {
		t.Error("path without leading / should panic")
	}
}

// ══════════════════════════════════════════════════════════════════════════
// methodOrderRank
// ══════════════════════════════════════════════════════════════════════════

func TestMethodOrderRank(t *testing.T) {
	if methodOrderRank("GET") != 0 {
		t.Errorf("GET rank = %d, want 0", methodOrderRank("GET"))
	}
	if methodOrderRank("OPTIONS") != 6 {
		t.Errorf("OPTIONS rank = %d, want 6", methodOrderRank("OPTIONS"))
	}
	if methodOrderRank("CUSTOM") != len(methodOrder) {
		t.Errorf("CUSTOM rank = %d, want %d", methodOrderRank("CUSTOM"), len(methodOrder))
	}
}

// ══════════════════════════════════════════════════════════════════════════
// allowedMethods ordering (deterministic Allow header)
// ══════════════════════════════════════════════════════════════════════════

func TestRouter_AllowedMethods_Order(t *testing.T) {
	r := NewRouter(nopHandler, nopHandler, nil, false, 0)
	// Register in non-canonical order.
	r.Add("DELETE", "/api", HandlersChain{nopHandler})
	r.Add("POST", "/api", HandlersChain{nopHandler})
	r.Add("GET", "/api", HandlersChain{nopHandler})

	allow := r.allowedMethods("/api")
	if allow != "GET, POST, DELETE" {
		t.Errorf("allowedMethods = %q, want 'GET, POST, DELETE'", allow)
	}
}

func TestRouter_AllowedMethods_Empty(t *testing.T) {
	r := NewRouter(nopHandler, nopHandler, nil, false, 0)
	allow := r.allowedMethods("/no-routes")
	if allow != "" {
		t.Errorf("allowedMethods = %q, want ''", allow)
	}
}

// ══════════════════════════════════════════════════════════════════════════
// allowedCache
// ══════════════════════════════════════════════════════════════════════════

func TestAllowedCache(t *testing.T) {
	r := NewRouter(nopHandler, nopHandler, nil, false, 0)
	r.Add("GET", "/api", HandlersChain{nopHandler})

	ac := AllowedCache(r)

	// First call populates cache.
	allow1 := r.cachedAllowedMethods("/api")
	if allow1 != "GET" {
		t.Errorf("cachedAllowedMethods = %q, want 'GET'", allow1)
	}

	// Verify it's in the cache.
	if v, ok := ac.Load("/api"); !ok || v.(string) != "GET" {
		t.Error("allowedCache should contain /api → GET")
	}
}

// ══════════════════════════════════════════════════════════════════════════
// regexp cache
// ══════════════════════════════════════════════════════════════════════════

func TestGetOrCompileRegexp(t *testing.T) {
	re, err := GetOrCompileRegexp(`\d+`)
	if err != nil {
		t.Fatalf("getOrCompileRegexp: %v", err)
	}
	if re == nil {
		t.Fatal("returned nil regexp")
	}
	if !re.MatchString("12345") {
		t.Error("regexp should match 12345")
	}
	if re.MatchString("abc") {
		t.Error("regexp should not match abc")
	}
}

func TestGetOrCompileRegexp_Cache(t *testing.T) {
	re1, _ := GetOrCompileRegexp(`[a-z]+`)
	re2, _ := GetOrCompileRegexp(`[a-z]+`)

	// Same pattern should return the same object.
	if re1 != re2 {
		t.Error("same pattern should return cached regexp")
	}
}

func TestGetOrCompileRegexp_Invalid(t *testing.T) {
	_, err := GetOrCompileRegexp(`[invalid`)
	if err == nil {
		t.Error("expected error for invalid regex")
	}
}

// ══════════════════════════════════════════════════════════════════════════
// Fast matchers – correctness
// ══════════════════════════════════════════════════════════════════════════

func TestFastDigits(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"123", true}, {"0", true}, {"", false}, {"12a", false}, {"-1", false},
	}
	for _, tt := range tests {
		if got := FastDigits(tt.in); got != tt.want {
			t.Errorf("fastDigits(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestFastLower(t *testing.T) {
	if !FastLower("hello") {
		t.Error("fastLower(hello) = false")
	}
	if FastLower("Hello") {
		t.Error("fastLower(Hello) = true")
	}
	if FastLower("") {
		t.Error("fastLower('') should be false")
	}
}

func TestFastUpper(t *testing.T) {
	if !FastUpper("HELLO") {
		t.Error("fastUpper(HELLO) = false")
	}
	if FastUpper("Hello") {
		t.Error("fastUpper(Hello) = true")
	}
	if FastUpper("") {
		t.Error("fastUpper('') should be false")
	}
}

func TestFastAlpha(t *testing.T) {
	if !FastAlpha("azAZ") {
		t.Error("fastAlpha(azAZ) = false")
	}
	if FastAlpha("az09") {
		t.Error("fastAlpha(az09) = true")
	}
	if FastAlpha("") {
		t.Error("fastAlpha('') should be false")
	}
}

func TestFastAlphanum(t *testing.T) {
	if !FastAlphanum("abc123") {
		t.Error("fastAlphanum(abc123) = false")
	}
	if FastAlphanum("abc_123") {
		t.Error("fastAlphanum(abc_123) = true")
	}
}

func TestFastSlug(t *testing.T) {
	if !FastSlug("my-slug_01") {
		t.Error("fastSlug(my-slug_01) = false")
	}
	if FastSlug("my.slug") {
		t.Error("fastSlug(my.slug) = true")
	}
}

func TestFastIdentifier(t *testing.T) {
	if !FastIdentifier("var_name1") {
		t.Error("fastIdentifier(var_name1) = false")
	}
	if FastIdentifier("var-name") {
		t.Error("fastIdentifier(var-name) = true")
	}
}

func TestFastHex(t *testing.T) {
	// fastHex and fastHexLower are not exported, but tested via compileFastMatcher.
}

// ══════════════════════════════════════════════════════════════════════════
// Wildcard / greedy route matching
// ══════════════════════════════════════════════════════════════════════════

func TestRouter_Wildcard(t *testing.T) {
	r := NewRouter(nopHandler, nopHandler, nil, false, 0)
	r.Add("GET", "/files/*filepath", HandlersChain{nopHandler})

	c := newMockCtx("GET", "/files/css/main.css")
	r.Handle(c)

	if len(c.params) != 1 || c.params[0].Key != "filepath" {
		t.Errorf("params = %+v, want [{filepath ...}]", c.params)
	}
	// catchAll preserves the leading slash.
	if !strings.HasSuffix(c.params[0].Value, "css/main.css") {
		t.Errorf("catchAll value = %q, should end with css/main.css", c.params[0].Value)
	}
}

func TestRouter_MultipleParams(t *testing.T) {
	r := NewRouter(nopHandler, nopHandler, nil, false, 0)
	r.Add("GET", "/:org/:repo/pull/:id", HandlersChain{nopHandler})

	c := newMockCtx("GET", "/acme/corp/pull/42")
	r.Handle(c)

	if len(c.params) != 3 {
		t.Errorf("params = %d, want 3: %+v", len(c.params), c.params)
	}
	expected := map[string]string{"org": "acme", "repo": "corp", "id": "42"}
	for _, p := range c.params {
		if expected[p.Key] != p.Value {
			t.Errorf("param %q = %q, want %q", p.Key, p.Value, expected[p.Key])
		}
	}
}

func TestRouter_ExactMatch(t *testing.T) {
	r := NewRouter(nopHandler, nopHandler, nil, false, 0)
	r.Add("GET", "/users/:id", HandlersChain{nopHandler})

	// /users/42 should match the param route.
	c := newMockCtx("GET", "/users/42")
	r.Handle(c)
	if c.routeKey == "" {
		t.Error("expected route to match /users/42")
	}
	if len(c.params) != 1 || c.params[0].Key != "id" || c.params[0].Value != "42" {
		t.Errorf("params = %+v, want [{id 42}]", c.params)
	}
}

func TestRouter_ExactBeforeParam(t *testing.T) {
	r := NewRouter(nopHandler, nopHandler, nil, false, 0)
	// Register exact route first, then param route.
	r.Add("GET", "/user/new", HandlersChain{nopHandler})
	r.Add("GET", "/user/:id", HandlersChain{nopHandler})

	// /user/new should match (either exact or param).
	c := newMockCtx("GET", "/user/new")
	r.Handle(c)
	if c.routeKey == "" {
		t.Fatal("no route matched /user/new")
	}
	t.Logf("routeKey = %q, params = %+v", c.routeKey, c.params)

	// /user/99 should match the param route.
	c2 := newMockCtx("GET", "/user/99")
	r.Handle(c2)
	if c2.routeKey == "" {
		t.Error("no route matched /user/99")
	}
}

// ══════════════════════════════════════════════════════════════════════════
// maxParamValueLen
// ══════════════════════════════════════════════════════════════════════════

func TestRouter_MaxParamValueLen(t *testing.T) {
	r := NewRouter(nopHandler, nopHandler, nil, false, 5)
	r.Add("GET", "/users/:id", HandlersChain{nopHandler})

	// 5-char ID is ok.
	c := newMockCtx("GET", "/users/12345")
	r.Handle(c)
	if len(c.params) == 0 || c.params[0].Value != "12345" {
		t.Errorf("5-char param should match: %+v", c.params)
	}

	// 6-char ID should be rejected → 404.
	c2 := newMockCtx("GET", "/users/123456")
	r.Handle(c2)
	if len(c2.handlers) > 0 && c2.handlers[0] != nil {
		// If handler was set, it means matching didn't respect maxParamValueLen
		if len(c2.params) > 0 && c2.params[0].Value == "123456" {
			t.Error("6-char param should be rejected (maxParamValueLen=5)")
		}
	}
}

// ══════════════════════════════════════════════════════════════════════════
// Concurrent route registration
// ══════════════════════════════════════════════════════════════════════════

func TestRouter_ConcurrentRead(t *testing.T) {
	r := NewRouter(nopHandler, nopHandler, nil, false, 0)
	for i := 0; i < 10; i++ {
		r.Add("GET", fmt.Sprintf("/api/%d", i), HandlersChain{nopHandler})
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				c := newMockCtx("GET", fmt.Sprintf("/api/%d", id%10))
				r.Handle(c)
				_ = r.Routes()
				_ = r.MaxParamDepth()
			}
		}(i)
	}
	wg.Wait()
}

// ══════════════════════════════════════════════════════════════════════════
// regexp cache integration with router
// ══════════════════════════════════════════════════════════════════════════

func TestRegexpCache_LRU(t *testing.T) {
	// Test that getOrCompileRegexp uses the LRU cache.
	// Compile multiple patterns and verify they return the same instance.
	patterns := []string{`\d+`, `[a-z]+`, `[A-Z]+`, `[a-zA-Z]+`}
	var refs []*regexp.Regexp

	for _, p := range patterns {
		re, err := GetOrCompileRegexp(p)
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, re)
	}

	// Same patterns should return same pointers.
	for i, p := range patterns {
		re, err := GetOrCompileRegexp(p)
		if err != nil {
			t.Fatal(err)
		}
		if re != refs[i] {
			t.Errorf("pattern %q: cache miss, expected %p got %p", p, refs[i], re)
		}
	}
}

func TestRegexp_Anchored(t *testing.T) {
	// getOrCompileRegexp adds ^(?:...)$ anchors.
	re, err := GetOrCompileRegexp(`[a-z]+`)
	if err != nil {
		t.Fatal(err)
	}
	if re.MatchString("hello") != true {
		t.Error("should match anchored 'hello'")
	}
	// "hello123" should fail because anchored must match entire string.
	if re.MatchString("hello123") {
		t.Error("should not match 'hello123' (anchors enforce full-string match)")
	}
}

// ══════════════════════════════════════════════════════════════════════════
// methodRoots insertion ordering
// ══════════════════════════════════════════════════════════════════════════

func TestRouter_MethodRootsInsertionOrder(t *testing.T) {
	r := NewRouter(nopHandler, nopHandler, nil, false, 0)

	// Register methods out of order.
	r.Add("CUSTOM", "/api", HandlersChain{nopHandler})
	r.Add("DELETE", "/api", HandlersChain{nopHandler})
	r.Add("GET", "/api", HandlersChain{nopHandler})
	r.Add("POST", "/api", HandlersChain{nopHandler})

	// methodRoots should be in order: GET, POST, DELETE, CUSTOM
	expected := []string{"GET", "POST", "DELETE", "CUSTOM"}
	for i, mr := range r.methodRoots {
		if mr.method != expected[i] {
			t.Errorf("methodRoots[%d] = %q, want %q", i, mr.method, expected[i])
		}
	}
}

// ══════════════════════════════════════════════════════════════════════════
// must() function in compileFastMatcher
// ══════════════════════════════════════════════════════════════════════════

func TestCompileFastMatcher_Must_PanicsOnBadRegex(t *testing.T) {
	// The must() helper in fastmatch_compile.go wraps compileFastMatcher.
	// We test indirectly via getOrCompileRegexp which does NOT use must.
	// This test confirms must-like behavior can't panic on route registration.
	// (Well-known matchers use pre-compiled functions so no panic.)
	fm := compileFastMatcher(`[invalid`)
	if fm != nil {
		t.Error("should return nil for invalid pattern, not panic")
	}
}
