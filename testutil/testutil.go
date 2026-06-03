// Package testutil provides testing helpers for Astra applications.
//
// # HTTP integration testing
//
//	func TestUserAPI(t *testing.T) {
//	    app := astra.New()
//	    app.GET("/users/:id", getUserHandler)
//
//	    srv := testutil.NewServer(t, app)
//
//	    resp := srv.GET("/users/42")
//	    resp.AssertStatus(200)
//
//	    var body struct{ ID int `json:"id"` }
//	    resp.AssertJSON(&body)
//	    // body.ID == 42
//	}
//
// # Assertion helpers
//
//	resp.AssertStatus(http.StatusCreated).
//	     AssertBodyContains(`"id"`).
//	     AssertHeader("Content-Type", "application/json; charset=utf-8")
package testutil

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/astra-go/astra"
)

// ─── Test server ──────────────────────────────────────────────────────────────

// Server wraps an astra.App in an httptest.Server for black-box HTTP testing.
// It is automatically closed when the test ends via t.Cleanup.
type Server struct {
	// App is the underlying Astra application — add routes and middleware here
	// before calling NewServer.
	App    *astra.App
	server *httptest.Server
	// T is the test helper; fatal errors are reported here.
	T testing.TB
}

// NewServer creates and starts a test HTTP server wrapping app.
// The server is registered with t.Cleanup so it stops when the test ends.
func NewServer(t testing.TB, app *astra.App) *Server {
	t.Helper()
	srv := &Server{App: app, T: t}
	srv.server = httptest.NewServer(app)
	t.Cleanup(srv.Close)
	return srv
}

// Close shuts down the test server.
func (s *Server) Close() { s.server.Close() }

// URL returns the base URL of the test server (e.g. "http://127.0.0.1:PORT").
func (s *Server) URL() string { return s.server.URL }

// Client returns an *http.Client configured to talk to this server (TLS-aware).
func (s *Server) Client() *http.Client { return s.server.Client() }

// ─── Request helpers ──────────────────────────────────────────────────────────

// GET performs a GET request against the test server.
func (s *Server) GET(path string, headers ...map[string]string) *Response {
	return s.do(http.MethodGet, path, nil, headers...)
}

// POST performs a POST request with body serialised as JSON.
func (s *Server) POST(path string, body any, headers ...map[string]string) *Response {
	return s.do(http.MethodPost, path, body, headers...)
}

// PUT performs a PUT request with body serialised as JSON.
func (s *Server) PUT(path string, body any, headers ...map[string]string) *Response {
	return s.do(http.MethodPut, path, body, headers...)
}

// PATCH performs a PATCH request with body serialised as JSON.
func (s *Server) PATCH(path string, body any, headers ...map[string]string) *Response {
	return s.do(http.MethodPatch, path, body, headers...)
}

// DELETE performs a DELETE request.
func (s *Server) DELETE(path string, headers ...map[string]string) *Response {
	return s.do(http.MethodDelete, path, nil, headers...)
}

// Do performs a raw request, giving full control over method, path, and body.
func (s *Server) Do(method, path string, body any, headers ...map[string]string) *Response {
	return s.do(method, path, body, headers...)
}

func (s *Server) do(method, path string, body any, headers ...map[string]string) *Response {
	s.T.Helper()

	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			s.T.Fatalf("testutil: marshal body: %v", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(context.Background(), method, s.server.URL+path, bodyReader)
	if err != nil {
		s.T.Fatalf("testutil: new request %s %s: %v", method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, h := range headers {
		for k, v := range h {
			req.Header.Set(k, v)
		}
	}

	resp, err := s.server.Client().Do(req)
	if err != nil {
		s.T.Fatalf("testutil: execute request %s %s: %v", method, path, err)
	}
	return newResponse(s.T, resp)
}

// ─── Response assertions ──────────────────────────────────────────────────────

// Response wraps *http.Response with chainable assertion helpers.
// The body is read once and cached; all assertion methods share the same copy.
type Response struct {
	T    testing.TB
	resp *http.Response
	once sync.Once
	body []byte
	err  error
}

func newResponse(t testing.TB, resp *http.Response) *Response {
	return &Response{T: t, resp: resp}
}

func (r *Response) readBody() {
	r.once.Do(func() {
		defer r.resp.Body.Close()
		r.body, r.err = io.ReadAll(r.resp.Body)
	})
}

// Status returns the HTTP status code.
func (r *Response) Status() int { return r.resp.StatusCode }

// Body returns the raw response body. Safe to call multiple times.
func (r *Response) Body() []byte {
	r.readBody()
	return r.body
}

// BodyString returns the response body as a string.
func (r *Response) BodyString() string { return string(r.Body()) }

// Header returns the value of the given response header.
func (r *Response) Header(key string) string { return r.resp.Header.Get(key) }

// AssertStatus asserts the response status code equals want. Returns r for chaining.
func (r *Response) AssertStatus(want int) *Response {
	r.T.Helper()
	if r.resp.StatusCode != want {
		r.T.Errorf("testutil: status: want %d, got %d (body: %s)",
			want, r.resp.StatusCode, r.Body())
	}
	return r
}

// AssertJSON decodes the JSON body into v.
// Fails the test if decoding fails. Returns r for chaining.
func (r *Response) AssertJSON(v any) *Response {
	r.T.Helper()
	r.readBody()
	if r.err != nil {
		r.T.Errorf("testutil: read body: %v", r.err)
		return r
	}
	if err := json.Unmarshal(r.body, v); err != nil {
		r.T.Errorf("testutil: decode JSON: %v (body: %s)", err, r.body)
	}
	return r
}

// AssertBodyContains asserts the response body contains the given substring.
func (r *Response) AssertBodyContains(substr string) *Response {
	r.T.Helper()
	body := r.BodyString()
	if !strings.Contains(body, substr) {
		r.T.Errorf("testutil: body does not contain %q (body: %s)", substr, body)
	}
	return r
}

// AssertBodyNotContains asserts the response body does NOT contain the substring.
func (r *Response) AssertBodyNotContains(substr string) *Response {
	r.T.Helper()
	body := r.BodyString()
	if strings.Contains(body, substr) {
		r.T.Errorf("testutil: body should not contain %q (body: %s)", substr, body)
	}
	return r
}

// AssertHeader asserts that the response header key has the expected value.
func (r *Response) AssertHeader(key, want string) *Response {
	r.T.Helper()
	got := r.resp.Header.Get(key)
	if got != want {
		r.T.Errorf("testutil: header %q: want %q, got %q", key, want, got)
	}
	return r
}

// AssertHeaderContains asserts that the response header key contains the substring.
func (r *Response) AssertHeaderContains(key, substr string) *Response {
	r.T.Helper()
	got := r.resp.Header.Get(key)
	if !strings.Contains(got, substr) {
		r.T.Errorf("testutil: header %q: want to contain %q, got %q", key, substr, got)
	}
	return r
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// NewTestApp creates an astra.App configured for testing:
//   - Mode: test (no OS signal handling)
//   - No default middleware
func NewTestApp(opts ...astra.Option) *astra.App {
	return astra.New(append([]astra.Option{astra.WithMode(astra.ModeTest)}, opts...)...)
}

// AssertNoError fails the test if err is non-nil.
func AssertNoError(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("testutil: unexpected error: %v", err)
	}
}

// AssertError fails the test if err is nil.
func AssertError(t testing.TB, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("testutil: expected an error but got nil")
	}
}

// AssertErrorIs fails the test if errors.Is(err, target) is false.
func AssertErrorIs(t testing.TB, err, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Errorf("testutil: errors.Is(%v, %v) = false", err, target)
	}
}

// AssertEqual fails the test when got != want using fmt.Sprintf comparison.
func AssertEqual[T comparable](t testing.TB, want, got T) {
	t.Helper()
	if want != got {
		t.Errorf("testutil: want %v, got %v", want, got)
	}
}
