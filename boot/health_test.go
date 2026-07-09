package boot

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// MockChecker 健康检查模拟器
type MockChecker struct {
	name    string
	err     error
	latency time.Duration
}

func (m *MockChecker) Name() string { return m.name }
func (m *MockChecker) Check(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(m.latency):
	}
	return m.err
}

func TestRegisterHealthChecker(t *testing.T) {
	svc := New("test-svc", WithoutConfig())

	// 注册健康检查器
	svc.RegisterHealthChecker(&MockChecker{name: "mysql", err: nil})
	svc.RegisterHealthChecker(&MockChecker{name: "redis", err: nil})

	if len(svc.healthCheckers) != 2 {
		t.Errorf("expected 2 health checkers, got %d", len(svc.healthCheckers))
	}
}

func TestRegisterHealthCheckerFunc(t *testing.T) {
	svc := New("test-svc", WithoutConfig())

	called := false
	svc.RegisterHealthCheckerFunc("custom-check", func(ctx context.Context) error {
		called = true
		return nil
	})

	if len(svc.healthCheckers) != 1 {
		t.Errorf("expected 1 health checker, got %d", len(svc.healthCheckers))
	}

	// 执行检查
	report := svc.CheckHealth(context.Background())
	if !called {
		t.Error("checker function was not called")
	}
	if report.Status != "ok" {
		t.Errorf("expected status ok, got %s", report.Status)
	}
}

func TestCheckHealthAllPass(t *testing.T) {
	svc := New("test-svc", WithoutConfig())
	svc.RegisterHealthChecker(&MockChecker{name: "mysql", err: nil})
	svc.RegisterHealthChecker(&MockChecker{name: "redis", err: nil})

	report := svc.CheckHealth(context.Background())

	if report.Status != "ok" {
		t.Errorf("expected status ok, got %s", report.Status)
	}
	if report.Passed != 2 {
		t.Errorf("expected 2 passed, got %d", report.Passed)
	}
	if report.Failed != 0 {
		t.Errorf("expected 0 failed, got %d", report.Failed)
	}
	if report.Total != 2 {
		t.Errorf("expected 2 total, got %d", report.Total)
	}
}

func TestCheckHealthPartialFail(t *testing.T) {
	svc := New("test-svc", WithoutConfig())
	svc.RegisterHealthChecker(&MockChecker{name: "mysql", err: nil})
	svc.RegisterHealthChecker(&MockChecker{name: "redis", err: fmt.Errorf("connection refused")})

	report := svc.CheckHealth(context.Background())

	if report.Status != "degraded" {
		t.Errorf("expected status degraded, got %s", report.Status)
	}
	if report.Passed != 1 {
		t.Errorf("expected 1 passed, got %d", report.Passed)
	}
	if report.Failed != 1 {
		t.Errorf("expected 1 failed, got %d", report.Failed)
	}
}

func TestCheckHealthAllFail(t *testing.T) {
	svc := New("test-svc", WithoutConfig())
	svc.RegisterHealthChecker(&MockChecker{name: "mysql", err: fmt.Errorf("db down")})
	svc.RegisterHealthChecker(&MockChecker{name: "redis", err: fmt.Errorf("redis down")})

	report := svc.CheckHealth(context.Background())

	if report.Status != "error" {
		t.Errorf("expected status error, got %s", report.Status)
	}
	if report.Passed != 0 {
		t.Errorf("expected 0 passed, got %d", report.Passed)
	}
	if report.Failed != 2 {
		t.Errorf("expected 2 failed, got %d", report.Failed)
	}
}

func TestCheckHealthWithLatency(t *testing.T) {
	svc := New("test-svc", WithoutConfig())
	svc.RegisterHealthChecker(&MockChecker{name: "slow-check", err: nil, latency: 10 * time.Millisecond})

	report := svc.CheckHealth(context.Background())

	if report.Status != "ok" {
		t.Errorf("expected status ok, got %s", report.Status)
	}
	if len(report.Checks) != 1 {
		t.Errorf("expected 1 check result, got %d", len(report.Checks))
	}
	if report.Checks[0].Latency == "" {
		t.Error("expected latency to be set")
	}
}

func TestHealthEndpointDetailed(t *testing.T) {
	svc := New("test-svc",
		WithoutConfig(),
		WithConfig(&Config{
			Health: HealthConfig{
				LivePath:  "/health/live",
				ReadyPath: "/health/ready",
				Detailed:  true,
			},
		}),
	)

	svc.RegisterHealthChecker(&MockChecker{name: "mysql", err: nil})
	svc.RegisterHealthChecker(&MockChecker{name: "redis", err: fmt.Errorf("connection refused")})

	// Ready 端点应该返回详细报告
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rec := httptest.NewRecorder()

	svc.App().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	// 检查响应包含详细信息
	body := rec.Body.String()
	if body == "" {
		t.Error("expected non-empty response body")
	}
	t.Logf("Response body: %s", body)
}

func TestHealthEndpointSimple(t *testing.T) {
	svc := New("test-svc",
		WithoutConfig(),
		WithConfig(&Config{
			Health: HealthConfig{
				LivePath:  "/health/live",
				ReadyPath: "/health/ready",
				Detailed:  false,
			},
		}),
	)

	svc.RegisterHealthChecker(&MockChecker{name: "mysql", err: nil})
	svc.RegisterHealthChecker(&MockChecker{name: "redis", err: fmt.Errorf("connection refused")})

	// Ready 端点应该返回简单状态（兼容旧行为）
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rec := httptest.NewRecorder()

	svc.App().ServeHTTP(rec, req)

	// 部分失败时仍返回 200（兼容旧行为）
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

func TestHealthEndpointAllFails(t *testing.T) {
	svc := New("test-svc",
		WithoutConfig(),
		WithConfig(&Config{
			Health: HealthConfig{
				LivePath:  "/health/live",
				ReadyPath: "/health/ready",
				Detailed:  true,
			},
		}),
	)

	svc.RegisterHealthChecker(&MockChecker{name: "mysql", err: fmt.Errorf("db down")})

	// 所有检查失败时应该返回 503
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rec := httptest.NewRecorder()

	svc.App().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", rec.Code)
	}
}

func TestDBCheckerInterface(t *testing.T) {
	// 验证 DBChecker 实现了 HealthChecker 接口
	var _ HealthChecker = &DBChecker{}
}

func TestRedisCheckerInterface(t *testing.T) {
	// 验证 RedisChecker 实现了 HealthChecker 接口
	var _ HealthChecker = &RedisChecker{}
}

func TestHTTPEndpointCheckerInterface(t *testing.T) {
	// 验证 HTTPEndpointChecker 实现了 HealthChecker 接口
	var _ HealthChecker = &HTTPEndpointChecker{}
}

func TestHTTPEndpointChecker(t *testing.T) {
	// 创建测试服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	checker := &HTTPEndpointChecker{
		NameValue: "upstream-svc",
		URL:      server.URL,
		Expected: http.StatusOK,
	}

	err := checker.Check(context.Background())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestHTTPEndpointCheckerUnexpectedStatus(t *testing.T) {
	// 创建返回 500 的测试服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	checker := &HTTPEndpointChecker{
		URL:      server.URL,
		Expected: http.StatusOK,
	}

	err := checker.Check(context.Background())
	if err == nil {
		t.Error("expected error for unexpected status")
	}
}
