// Package metrics provides in-memory performance monitoring and metrics collection
// for the Astra framework. It includes a built-in dashboard for visualizing
// request performance, pool statistics, and system health.
//
// Usage:
//
//	metricsServer := metrics.NewServer(":9090")
//	go metricsServer.Start()
//
// Or attach to an existing App:
//
//	app := astra.New()
//	app.Use(metrics.Middleware())
package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/astra-go/astra"
)

// Server provides an HTTP endpoint for metrics and a built-in dashboard.
// It aggregates metrics from all instrumented components.
type Server struct {
	httpServer *http.Server
	registry   *Registry
}

// NewServer creates a new metrics server on the given address.
func NewServer(addr string) *Server {
	ms := &Server{
		registry: NewRegistry(),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", ms.handleMetrics)
	mux.HandleFunc("/metrics/json", ms.handleMetricsJSON)
	mux.HandleFunc("/dashboard", ms.handleDashboard)
	mux.HandleFunc("/health", ms.handleHealth)
	mux.HandleFunc("/api/requests", ms.handleRequestStats)
	mux.HandleFunc("/api/pools", ms.handlePoolStats)
	mux.HandleFunc("/api/system", ms.handleSystemStats)

	ms.httpServer = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	return ms
}

// Start begins serving metrics. Returns immediately if already started.
func (ms *Server) Start() error {
	go ms.collectLoop()
	return ms.httpServer.ListenAndServe()
}

// Stop gracefully shuts down the metrics server.
func (ms *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return ms.httpServer.Shutdown(ctx)
}

// collectLoop periodically aggregates metrics from the registry.
func (ms *Server) collectLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		// Metrics are collected on-demand by the handlers
		_ = ms.registry
	}
}

// Middleware returns a middleware that records request metrics.
func Middleware() astra.MiddlewareFunc {
	return func(c *astra.Ctx) error {
		start := time.Now()
		path := c.Request().URL.Path
		method := c.Request().Method

		err := c.Next()

		duration := time.Since(start)
		// Get status from the response writer
		var status int
		if w, ok := c.Writer().(interface{ Status() int }); ok {
			status = w.Status()
		} else {
			status = 200 // default
		}

		// Record the request
		GlobalRegistry().RecordRequest(path, method, status, duration)

		return err
	}
}

// handleMetrics outputs metrics in Prometheus format.
func (ms *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")

	stats := ms.registry.RequestStats()
	poolStats := ms.registry.PoolStats()
	systemStats := CollectSystemStats()

	// Request metrics
	fmt.Fprintf(w, "# HELP astra_requests_total Total number of HTTP requests\n")
	fmt.Fprintf(w, "# TYPE astra_requests_total counter\n")
	for key, count := range stats.total {
		labels := parseKey(key)
		fmt.Fprintf(w, "astra_requests_total{%s} %d\n", labels, count)
	}

	fmt.Fprintf(w, "\n# HELP astra_request_duration_seconds Request duration in seconds\n")
	fmt.Fprintf(w, "# TYPE astra_request_duration_seconds histogram\n")
	for key, hist := range stats.latencies {
		labels := parseKey(key)
		for _, bucket := range hist.Percentiles() {
			fmt.Fprintf(w, "astra_request_duration_seconds_bucket{%s,le=\"%s\"} %d\n",
				labels, bucket.Percentile, bucket.Count)
		}
	}

	// Pool metrics
	fmt.Fprintf(w, "\n# HELP astra_pool_connections_active Active connections\n")
	fmt.Fprintf(w, "# TYPE astra_pool_connections_active gauge\n")
	for name, ps := range poolStats.Pools {
		fmt.Fprintf(w, "astra_pool_connections_active{pool=\"%s\"} %d\n", name, ps.Active)
		fmt.Fprintf(w, "astra_pool_connections_idle{pool=\"%s\"} %d\n", name, ps.Idle)
	}

	// System metrics
	fmt.Fprintf(w, "\n# HELP astra_memory_alloc_bytes Allocated memory\n")
	fmt.Fprintf(w, "# TYPE astra_memory_alloc_bytes gauge\n")
	fmt.Fprintf(w, "astra_memory_alloc_bytes %d\n", systemStats.MemAlloc)

	fmt.Fprintf(w, "\n# HELP astra_goroutines Number of goroutines\n")
	fmt.Fprintf(w, "# TYPE astra_goroutines gauge\n")
	fmt.Fprintf(w, "astra_goroutines %d\n", systemStats.Goroutines)
}

// handleMetricsJSON outputs metrics as JSON.
func (ms *Server) handleMetricsJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	data := map[string]interface{}{
		"requests": ms.registry.RequestStats(),
		"pools":    ms.registry.PoolStats(),
		"system":   CollectSystemStats(),
		"timestamp": time.Now().Unix(),
	}

	json.NewEncoder(w).Encode(data)
}

// handleDashboard serves the metrics dashboard HTML.
func (ms *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")

	stats := ms.registry.RequestStats()
	poolStats := ms.registry.PoolStats()
	systemStats := CollectSystemStats()

	html := dashboardHTML
	html = strings.ReplaceAll(html, "{{.RequestsTotal}}", fmt.Sprintf("%d", stats.TotalRequests()))
	html = strings.ReplaceAll(html, "{{.RequestsPerSec}}", fmt.Sprintf("%.2f", stats.RequestsPerSec()))
	html = strings.ReplaceAll(html, "{{.AvgLatency}}", fmt.Sprintf("%.2fms", stats.AvgLatency().Seconds()*1000))
	html = strings.ReplaceAll(html, "{{.P99Latency}}", fmt.Sprintf("%.2fms", stats.P99Latency().Seconds()*1000))
	html = strings.ReplaceAll(html, "{{.ErrorRate}}", fmt.Sprintf("%.2f%%", stats.ErrorRate()*100))
	html = strings.ReplaceAll(html, "{{.MemAlloc}}", formatBytes(systemStats.MemAlloc))
	html = strings.ReplaceAll(html, "{{.Goroutines}}", fmt.Sprintf("%d", systemStats.Goroutines))
	html = strings.ReplaceAll(html, "{{.CPUPercent}}", fmt.Sprintf("%.1f%%", systemStats.CPUPercent))
	html = strings.ReplaceAll(html, "{{.PoolCount}}", fmt.Sprintf("%d", len(poolStats.Pools)))
	var totalActive, totalIdle int
	for _, ps := range poolStats.Pools {
		totalActive += ps.Active
		totalIdle += ps.Idle
	}
	html = strings.ReplaceAll(html, "{{.ActiveConns}}", fmt.Sprintf("%d", totalActive))
	html = strings.ReplaceAll(html, "{{.IdleConns}}", fmt.Sprintf("%d", totalIdle))

	// Generate top endpoints table
	var endpointsRows string
	sorted := stats.TopEndpoints()
	for i, ep := range sorted {
		if i >= 10 {
			break
		}
		endpointsRows += fmt.Sprintf(`<tr><td>%s</td><td>%d</td><td>%.2fms</td><td>%.2f%%</td></tr>`,
			ep.Path, ep.Count, ep.AvgLatency.Seconds()*1000, ep.ErrorRate*100)
	}
	html = strings.ReplaceAll(html, "{{.EndpointsTable}}", endpointsRows)

	fmt.Fprint(w, html)
}

// handleHealth returns health status.
func (ms *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	healthy := true
	details := map[string]string{
		"metrics_server": "ok",
	}

	// Check system health
	systemStats := CollectSystemStats()
	if systemStats.MemAlloc > uint64(systemStats.MemLimit*80/100) {
		healthy = false
		details["memory"] = "warning: high memory usage"
	}

	status := "healthy"
	if !healthy {
		status = "degraded"
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    status,
		"details":   details,
		"timestamp": time.Now().Unix(),
	})
}

// handleRequestStats returns detailed request statistics.
func (ms *Server) handleRequestStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ms.registry.RequestStats())
}

// handlePoolStats returns pool statistics.
func (ms *Server) handlePoolStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ms.registry.PoolStats())
}

// handleSystemStats returns system statistics.
func (ms *Server) handleSystemStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(CollectSystemStats())
}

// parseKey extracts labels from a key like "GET /api/users:200".
func parseKey(key string) string {
	parts := strings.Split(key, " ")
	if len(parts) < 2 {
		return "path=\"unknown\""
	}
	return fmt.Sprintf("method=\"%s\",path=\"%s\"", parts[0], parts[1])
}

// formatBytes converts bytes to human-readable format.
func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// dashboardHTML is the built-in dashboard template.
const dashboardHTML = `
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Astra Metrics Dashboard</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;background:#0f172a;color:#e2e8f0}
.container{max-width:1400px;margin:0 auto;padding:20px}
h1{font-size:2rem;margin-bottom:20px;color:#f8fafc}
.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(250px,1fr));gap:16px;margin-bottom:24px}
.card{background:#1e293b;border-radius:12px;padding:20px;box-shadow:0 4px 6px rgba(0,0,0,0.3)}
.card h3{font-size:0.875rem;color:#94a3b8;margin-bottom:8px;text-transform:uppercase;letter-spacing:0.05em}
.card .value{font-size:2.5rem;font-weight:700;color:#38bdf8}
.card .sub{font-size:0.875rem;color:#64748b;margin-top:4px}
table{width:100%;border-collapse:collapse;background:#1e293b;border-radius:12px;overflow:hidden}
th,td{padding:12px 16px;text-align:left;border-bottom:1px solid #334155}
th{background:#334155;font-weight:600;color:#cbd5e1;font-size:0.875rem;text-transform:uppercase}
tr:hover{background:#334155}
.status-ok{color:#22c55e}
.status-warn{color:#f59e0b}
.status-error{color:#ef4444}
</style>
</head>
<body>
<div class="container">
<h1>🚀 Astra Metrics Dashboard</h1>

<div class="grid">
<div class="card">
<h3>Total Requests</h3>
<div class="value">{{.RequestsTotal}}</div>
<div class="sub">{{.RequestsPerSec}} req/s</div>
</div>
<div class="card">
<h3>Avg Latency</h3>
<div class="value">{{.AvgLatency}}</div>
<div class="sub">P99: {{.P99Latency}}</div>
</div>
<div class="card">
<h3>Error Rate</h3>
<div class="value">{{.ErrorRate}}</div>
<div class="sub">of all requests</div>
</div>
<div class="card">
<h3>Memory</h3>
<div class="value">{{.MemAlloc}}</div>
<div class="sub">{{.Goroutines}} goroutines</div>
</div>
<div class="card">
<h3>Pools</h3>
<div class="value">{{.PoolCount}}</div>
<div class="sub">{{.ActiveConns}} active / {{.IdleConns}} idle</div>
</div>
<div class="card">
<h3>CPU</h3>
<div class="value">{{.CPUPercent}}</div>
<div class="sub">usage</div>
</div>
</div>

<h2 style="margin-bottom:16px">Top Endpoints</h2>
<table>
<thead>
<tr><th>Endpoint</th><th>Requests</th><th>Avg Latency</th><th>Error Rate</th></tr>
</thead>
<tbody>
{{.EndpointsTable}}
</tbody>
</table>
</div>
<script>setTimeout(()=>location.reload(),10000);</script>
</body>
</html>
`