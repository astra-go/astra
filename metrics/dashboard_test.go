package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRegistry_RecordRequest(t *testing.T) {
	registry := NewRegistry()

	// Record some requests
	registry.RecordRequest("/api/users", "GET", 200, 10*time.Millisecond)
	registry.RecordRequest("/api/users", "GET", 200, 20*time.Millisecond)
	registry.RecordRequest("/api/users", "GET", 500, 100*time.Millisecond)
	registry.RecordRequest("/api/posts", "POST", 201, 30*time.Millisecond)

	stats := registry.RequestStats()

	if stats.TotalRequests() != 4 {
		t.Errorf("expected 4 total requests, got %d", stats.TotalRequests())
	}

	if stats.ErrorRate() != 0.25 {
		t.Errorf("expected error rate 0.25, got %f", stats.ErrorRate())
	}
}

func TestRegistry_PoolStats(t *testing.T) {
	registry := NewRegistry()

	registry.UpdatePoolStats("db", 5, 10)
	registry.RecordPoolOperation("db", "hit")
	registry.RecordPoolOperation("db", "miss")

	stats := registry.PoolStats()

	if stats.Pools["db"] == nil {
		t.Fatal("expected db pool in stats")
	}

	if stats.Pools["db"].Active != 5 {
		t.Errorf("expected 5 active, got %d", stats.Pools["db"].Active)
	}

	if stats.Pools["db"].Idle != 10 {
		t.Errorf("expected 10 idle, got %d", stats.Pools["db"].Idle)
	}
}

func TestHistogram_Percentile(t *testing.T) {
	hist := NewHistogram()

	// Record 100 values from 1 to 100
	for i := 1; i <= 100; i++ {
		hist.Record(time.Duration(i) * time.Millisecond)
	}

	p50 := hist.Percentile(50)
	if p50 < 40*1e6 || p50 > 60*1e6 { // 40-60ms range for p50
		t.Errorf("expected p50 around 50ms, got %v", time.Duration(p50))
	}

	p99 := hist.Percentile(99)
	if p99 < 90*1e6 || p99 > 100*1e6 {
		t.Errorf("expected p99 around 99ms, got %v", time.Duration(p99))
	}
}

func TestServer_Dashboard(t *testing.T) {
	server := NewServer(":0")

	// Record some metrics
	registry := GlobalRegistry()
	registry.RecordRequest("/api/test", "GET", 200, 10*time.Millisecond)

	// Test dashboard endpoint
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	w := httptest.NewRecorder()

	// We need to test via the handler directly since we can't easily start the server
	server.handleDashboard(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Header().Get("Content-Type") != "text/html" {
		t.Errorf("expected text/html content type, got %s", w.Header().Get("Content-Type"))
	}
}

func TestServer_Health(t *testing.T) {
	server := NewServer(":0")

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	server.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestServer_Metrics(t *testing.T) {
	server := NewServer(":0")

	// Record some metrics
	registry := GlobalRegistry()
	registry.RecordRequest("/api/test", "GET", 200, 10*time.Millisecond)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	server.handleMetrics(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// Should contain Prometheus format
	body := w.Body.String()
	if len(body) == 0 {
		t.Error("expected non-empty metrics response")
	}
}

func TestRequestStats_TopEndpoints(t *testing.T) {
	registry := NewRegistry()

	// Record requests to different endpoints
	for i := 0; i < 100; i++ {
		registry.RecordRequest("/api/popular", "GET", 200, 10*time.Millisecond)
	}
	for i := 0; i < 50; i++ {
		registry.RecordRequest("/api/moderate", "GET", 200, 10*time.Millisecond)
	}
	for i := 0; i < 10; i++ {
		registry.RecordRequest("/api/rare", "GET", 200, 10*time.Millisecond)
	}

	stats := registry.RequestStats()
	top := stats.TopEndpoints()

	if len(top) == 0 {
		t.Fatal("expected at least one endpoint")
	}

	// Most popular should be first
	if top[0].Path != "/api/popular" {
		if top[0].Path != "/api/popular" { t.Errorf("expected /api/popular first, got %s", top[0].Path) }
	}
}

func TestSystemStats(t *testing.T) {
	stats := CollectSystemStats()

	if stats.MemAlloc == 0 {
		t.Error("expected non-zero memory allocation")
	}

	if stats.Goroutines == 0 {
		t.Error("expected non-zero goroutine count")
	}
}