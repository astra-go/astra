// Package netengine_test — end-to-end benchmarks for the Reactor HTTP engine.
//
// Unlike the white-box worker_pool_bench_test.go, these benchmarks use real
// TCP connections so they capture the complete path:
//
//	net.Dial → TCP → reactor accept → event loop → worker → handler → response
//
// The benchmarks are deliberately designed to test two distinct scenarios:
//   - Single-connection throughput (keep-alive reuse, hot path)
//   - Per-request latency (cold connection, measures accept + poller overhead)
//
// Run:
//
//	go test -bench=BenchmarkReactor -benchmem -count=3 -benchtime=3s ./netengine/
package netengine_test

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"

	"github.com/astra-go/astra/netengine"
)

// reactorBenchSink prevents dead-code elimination.
var reactorBenchSink any

// startBenchReactor starts a Reactor engine with the given handler on a random
// local port. Returns the listener address; the caller must close the listener
// to stop the engine.
//
// ReactorConfig is tuned for benchmarks:
//   - NumLoops = 1   (minimize scheduling noise)
//   - No read/write timeouts (remove I/O deadline overhead)
func startBenchReactor(b *testing.B, handler http.Handler) string {
	b.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatal("listen:", err)
	}

	engine, err := netengine.New(handler, netengine.ReactorConfig{
		NumLoops:       1,
		WorkerPoolSize: 16,
		ReadBufferSize: 16 * 1024,
		// Disable timeouts so deadline syscalls do not skew latency numbers.
		ReadTimeout:  0,
		WriteTimeout: 0,
	})
	if err != nil {
		ln.Close()
		b.Skip("netengine not available on this platform:", err)
	}

	go engine.Serve(ln) //nolint:errcheck

	b.Cleanup(func() { ln.Close() })
	return ln.Addr().String()
}

// pingHandler is a minimal HTTP/1.1 handler that returns a 3-byte body.
// It is used as the baseline for all reactor benchmarks.
var pingHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok\n")) //nolint:errcheck
})

// BenchmarkReactor_HTTP_Keepalive benchmarks the steady-state hot path:
// a persistent (keep-alive) TCP connection issuing sequential GET requests.
//
// The client side uses raw TCP I/O (conn.Write + manual response drain) instead
// of fmt.Fprint + http.ReadResponse.  This eliminates ~12 client-side allocs and
// ~15 KiB per iteration that http.ReadResponse allocates for *http.Response,
// http.Header, body readers, etc., so the numbers reflect the actual server cost:
//
//   - poller.wait event delivery
//   - worker pool handoff
//   - http.ReadRequest + handler.ServeHTTP + response flush
func BenchmarkReactor_HTTP_Keepalive(b *testing.B) {
	addr := startBenchReactor(b, pingHandler)

	// Establish one persistent connection for the entire benchmark.
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		b.Fatal("dial:", err)
	}
	defer conn.Close()

	// Pre-allocate the request as []byte once — no alloc on each iteration.
	reqBytes := []byte("GET / HTTP/1.1\r\nHost: bench\r\nConnection: keep-alive\r\n\r\n")

	// bodyBuf is pre-allocated to drain response bodies without heap allocation.
	bodyBuf := make([]byte, 512)

	// drainResponse reads and discards one HTTP/1.1 response from br.
	// It avoids http.ReadResponse (which allocates *http.Response, Header map,
	// body reader, etc.) by reading the wire bytes directly.
	br := bufio.NewReaderSize(conn, 4096)
	drainResponse := func() {
		// Read status line.
		if _, err := br.ReadString('\n'); err != nil {
			b.Fatal("read status:", err)
		}
		// Read headers until blank line; extract Content-Length.
		var bodyLen int
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				b.Fatal("read header:", err)
			}
			if line == "\r\n" {
				break
			}
			// Parse Content-Length so we can drain the body exactly.
			if len(line) > 16 && line[:15] == "Content-Length:" {
				for i := 15; i < len(line); i++ {
					c := line[i]
					if c >= '0' && c <= '9' {
						bodyLen = bodyLen*10 + int(c-'0')
					}
				}
			}
		}
		// Drain body bytes using the pre-allocated buffer.
		for bodyLen > 0 {
			buf := bodyBuf
			if len(buf) > bodyLen {
				buf = buf[:bodyLen]
			}
			n, err := io.ReadFull(br, buf)
			bodyLen -= n
			if err != nil {
				b.Fatal("read body:", err)
			}
		}
	}

	// Warm-up: one round-trip before starting the timer.
	conn.Write(reqBytes) //nolint:errcheck
	drainResponse()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn.Write(reqBytes) //nolint:errcheck
		drainResponse()
	}
}

// BenchmarkReactor_HTTP_NewConn benchmarks connection setup overhead:
// each iteration opens a new TCP connection, sends one request, and closes.
//
// This is the cold path corresponding to short-lived API clients or
// clients that do not support keep-alive.  It measures:
//   - TCP 3-way handshake (loopback, ~10 µs)
//   - Reactor accept + event-loop registration
//   - HTTP round-trip
//   - Connection teardown
func BenchmarkReactor_HTTP_NewConn(b *testing.B) {
	addr := startBenchReactor(b, pingHandler)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			b.Fatal("dial:", err)
		}

		fmt.Fprint(conn, "GET / HTTP/1.1\r\nHost: bench\r\nConnection: close\r\n\r\n") //nolint:errcheck
		br := bufio.NewReader(conn)
		resp, err := http.ReadResponse(br, nil)
		if err != nil {
			conn.Close()
			b.Fatal("read response:", err)
		}
		io.Copy(io.Discard, resp.Body) //nolint:errcheck
		resp.Body.Close()
		conn.Close()

		reactorBenchSink = resp.StatusCode
	}
}

// BenchmarkReactor_HTTP_Parallel measures throughput under concurrent load.
// b.RunParallel spawns GOMAXPROCS goroutines each maintaining its own
// persistent connection, modelling a realistic connection-pool client.
func BenchmarkReactor_HTTP_Parallel(b *testing.B) {
	addr := startBenchReactor(b, pingHandler)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			b.Error("dial:", err)
			return
		}
		defer conn.Close()

		br := bufio.NewReader(conn)
		req := "GET / HTTP/1.1\r\nHost: bench\r\nConnection: keep-alive\r\n\r\n"

		for pb.Next() {
			fmt.Fprint(conn, req) //nolint:errcheck
			resp, err := http.ReadResponse(br, nil)
			if err != nil {
				b.Error("read response:", err)
				return
			}
			io.Copy(io.Discard, resp.Body) //nolint:errcheck
			resp.Body.Close()
			reactorBenchSink = resp.StatusCode
		}
	})
}
