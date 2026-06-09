package netengine_test

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/astra-go/astra/netengine"
)

// testHTTPClient is used in tests with proper timeouts.
var testHTTPClient = &http.Client{
	Timeout: 10 * time.Second,
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// startEngine creates a Reactor engine with the given handler, listens on a
// random port and returns the engine, the listener address, and a stop func.
func startEngine(t *testing.T, handler http.Handler, cfg netengine.ReactorConfig) (addr string, stop func()) {
	t.Helper()
	engine, err := netengine.New(handler, cfg)
	if err != nil {
		t.Skipf("netengine not supported on this platform: %v", err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() { errCh <- engine.Serve(ln) }()
	// Give the engine a moment to start accepting.
	time.Sleep(10 * time.Millisecond)
	return ln.Addr().String(), func() {
		ln.Close()
		select {
		case <-errCh:
		case <-time.After(2 * time.Second):
			t.Error("engine did not stop within 2s")
		}
	}
}

// get issues an HTTP/1.1 GET request to url and returns status + body.
func get(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// ─── basic request/response ───────────────────────────────────────────────────

func TestEngine_BasicGET(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "hello reactor")
	})
	addr, stop := startEngine(t, handler, netengine.ReactorConfig{})
	defer stop()

	status, body := get(t, "http://"+addr+"/")
	if status != http.StatusOK {
		t.Errorf("expected 200, got %d", status)
	}
	if body != "hello reactor" {
		t.Errorf("expected body %q, got %q", "hello reactor", body)
	}
}

func TestEngine_POSTWithBody(t *testing.T) {
	var received string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		received = string(b)
		w.WriteHeader(http.StatusCreated)
	})
	addr, stop := startEngine(t, handler, netengine.ReactorConfig{})
	defer stop()

	resp, err := http.Post("http://"+addr+"/items", "application/json", strings.NewReader(`{"x":1}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected 201, got %d", resp.StatusCode)
	}
	if received != `{"x":1}` {
		t.Errorf("body not received correctly: %q", received)
	}
}

func TestEngine_404(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	addr, stop := startEngine(t, handler, netengine.ReactorConfig{})
	defer stop()

	status, _ := get(t, "http://"+addr+"/not-found")
	if status != http.StatusNotFound {
		t.Errorf("expected 404, got %d", status)
	}
}

// ─── keep-alive / pipelining ─────────────────────────────────────────────────

func TestEngine_KeepAlive_MultipleRequestsSameConn(t *testing.T) {
	var count int64
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt64(&count, 1)
		fmt.Fprintf(w, "req-%d", n)
	})
	addr, stop := startEngine(t, handler, netengine.ReactorConfig{})
	defer stop()

	// A persistent http.Client reuses the underlying TCP connection.
	client := &http.Client{Transport: &http.Transport{
		DisableKeepAlives: false,
		MaxIdleConns:      1,
	}}
	const N = 5
	for i := 0; i < N; i++ {
		resp, err := client.Get("http://" + addr + "/")
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		want := fmt.Sprintf("req-%d", i+1)
		if string(body) != want {
			t.Errorf("request %d: expected %q, got %q", i, want, string(body))
		}
	}
}

// ─── concurrency ──────────────────────────────────────────────────────────────

func TestEngine_ConcurrentRequests(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	})
	addr, stop := startEngine(t, handler, netengine.ReactorConfig{
		NumLoops:       2,
		WorkerPoolSize: 8,
	})
	defer stop()

	const goroutines = 20
	const reqsPerGoroutine = 10
	var wg sync.WaitGroup
	var failed int64
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < reqsPerGoroutine; j++ {
				resp, err := http.Get("http://" + addr + "/")
				if err != nil {
					if !strings.Contains(err.Error(), "503") {
						atomic.AddInt64(&failed, 1)
					}
					continue
				}
				defer resp.Body.Close()
				if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusServiceUnavailable {
					atomic.AddInt64(&failed, 1)
				}
				io.ReadAll(resp.Body)
			}
		}()
	}
	wg.Wait()
	if failed > 0 {
		t.Errorf("%d requests failed with unexpected errors", failed)
	}
}

// ─── ActiveConns ──────────────────────────────────────────────────────────────

func TestEngine_ActiveConns(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	})
	engine, err := netengine.New(handler, netengine.ReactorConfig{})
	if err != nil {
		t.Skipf("not supported: %v", err)
	}
	_ = engine
	if engine.ActiveConns() != 0 {
		t.Errorf("expected 0 active conns before serving, got %d", engine.ActiveConns())
	}
}

// ─── New() on unsupported platform returns error ─────────────────────────────

// This test documents the expected fallback behaviour; it passes trivially on
// supported platforms (New succeeds) and is meaningful on unsupported ones.
func TestNew_UnsupportedPlatform_NosPanic(t *testing.T) {
	// Simply verify that New does not panic regardless of the result.
	_, _ = netengine.New(http.DefaultServeMux, netengine.ReactorConfig{})
}

// ─── NumLoops / WorkerPoolSize accessors ──────────────────────────────────────

func TestEngine_Accessors(t *testing.T) {
	e, err := netengine.New(http.DefaultServeMux, netengine.ReactorConfig{
		NumLoops:       3,
		WorkerPoolSize: 7,
	})
	if err != nil {
		t.Skipf("not supported: %v", err)
	}
	if e.NumLoops() != 3 {
		t.Errorf("NumLoops: expected 3, got %d", e.NumLoops())
	}
	if e.WorkerPoolSize() != 7 {
		t.Errorf("WorkerPoolSize: expected 7, got %d", e.WorkerPoolSize())
	}
	e.Close()
}

// ─── HEAD request ─────────────────────────────────────────────────────────────

// TestEngine_HEADRequest verifies that HEAD responses carry the correct headers
// (including Content-Length that reflects the GET body size) but an empty body,
// as required by RFC 7231 §4.3.2.
func TestEngine_HEADRequest(t *testing.T) {
	const payload = `{"hello":"world"}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Custom", "test-val")
		fmt.Fprint(w, payload)
	})
	addr, stop := startEngine(t, handler, netengine.ReactorConfig{})
	defer stop()

	req, err := http.NewRequest(http.MethodHead, "http://"+addr+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := testHTTPClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: want 200, got %d", resp.StatusCode)
	}
	// Body must be empty for HEAD.
	b, _ := io.ReadAll(resp.Body)
	if len(b) != 0 {
		t.Errorf("HEAD body must be empty, got %q", b)
	}
	// Content-Length must still reflect the body size a GET would return.
	if cl := resp.ContentLength; cl != int64(len(payload)) {
		t.Errorf("Content-Length: want %d, got %d", len(payload), cl)
	}
	if resp.Header.Get("X-Custom") != "test-val" {
		t.Errorf("X-Custom header missing or wrong")
	}
}

// ─── ListenReusePort ──────────────────────────────────────────────────────────

// TestListenReusePort_Basic checks that ListenReusePort creates a working listener.
func TestListenReusePort_Basic(t *testing.T) {
	ln, err := netengine.ListenReusePort("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("SO_REUSEPORT not supported on this platform: %v", err)
	}
	defer ln.Close()
	if ln.Addr() == nil {
		t.Fatal("expected non-nil Addr")
	}
}

// TestListenReusePort_SamePort verifies that two listeners can bind the same
// address simultaneously — the defining property of SO_REUSEPORT.
func TestListenReusePort_SamePort(t *testing.T) {
	ln1, err := netengine.ListenReusePort("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("SO_REUSEPORT not supported on this platform: %v", err)
	}
	defer ln1.Close()

	// A second listener on the exact same address must succeed.
	ln2, err := netengine.ListenReusePort("tcp", ln1.Addr().String())
	if err != nil {
		t.Fatalf("second ListenReusePort on %s failed: %v", ln1.Addr(), err)
	}
	defer ln2.Close()
}

// TestListenReusePort_WithEngine verifies that an Engine started on a
// SO_REUSEPORT listener correctly serves HTTP requests.
func TestListenReusePort_WithEngine(t *testing.T) {
	ln, err := netengine.ListenReusePort("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("SO_REUSEPORT not supported on this platform: %v", err)
	}

	engine, err := netengine.New(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "reuseport-ok")
		}),
		netengine.ReactorConfig{},
	)
	if err != nil {
		ln.Close()
		t.Skipf("netengine not supported: %v", err)
	}

	errCh := make(chan error, 1)
	go func() { errCh <- engine.Serve(ln) }()
	time.Sleep(10 * time.Millisecond)

	addr := ln.Addr().String()
	defer func() {
		ln.Close()
		select {
		case <-errCh:
		case <-time.After(2 * time.Second):
			t.Error("engine did not stop within 2s")
		}
	}()

	status, body := get(t, "http://"+addr+"/")
	if status != http.StatusOK || body != "reuseport-ok" {
		t.Errorf("want 200 reuseport-ok, got %d %q", status, body)
	}
}

// TestListenReusePort_InvalidNetwork checks that an unsupported network string
// returns an error immediately rather than panicking.
func TestListenReusePort_InvalidNetwork(t *testing.T) {
	_, err := netengine.ListenReusePort("udp", "127.0.0.1:0")
	if err == nil {
		t.Error("expected error for unsupported network 'udp', got nil")
	}
}

// ─── Edge case: abnormal disconnection ───────────────────────────────────────

// TestEngine_AbnormalDisconnect_ImmediateClose verifies that the engine remains
// healthy when a client opens a connection and immediately closes it (TCP RST).
// After such connections the engine must still serve normal requests.
func TestEngine_AbnormalDisconnect_ImmediateClose(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "alive")
	})
	addr, stop := startEngine(t, handler, netengine.ReactorConfig{})
	defer stop()

	// Open and immediately close several connections without sending any data.
	const abortCount = 10
	for i := 0; i < abortCount; i++ {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatalf("dial %d: %v", i, err)
		}
		conn.Close() // close without sending anything → EOF/RST on engine side
	}

	// Give the engine a moment to process the half-open connections.
	time.Sleep(20 * time.Millisecond)

	// Engine must still serve normal requests after the disconnects.
	for i := 0; i < 3; i++ {
		status, body := get(t, "http://"+addr+"/")
		if status != http.StatusOK || body != "alive" {
			t.Errorf("post-disconnect request %d: want 200 alive, got %d %q", i, status, body)
		}
	}
}

// TestEngine_AbnormalDisconnect_DuringRead verifies that the engine handles a
// client that sends a partial HTTP request header and then abruptly closes.
// The engine must not crash and must continue serving other clients.
func TestEngine_AbnormalDisconnect_DuringRead(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	})
	addr, stop := startEngine(t, handler, netengine.ReactorConfig{})
	defer stop()

	// Send a partial HTTP request header and close mid-request.
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	conn.Write([]byte("GET / HTTP/1.1\r\nHost: ")) // incomplete header
	conn.Close()

	time.Sleep(20 * time.Millisecond)

	// Engine must still serve a full request.
	status, body := get(t, "http://"+addr+"/")
	if status != http.StatusOK || body != "ok" {
		t.Errorf("post-partial-request: want 200 ok, got %d %q", status, body)
	}
}

// ─── Edge case: burst connections ────────────────────────────────────────────

// TestEngine_BurstConnections verifies that a sudden spike of concurrent
// connections all eventually get a response without the engine panicking,
// deadlocking, or returning wrong results.
func TestEngine_BurstConnections(t *testing.T) {
	var seq int64
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt64(&seq, 1)
		fmt.Fprintf(w, "%d", n)
	})
	addr, stop := startEngine(t, handler, netengine.ReactorConfig{
		NumLoops:          2,
		WorkerPoolSize:    16,
		ConnChannelBuffer: 256,
	})
	defer stop()

	// Burst: 100 goroutines each making 5 sequential requests = 500 total.
	const goroutines = 100
	const perGoroutine = 5
	var wg sync.WaitGroup
	var failed int64
	client := &http.Client{Transport: &http.Transport{
		MaxIdleConnsPerHost: goroutines,
		DisableKeepAlives:   false,
	}}
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				resp, err := client.Get("http://" + addr + "/")
				if err != nil {
					// 503 is acceptable under pool saturation; anything else is a failure.
					if !strings.Contains(err.Error(), "503") {
						atomic.AddInt64(&failed, 1)
					}
					continue
				}
				if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusServiceUnavailable {
					atomic.AddInt64(&failed, 1)
				}
				io.ReadAll(resp.Body) //nolint:errcheck
				resp.Body.Close()
			}
		}()
	}
	wg.Wait()
	if failed > 0 {
		t.Errorf("%d requests failed with unexpected errors", failed)
	}
}

// ─── Listen with options ──────────────────────────────────────────────────────

// TestListenWithOptions_ReusePort verifies that Listen with ReusePort:true
// produces the same behaviour as ListenReusePort.
func TestListenWithOptions_ReusePort(t *testing.T) {
	ln, err := netengine.Listen("tcp", "127.0.0.1:0", netengine.ListenOptions{ReusePort: true})
	if err != nil {
		t.Skipf("SO_REUSEPORT not supported: %v", err)
	}
	defer ln.Close()

	// Second listener on the same port must also succeed.
	ln2, err := netengine.Listen("tcp", ln.Addr().String(), netengine.ListenOptions{ReusePort: true})
	if err != nil {
		t.Fatalf("second Listen (ReusePort) on %s: %v", ln.Addr(), err)
	}
	ln2.Close()
}

// TestListenWithOptions_FastOpen verifies that Listen with FastOpen:true
// either succeeds (kernel supports TFO) or returns a clear error (not a panic).
func TestListenWithOptions_FastOpen(t *testing.T) {
	ln, err := netengine.Listen("tcp", "127.0.0.1:0", netengine.ListenOptions{FastOpen: true})
	if err != nil {
		// TFO may be disabled by kernel config; accept the error gracefully.
		t.Logf("TCP_FASTOPEN not available (acceptable): %v", err)
		return
	}
	defer ln.Close()
	if ln.Addr() == nil {
		t.Fatal("expected non-nil Addr")
	}
}

// TestListenWithOptions_ReusePortAndFastOpen exercises the combination of both
// socket options at once.  At minimum, neither should panic.
func TestListenWithOptions_ReusePortAndFastOpen(t *testing.T) {
	ln, err := netengine.Listen("tcp", "127.0.0.1:0", netengine.ListenOptions{
		ReusePort: true,
		FastOpen:  true,
	})
	if err != nil {
		t.Logf("ReusePort+FastOpen not available (acceptable): %v", err)
		return
	}
	defer ln.Close()

	// Verify the listener actually serves requests.
	engine, err := netengine.New(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "tfo-ok")
		}),
		netengine.ReactorConfig{},
	)
	if err != nil {
		t.Skipf("netengine not supported: %v", err)
	}

	errCh := make(chan error, 1)
	go func() { errCh <- engine.Serve(ln) }()
	time.Sleep(10 * time.Millisecond)

	addr := ln.Addr().String()
	defer func() {
		ln.Close()
		select {
		case <-errCh:
		case <-time.After(2 * time.Second):
			t.Error("engine did not stop")
		}
	}()

	status, body := get(t, "http://"+addr+"/")
	if status != http.StatusOK || body != "tfo-ok" {
		t.Errorf("want 200 tfo-ok, got %d %q", status, body)
	}
}

// TestListenWithOptions_InvalidNetwork verifies Listen rejects non-TCP networks.
func TestListenWithOptions_InvalidNetwork(t *testing.T) {
	_, err := netengine.Listen("udp", "127.0.0.1:0", netengine.ListenOptions{})
	if err == nil {
		t.Error("expected error for unsupported network 'udp', got nil")
	}
}

// ─── Clean shutdown (no spurious error logs) ──────────────────────────────────

// TestEngine_CleanShutdown_NoErrorLog verifies that stopping the engine via
// listener close does not produce any ERROR-level log messages.
// Before the fix, poller.close() unblocked poller.wait() via EBADF, causing
// run() to log "poller.wait error" even during normal shutdown.
func TestEngine_CleanShutdown_NoErrorLog(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError}))

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	})
	engine, err := netengine.New(handler, netengine.ReactorConfig{Logger: logger})
	if err != nil {
		t.Skipf("netengine not supported: %v", err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	errCh := make(chan error, 1)
	go func() { errCh <- engine.Serve(ln) }()
	time.Sleep(10 * time.Millisecond)

	// Make a successful request, then close the listener to trigger shutdown.
	get(t, "http://"+ln.Addr().String()+"/")
	ln.Close()

	select {
	case <-errCh:
	case <-time.After(2 * time.Second):
		t.Fatal("engine did not stop within 2s")
	}

	if out := buf.String(); out != "" {
		t.Errorf("unexpected ERROR log during clean shutdown: %s", out)
	}
}

// TestEngine_CleanShutdown_AddChDrained verifies that connections queued in
// the addCh channel at shutdown time are properly closed and activeConns
// returns to zero — not stuck at a positive value.
func TestEngine_CleanShutdown_AddChDrained(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	})
	engine, err := netengine.New(handler, netengine.ReactorConfig{
		NumLoops:          1,
		ConnChannelBuffer: 64,
	})
	if err != nil {
		t.Skipf("netengine not supported: %v", err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	errCh := make(chan error, 1)
	go func() { errCh <- engine.Serve(ln) }()
	time.Sleep(10 * time.Millisecond)

	// Flood connections then immediately close the listener.
	// Some of those connections will land in addCh before the event loop
	// drains them; close() must handle them.
	const flood = 20
	for i := 0; i < flood; i++ {
		c, dialErr := net.Dial("tcp", ln.Addr().String())
		if dialErr == nil {
			c.Close()
		}
	}
	ln.Close()

	select {
	case <-errCh:
	case <-time.After(2 * time.Second):
		t.Fatal("engine did not stop within 2s")
	}

	// After shutdown, activeConns must be zero.
	if n := engine.ActiveConns(); n != 0 {
		t.Errorf("activeConns after shutdown: got %d, want 0", n)
	}
}
