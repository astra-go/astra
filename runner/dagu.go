//go:build dagu
// +build dagu

package runner

// This file provides the dagu backend, enabled with build tag "dagu".

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"text/template"
	"time"
)

const (
	defaultDaguCallbackPort = 9000
	defaultDaguTimeout      = 5 * time.Minute
)

// DaguConfig configures the DaguRunner.
type DaguConfig struct {
	// BaseURL is the Dagu REST API base URL, e.g. "http://localhost:8080".
	// Used by Jobs() to query registered DAGs from Dagu.
	BaseURL string

	// DAGsDir is the path to Dagu's DAGs directory.
	// Add/Every writes YAML files here; Dagu hot-reloads them automatically.
	// Example: "/home/user/.config/dagu/dags"
	DAGsDir string

	// CallbackURL is the address at which this service is reachable from Dagu.
	// Dagu will POST to {CallbackURL}/runner/execute/{name} when a DAG fires.
	// Example: "http://my-service:9000"
	CallbackURL string

	// CallbackPort is the local port for the callback HTTP server. Default: 9000.
	CallbackPort int

	// Username and Password enable HTTP basic auth when calling the Dagu API.
	Username string
	Password string

	// Timeout is the per-job execution deadline passed to the DAG step.
	// Dagu's HTTP executor will abort the step if the callback exceeds this.
	// Default: 5 minutes.
	Timeout time.Duration
}

func (c *DaguConfig) setDaguDefaults() {
	if c.CallbackPort == 0 {
		c.CallbackPort = defaultDaguCallbackPort
	}
	if c.Timeout == 0 {
		c.Timeout = defaultDaguTimeout
	}
}

// DaguRunner is a Runner backed by Dagu.
// All methods are safe for concurrent use.
type DaguRunner struct {
	cfg      DaguConfig
	handlers sync.Map    // name → JobFunc
	mux      *http.ServeMux
	mu       sync.Mutex // serialises Start/Stop
	httpSrv  *http.Server
}

// NewDaguRunner creates a DaguRunner.
// BaseURL, DAGsDir, and CallbackURL are required.
func NewDaguRunner(cfg DaguConfig) (*DaguRunner, error) {
	cfg.setDaguDefaults()
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("runner: BaseURL is required")
	}
	if cfg.DAGsDir == "" {
		return nil, fmt.Errorf("runner: DAGsDir is required")
	}
	if cfg.CallbackURL == "" {
		return nil, fmt.Errorf("runner: CallbackURL is required")
	}
	return &DaguRunner{cfg: cfg, mux: http.NewServeMux()}, nil
}

// Add registers job and writes a Dagu DAG YAML file that invokes it.
//
// The YAML is written to {DAGsDir}/{name}.yaml. Dagu picks up new/changed
// files automatically (live reload). The DAG step calls:
//
//	POST {CallbackURL}/runner/execute/{name}
//
// Returns an error if a job with the same name is already registered.
func (r *DaguRunner) Add(name, expr string, job JobFunc) error {
	if _, loaded := r.handlers.LoadOrStore(name, job); loaded {
		return fmt.Errorf("runner: job %q already registered", name)
	}
	r.mux.HandleFunc("/runner/execute/"+name, r.makeDaguHandler(name))
	return r.writeDAG(name, expr)
}

// Every schedules job at a fixed interval.
// Returns an error if a job with the same name is already registered.
func (r *DaguRunner) Every(name string, d time.Duration, job JobFunc) error {
	return r.Add(name, fmt.Sprintf("@every %s", d), job)
}

// Start launches the local HTTP callback server on CallbackPort. Non-blocking.
func (r *DaguRunner) Start(_ context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.httpSrv != nil {
		return fmt.Errorf("runner: already started")
	}
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", r.cfg.CallbackPort),
		Handler: r.mux,
	}
	r.httpSrv = srv
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("runner: callback server error", "err", err)
		}
	}()
	slog.Info("runner: callback server started", "port", r.cfg.CallbackPort)
	return nil
}

// Stop gracefully shuts down the callback HTTP server.
// In-flight job callbacks are allowed to complete until ctx expires.
func (r *DaguRunner) Stop(ctx context.Context) error {
	r.mu.Lock()
	srv := r.httpSrv
	r.mu.Unlock()
	if srv == nil {
		return nil
	}
	return srv.Shutdown(ctx)
}

// Jobs queries the Dagu REST API and returns info for DAGs managed by this runner.
// Falls back to a local name-only list if the Dagu API is unreachable.
func (r *DaguRunner) Jobs() []JobInfo {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.cfg.BaseURL+"/api/v1/dags", nil)
	if err != nil {
		slog.Warn("runner: Jobs() build request error", "err", err)
		return r.localDaguJobs()
	}
	if r.cfg.Username != "" {
		req.SetBasicAuth(r.cfg.Username, r.cfg.Password)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Warn("runner: Jobs() HTTP error", "err", err)
		return r.localDaguJobs()
	}
	defer resp.Body.Close()

	// Decode the subset of Dagu's response we care about.
	var payload struct {
		DAGs []struct {
			DAG struct {
				Name     string `json:"Name"`
				Schedule []struct {
					Expression string `json:"Expression"`
				} `json:"Schedule"`
			} `json:"DAG"`
			Status struct {
				StartedAt string `json:"StartedAt"`
			} `json:"Status"`
		} `json:"DAGs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		slog.Warn("runner: Jobs() decode error", "err", err)
		return r.localDaguJobs()
	}

	out := make([]JobInfo, 0, len(payload.DAGs))
	for _, d := range payload.DAGs {
		// Only include DAGs that were registered through this Runner instance.
		if _, ok := r.handlers.Load(d.DAG.Name); !ok {
			continue
		}
		info := JobInfo{Name: d.DAG.Name}
		if len(d.DAG.Schedule) > 0 {
			info.Expr = d.DAG.Schedule[0].Expression
		}
		if t, err := time.Parse(time.RFC3339, d.Status.StartedAt); err == nil {
			info.Prev = t
		}
		out = append(out, info)
	}
	return out
}

// localDaguJobs returns a name-only JobInfo list from the local handler registry.
// Used as a fallback when the Dagu API is unreachable.
func (r *DaguRunner) localDaguJobs() []JobInfo {
	var out []JobInfo
	r.handlers.Range(func(key, _ any) bool {
		out = append(out, JobInfo{Name: key.(string)})
		return true
	})
	return out
}

// makeDaguHandler returns an HTTP handler that invokes the named job.
func (r *DaguRunner) makeDaguHandler(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		v, ok := r.handlers.Load(name)
		if !ok {
			http.Error(w, "job not found", http.StatusNotFound)
			return
		}
		if err := v.(JobFunc)(req.Context()); err != nil {
			slog.Error("runner: job error", "name", name, "err", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

// dagYAML is the Dagu DAG YAML template written for each registered job.
var dagYAML = template.Must(template.New("dag").Parse(
	`# Generated by astra/runner — do not edit manually.
name: {{.Name}}
schedule: "{{.Expr}}"
maxActiveRuns: 1
steps:
  - name: execute
    command: >-
      curl -sf
      -X POST
      -H "Content-Type: application/json"
      --max-time {{.TimeoutSec}}
      {{.CallbackURL}}/runner/execute/{{.Name}}
`))

type dagData struct {
	Name        string
	Expr        string
	CallbackURL string
	TimeoutSec  int64
}

// writeDAG renders and writes the DAG YAML to DAGsDir.
func (r *DaguRunner) writeDAG(name, expr string) error {
	if err := os.MkdirAll(r.cfg.DAGsDir, 0o755); err != nil {
		return fmt.Errorf("runner: create DAGs dir: %w", err)
	}
	var buf bytes.Buffer
	if err := dagYAML.Execute(&buf, dagData{
		Name:        name,
		Expr:        expr,
		CallbackURL: r.cfg.CallbackURL,
		TimeoutSec:  int64(r.cfg.Timeout.Seconds()),
	}); err != nil {
		return fmt.Errorf("runner: render YAML for %q: %w", name, err)
	}
	path := filepath.Join(r.cfg.DAGsDir, name+".yaml")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("runner: write %s: %w", path, err)
	}
	slog.Info("runner: wrote DAG YAML", "path", path)
	return nil
}

// Verify DaguRunner implements Runner at compile time.
var _ Runner = (*DaguRunner)(nil)
