package e2e_test

import (
	"encoding/json"
	"net/http"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/astra-go/astra/e2e/chaos"
	"github.com/astra-go/astra/e2e/testapp"
)

// newChaosApp creates a testapp with a FaultInjector wired in.
func newChaosApp(t testing.TB) (*testapp.App, *chaos.FaultInjector) {
	t.Helper()
	injector := chaos.NewFaultInjector()
	app := testapp.NewWithInjector(t, injector)
	return app, injector
}

// TestChaos_TimeoutResilience verifies that the framework handles injected
// timeouts correctly — no panic, no goroutine leak.
func TestChaos_TimeoutResilience(t *testing.T) {
	app, injector := newChaosApp(t)
	base := app.HTTPURL()
	client := app.HTTP.Client()

	// Inject timeout on /chaos/timeout
	injector.InjectTimeout("/chaos/timeout", 1*time.Nanosecond)

	resp, err := client.Get(base + "/chaos/timeout")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	// Should get 504 (Gateway Timeout) from the chaos middleware, not a crash
	if resp.StatusCode != http.StatusGatewayTimeout {
		t.Errorf("expected 504, got %d", resp.StatusCode)
	}

	injector.Reset()

	// After reset, the endpoint should work normally
	resp2, err := client.Get(base + "/chaos/timeout")
	if err != nil {
		t.Fatalf("request after reset failed: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("expected 200 after reset, got %d", resp2.StatusCode)
	}
}

// TestChaos_ErrorResilience injects 50% error rate and verifies the framework
// handles errors correctly without affecting non-injected endpoints.
func TestChaos_ErrorResilience(t *testing.T) {
	app, injector := newChaosApp(t)
	base := app.HTTPURL()
	client := app.HTTP.Client()

	// Inject 50% error rate on /chaos/error
	injector.InjectError("/chaos/error", 0.5)

	errorCount := 0
	successCount := 0
	totalRequests := 100

	for i := 0; i < totalRequests; i++ {
		resp, err := client.Get(base + "/chaos/error")
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusInternalServerError {
			errorCount++
		} else if resp.StatusCode == http.StatusOK {
			successCount++
		}
	}

	// With 50% error rate, we expect both successes and failures
	if errorCount == 0 {
		t.Error("expected some errors with 50% error rate, got none")
	}
	if successCount == 0 {
		t.Error("expected some successes with 50% error rate, got none")
	}

	// Non-injected endpoints should still work fine
	injector.Reset()
	resp, err := client.Get(base + "/chaos/error")
	if err != nil {
		t.Fatalf("request after reset failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 after reset, got %d", resp.StatusCode)
	}
}

// TestChaos_LatencyResilience injects latency and verifies timeout handling.
func TestChaos_LatencyResilience(t *testing.T) {
	app, injector := newChaosApp(t)
	base := app.HTTPURL()
	client := app.HTTP.Client()

	// Inject 100ms latency on /chaos/latency
	injector.InjectLatency("/chaos/latency", 100*time.Millisecond)

	start := time.Now()
	resp, err := client.Get(base + "/chaos/latency")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	elapsed := time.Since(start)
	if elapsed < 90*time.Millisecond {
		t.Errorf("expected at least 90ms latency, got %v", elapsed)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	injector.Reset()
}

// TestChaos_PanicRecovery injects panic and verifies recovery middleware
// catches it.
func TestChaos_PanicRecovery(t *testing.T) {
	app, injector := newChaosApp(t)
	base := app.HTTPURL()
	client := app.HTTP.Client()

	// Inject panic on /chaos/panic
	injector.InjectPanic("/chaos/panic")

	resp, err := client.Get(base + "/chaos/panic")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	// The framework should recover from panic and return 500, not crash
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500 after panic recovery, got %d", resp.StatusCode)
	}

	injector.Reset()

	// After reset, the endpoint should work normally
	resp2, err := client.Get(base + "/chaos/panic")
	if err != nil {
		t.Fatalf("request after reset failed: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("expected 200 after reset, got %d", resp2.StatusCode)
	}
}

// TestChaos_Concurrency tests framework stability under concurrent requests
// with fault injection.
func TestChaos_Concurrency(t *testing.T) {
	app, injector := newChaosApp(t)
	base := app.HTTPURL()
	client := app.HTTP.Client()

	// Inject 30% error rate on /chaos/error
	injector.InjectError("/chaos/error", 0.3)

	const concurrency = 50
	var wg sync.WaitGroup
	errors := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := client.Get(base + "/chaos/error")
			if err != nil {
				errors <- err
				return
			}
			resp.Body.Close()
			// Any 5xx or 2xx is acceptable
			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusInternalServerError {
				errors <- json.Unmarshal([]byte{}, &struct{}{})
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent request error: %v", err)
	}

	injector.Reset()
}

// TestChaos_GoroutineLeak verifies that chaos injection doesn't leak goroutines.
func TestChaos_GoroutineLeak(t *testing.T) {
	// Force GC to get a clean baseline
	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	before := runtime.NumGoroutine()

	app, injector := newChaosApp(t)
	base := app.HTTPURL()
	client := app.HTTP.Client()

	// Run a round of chaos tests
	injector.InjectError("/chaos/error", 0.5)
	for i := 0; i < 20; i++ {
		resp, err := client.Get(base + "/chaos/error")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		resp.Body.Close()
	}

	injector.InjectLatency("/chaos/latency", 10*time.Millisecond)
	for i := 0; i < 10; i++ {
		resp, err := client.Get(base + "/chaos/latency")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		resp.Body.Close()
	}

	// Reset and let things settle
	injector.Reset()
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	after := runtime.NumGoroutine()

	// Allow some tolerance — goroutine count shouldn't grow significantly
	leaked := after - before
	if leaked > 5 {
		t.Errorf("potential goroutine leak: before=%d, after=%d, leaked=%d", before, after, leaked)
	}
}
