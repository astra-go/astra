package apollo_test

// Apollo mock-server tests for config/apollo.
//
// These tests stand up an httptest.Server that implements the subset of
// Apollo's HTTP API used by the agollo SDK, so no running Apollo instance
// is required.
//
// Apollo HTTP API endpoints implemented by the mock:
//
//	GET /configs/{appId}/{cluster}/{namespace}
//	    → returns namespace key-value pairs (initial fetch)
//
//	GET /notifications/v2
//	    → returns an empty array (no changes); blocks only until ctx cancel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	apollocfg "github.com/astra-go/astra/config/apollo"
)

// ─── Mock Apollo server ───────────────────────────────────────────────────────

type mockApolloServer struct {
	// kv holds the key-value pairs returned under the "application" namespace.
	kv map[string]string
	// notifyChange, when closed, makes the long-poll return a change event.
	notifyChange chan struct{}
}

func newMockApolloServer(kv map[string]string) *mockApolloServer {
	return &mockApolloServer{
		kv:           kv,
		notifyChange: make(chan struct{}),
	}
}

// ServeHTTP handles the two Apollo API endpoints agollo uses.
func (m *mockApolloServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path

	// GET /configs/{appId}/{cluster}/{namespace}
	if strings.HasPrefix(p, "/configs/") {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"appId":          "test-app",
			"cluster":        "default",
			"namespaceName":  "application",
			"configurations": m.kv,
			"releaseKey":     "mock-release-key-001",
		})
		return
	}

	// GET /notifications/v2  (long-poll for changes)
	if strings.HasPrefix(p, "/notifications/v2") {
		// Block until either a change is signalled or the client disconnects.
		select {
		case <-m.notifyChange:
			// Return a non-empty notification so agollo re-fetches config.
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode([]map[string]any{
				{"namespaceName": "application", "notificationId": 1},
			})
		case <-r.Context().Done():
			// Client cancelled (test cleanup) — 304 means no change.
			w.WriteHeader(http.StatusNotModified)
		}
		return
	}

	w.WriteHeader(http.StatusNotFound)
}

// startMockApollo starts an httptest.Server backed by mockApolloServer.
func startMockApollo(t *testing.T, kv map[string]string) (*httptest.Server, *mockApolloServer) {
	t.Helper()
	mock := newMockApolloServer(kv)
	srv := httptest.NewServer(mock)
	t.Cleanup(srv.Close)
	return srv, mock
}

// ─── Load ─────────────────────────────────────────────────────────────────────

func TestMock_Load_ReturnsKV(t *testing.T) {
	srv, _ := startMockApollo(t, map[string]string{
		"db.host": "localhost",
		"db.port": "5432",
		"app.env": "test",
	})

	src, err := apollocfg.New(apollocfg.Config{
		AppID:    "test-app",
		MetaAddr: srv.URL,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	values, err := src.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	cases := map[string]string{
		"db.host": "localhost",
		"db.port": "5432",
		"app.env": "test",
	}
	for k, want := range cases {
		got, ok := values[k]
		if !ok {
			t.Errorf("key %q missing from Load result", k)
			continue
		}
		if got != want {
			t.Errorf("key %q: got %v, want %v", k, got, want)
		}
	}
}

func TestMock_Load_EmptyNamespace(t *testing.T) {
	srv, _ := startMockApollo(t, map[string]string{})

	src, err := apollocfg.New(apollocfg.Config{
		AppID:    "test-app",
		MetaAddr: srv.URL,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	values, err := src.Load()
	if err != nil {
		t.Fatalf("Load on empty namespace: %v", err)
	}
	if len(values) != 0 {
		t.Errorf("expected empty map, got %v", values)
	}
}

// ─── Name ─────────────────────────────────────────────────────────────────────

func TestMock_Name_ContainsAppIDAndNamespace(t *testing.T) {
	srv, _ := startMockApollo(t, map[string]string{"k": "v"})

	src, err := apollocfg.New(apollocfg.Config{
		AppID:    "my-service",
		MetaAddr: srv.URL,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	name := src.Name()
	if !strings.Contains(name, "my-service") {
		t.Errorf("Name() %q does not contain AppID", name)
	}
	if !strings.Contains(name, "application") {
		t.Errorf("Name() %q does not contain namespace", name)
	}
}

// ─── Watch ────────────────────────────────────────────────────────────────────

func TestMock_Watch_NotifiesOnChange(t *testing.T) {
	srv, mock := startMockApollo(t, map[string]string{"ver": "1"})

	src, err := apollocfg.New(apollocfg.Config{
		AppID:    "test-app",
		MetaAddr: srv.URL,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	notified := make(chan struct{}, 1)
	go func() {
		src.Watch(ctx, func() { //nolint:errcheck
			select {
			case notified <- struct{}{}:
			default:
			}
		})
	}()

	// Trigger a change notification after a short delay
	time.AfterFunc(200*time.Millisecond, func() {
		close(mock.notifyChange)
	})

	select {
	case <-notified:
		// success — Watch callback was invoked
	case <-time.After(4 * time.Second):
		t.Error("Watch: notify callback was not called within timeout")
	}
}

func TestMock_Watch_CancelStops(t *testing.T) {
	srv, _ := startMockApollo(t, map[string]string{})

	src, err := apollocfg.New(apollocfg.Config{
		AppID:    "test-app",
		MetaAddr: srv.URL,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- src.Watch(ctx, func() {})
	}()

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Watch returned unexpected error after cancel: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Watch did not return after ctx cancel")
	}
}

// ─── Defaults ────────────────────────────────────────────────────────────────

func TestMock_DefaultNamespace_IsApplication(t *testing.T) {
	srv, _ := startMockApollo(t, map[string]string{"x": "y"})

	// NamespaceName is intentionally left empty — should default to "application"
	src, err := apollocfg.New(apollocfg.Config{
		AppID:    "test-app",
		MetaAddr: srv.URL,
		// NamespaceName: "" — default
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Load should succeed (mock serves "application" namespace)
	if _, err := src.Load(); err != nil {
		t.Fatalf("Load with default namespace: %v", err)
	}
}

func TestMock_CustomNamespace(t *testing.T) {
	// The mock always serves the same kv regardless of the namespace path,
	// so this test verifies that a non-default namespace name is accepted by
	// the client (no panic / validation error) and Load returns data.
	srv, _ := startMockApollo(t, map[string]string{"feature.flag": "on"})

	src, err := apollocfg.New(apollocfg.Config{
		AppID:         "test-app",
		MetaAddr:      srv.URL,
		NamespaceName: "feature-flags",
	})
	if err != nil {
		t.Fatalf("New with custom namespace: %v", err)
	}

	values, err := src.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if values["feature.flag"] != "on" {
		t.Errorf("expected feature.flag=on, got %v", values["feature.flag"])
	}
}
