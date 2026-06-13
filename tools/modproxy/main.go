// Package main implements a lightweight Go module proxy.
// It proxies requests to an upstream proxy (e.g. goproxy.cn) and caches
// responses on local disk for fast subsequent fetches.
// It also supports hosting private modules.
package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultUpstream   = "https://goproxy.cn"
	defaultCacheDir   = "./cache"
	defaultPrivateDir = "./private"
	defaultAddr       = ":8080"
	requestTimeout    = 60 * time.Second
)

// httpClient is configured with proper timeouts to prevent hanging.
var httpClient = &http.Client{
	Timeout: requestTimeout,
	Transport: &http.Transport{
		TLSHandshakeTimeout:   30 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	},
}

var (
	upstreamURL string
	cacheDir    string
	privateDir  string
	uploadToken string
)

func main() {
	upstreamURL = getEnv("PROXY_UPSTREAM", defaultUpstream)
	cacheDir = getEnv("PROXY_CACHE_DIR", defaultCacheDir)
	privateDir = getEnv("PROXY_PRIVATE_DIR", defaultPrivateDir)
	uploadToken = getEnv("PROXY_UPLOAD_TOKEN", "")
	addr := getEnv("PROXY_LISTEN_ADDR", defaultAddr)

	logger := slog.Default()

	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		logger.Error("failed to create cache dir", "err", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(privateDir, 0700); err != nil {
		logger.Error("failed to create private dir", "err", err)
		os.Exit(1)
	}

	// Register handlers on DefaultServeMux
	http.HandleFunc("/", handleRequest)

	logger.Info("Astra Module Proxy started",
		"listen_addr", addr,
		"upstream", upstreamURL,
		"cache_dir", cacheDir,
		"private_dir", privateDir,
		"upload_enabled", uploadToken != "")
	logger.Info("usage: go env -w GOPROXY=http://localhost"+addr+",direct")

	if err := http.ListenAndServe(addr, nil); err != nil {
		logger.Error("server failed", "err", err)
		os.Exit(1)
	}
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// handleRequest dispatches all requests:
//   - GET  /              → HTML welcome page
//   - GET  /health        → health check
//   - GET  /*/@v/list     → module version list
//   - GET  /*/@v/*.ext    → module version file (info/mod/zip)
//   - GET  /*/@latest     → latest version
//   - PUT  /*/@v/*.ext    → upload private module file
func handleRequest(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Exact matches
	switch path {
	case "/":
		handleRoot(w, r)
		return
	case "/health":
		handleHealth(w, r)
		return
	}

	// Parse module proxy protocol paths
	if strings.HasSuffix(path, "/@latest") {
		modulePath := strings.TrimSuffix(path, "/@latest")
		handleModuleProxy(w, r, modulePath, "/@latest")
		return
	}

	if idx := strings.Index(path, "/@v/"); idx != -1 {
		modulePath := path[:idx]
		suffix := path[idx:] // e.g. "/@v/list" or "/@v/v1.0.0.info"
		handleModuleProxy(w, r, modulePath, suffix)
		return
	}

	http.NotFound(w, r)
}

// handleModuleProxy handles GET (proxy/download) and PUT (upload) requests.
func handleModuleProxy(w http.ResponseWriter, r *http.Request, modulePath, suffix string) {
	switch r.Method {
	case http.MethodGet:
		handleGet(w, r, modulePath, suffix)
	case http.MethodPut:
		handlePut(w, r, modulePath, suffix)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleGet tries private dir → cache → upstream.
func handleGet(w http.ResponseWriter, r *http.Request, modulePath, suffix string) {
	upstreamPath := modulePath + suffix // e.g. /github.com/astra-go/astra/@v/list

	// 1. Try private dir first
	privateFile := filepath.Join(privateDir, upstreamPath)
	if data, err := os.ReadFile(privateFile); err == nil {
		slog.Info("private hit", "path", upstreamPath)
		ct := contentTypeFor(upstreamPath)
		w.Header().Set("Content-Type", ct)
		w.Write(data)
		return
	}

	// 2. Try cache
	cacheFile := filepath.Join(cacheDir, upstreamPath)
	if data, err := os.ReadFile(cacheFile); err == nil {
		slog.Info("cache hit", "path", upstreamPath)
		ct := contentTypeFor(upstreamPath)
		w.Header().Set("Content-Type", ct)
		w.Write(data)
		return
	}

	slog.Info("cache miss", "path", upstreamPath)

	// 3. Fetch from upstream
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()

	url := upstreamURL + upstreamPath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		slog.Error("create request failed", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if accept := r.Header.Get("Accept"); accept != "" {
		req.Header.Set("Accept", accept)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		slog.Error("upstream request failed", "err", err)
		http.Error(w, "upstream request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if resp.StatusCode != http.StatusOK {
		slog.Error("upstream returned error", "status", resp.StatusCode, "path", upstreamPath)
		http.Error(w, "upstream error", http.StatusBadGateway)
		return
	}

	// Read body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("read response body failed", "err", err)
		http.Error(w, "read response body failed", http.StatusInternalServerError)
		return
	}

	// Save to cache (best effort)
	if err := os.MkdirAll(filepath.Dir(cacheFile), 0700); err == nil {
		if err := os.WriteFile(cacheFile, body, 0644); err != nil {
			slog.Warn("failed to write cache", "file", cacheFile, "err", err)
		}
	}

	ct := contentTypeFor(upstreamPath)
	w.Header().Set("Content-Type", ct)
	w.Write(body)
}

// handlePut saves private module file to private dir.
func handlePut(w http.ResponseWriter, r *http.Request, modulePath, suffix string) {
	// Check upload token
	if uploadToken != "" {
		token := r.Header.Get("Authorization")
		if token != "Bearer "+uploadToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// Only allow PUT for specific file types
	if !strings.HasSuffix(suffix, ".info") &&
		!strings.HasSuffix(suffix, ".mod") &&
		!strings.HasSuffix(suffix, ".zip") &&
		suffix != "/@v/list" &&
		suffix != "/@latest" {
		http.Error(w, "can only upload .info, .mod, .zip, /@v/list, or /@latest", http.StatusBadRequest)
		return
	}

	// Read body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("read request body failed", "err", err)
		http.Error(w, "read request body failed", http.StatusInternalServerError)
		return
	}

	// Save to private dir
	privateFile := filepath.Join(privateDir, modulePath+suffix)
	if err := os.MkdirAll(filepath.Dir(privateFile), 0700); err != nil {
		slog.Error("create private dir failed", "err", err)
		http.Error(w, "create private dir failed", http.StatusInternalServerError)
		return
	}

	if err := os.WriteFile(privateFile, body, 0644); err != nil {
		slog.Error("write private file failed", "err", err)
		http.Error(w, "write private file failed", http.StatusInternalServerError)
		return
	}

	slog.Info("upload saved", "file", privateFile)
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte("OK"))
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	addr := getEnv("PROXY_LISTEN_ADDR", defaultAddr)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<h1>Astra Module Proxy</h1>
<p>This is a lightweight Go module proxy for the astra-go/astra monorepo.</p>
<ul>
  <li>Upstream: <a href="%s">%s</a></li>
  <li>Cache dir: <code>%s</code></li>
  <li>Private dir: <code>%s</code></li>
</ul>
<p>Usage:</p>
<pre>
go env -w GOPROXY=http://localhost%s,direct
go env -w GOPRIVATE=github.com/astra-go/astra
go env -w GONOSUMDB=github.com/astra-go/astra
</pre>
<p>See <a href="https://github.com/astra-go/astra/blob/main/docs/module-proxy-setup.md">docs/module-proxy-setup.md</a> for details.</p>
`, upstreamURL, upstreamURL, cacheDir, privateDir, addr)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

func contentTypeFor(path string) string {
	switch {
	case strings.HasSuffix(path, ".info"):
		return "application/json"
	case strings.HasSuffix(path, ".mod"):
		return "text/plain; charset=utf-8"
	case strings.HasSuffix(path, ".zip"):
		return "application/zip"
	case strings.HasSuffix(path, "/list"):
		return "text/plain; charset=utf-8"
	case strings.HasSuffix(path, "/@latest"):
		return "application/json"
	default:
		return "application/octet-stream"
	}
}
