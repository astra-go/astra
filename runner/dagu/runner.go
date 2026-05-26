// Package dagu provides a Runner backed by the Dagu DAG-based workflow
// orchestrator (https://github.com/dagu-org/dagu).
//
// # Architecture
//
// DaguRunner bridges Go functions and Dagu's scheduling engine:
//
//  1. Add/Every writes a DAG YAML file to Dagu's DAGs directory. The YAML
//     configures Dagu's own cron scheduler to fire at the given interval.
//  2. Each DAG step calls this service back via an HTTP POST to a lightweight
//     server started by Start().
//  3. The callback server executes the registered Go JobFunc.
//
// This gives you full Dagu features (Web UI, execution history, manual
// retrigger, concurrency policies, DAG dependencies) while keeping business
// logic in Go.
//
// # Prerequisites
//
//   - Dagu must be running and its DAGs directory must be writable by this process.
//   - This service must be reachable from Dagu at CallbackURL.
//
// # Usage
//
//	r, err := dagu.New(dagu.Config{
//	    BaseURL:     "http://localhost:8080",
//	    DAGsDir:     "/home/user/.config/dagu/dags",
//	    CallbackURL: "http://my-service:9000",
//	})
//	r.Add("cleanup", "0 2 * * *", func(ctx context.Context) error {
//	    return cleanupExpiredData(ctx)
//	})
//	r.Start(ctx)
//	defer r.Stop(context.Background())
package dagu

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

	"github.com/astra-go/astra/runner"
)

const (
	defaultCallbackPort = 9000
	defaultTimeout      = 5 * time.Minute
)

// Config configures the DaguRunner.
type Config struct {
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

func (c *Config) setDefaults() {
	if c.CallbackPort == 0 {
		c.CallbackPort = defaultCallbackPort
	}
	if c.Timeout == 0 {
		c.Timeout = defaultTimeout
	}
}

// Runner is a Runner backed by Dagu.
// All methods are safe for concurrent use.
type Runner struct {
	cfg      Config
	handlers sync.Map    // name → runner.JobFunc
	mux      *http.ServeMux
	mu       sync.Mutex // serialises Start/Stop
	httpSrv  *http.Server
}

// New creates a DaguRunner.
// BaseURL, DAGsDir, and CallbackURL are required.
func New(cfg Config) (*Runner, error) {
	cfg.setDefaults()
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("runner/dagu: BaseURL is required")
	}
	if cfg.DAGsDir == "" {
		return nil, fmt.Errorf("runner/dagu: DAGsDir is required")
	}
	if cfg.CallbackURL == "" {
		return nil, fmt.Errorf("runner/dagu: CallbackURL is required")
	}
	return &Runner{cfg: cfg, mux: http.NewServeMux()}, nil
}

// Add registers job and writes a Dagu DAG YAML file that invokes it.
//
// The YAML is written to {DAGsDir}/{name}.yaml. Dagu picks up new/changed
// files automatically (live reload). The DAG step calls:
//
//	POST {CallbackURL}/runner/execute/{name}
//
// Returns an error if a job with the same name is already registered.
func (r *Runner) Add(name, expr string, job runner.JobFunc) error {
	if _, loaded := r.handlers.LoadOrStore(name, job); loaded {
		return fmt.Errorf("runner/dagu: job %q already registered", name)
	}
	r.mux.HandleFunc("/runner/execute/"+name, r.makeHandler(name))
	return r.writeDAG(name, expr)
}

// Every schedules job at a fixed interval.
// Returns an error if a job with the same name is already registered.
func (r *Runner) Every(name string, d time.Duration, job runner.JobFunc) error {
	return r.Add(name, fmt.Sprintf("@every %s", d), job)
}

// Start launches the local HTTP callback server on CallbackPort. Non-blocking.
func (r *Runner) Start(_ context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.httpSrv != nil {
		return fmt.Errorf("runner/dagu: already started")
	}
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", r.cfg.CallbackPort),
		Handler: r.mux,
	}
	r.httpSrv = srv
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("runner/dagu: callback server error", "err", err)
		}
	}()
	slog.Info("runner/dagu: callback server started", "port", r.cfg.CallbackPort)
	return nil
}

// Stop gracefully shuts down the callback HTTP server.
// In-flight job callbacks are allowed to complete until ctx expires.
func (r *Runner) Stop(ctx context.Context) error {
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
func (r *Runner) Jobs() []runner.JobInfo {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.cfg.BaseURL+"/api/v1/dags", nil)
	if err != nil {
		slog.Warn("runner/dagu: Jobs() build request error", "err", err)
		return r.localJobs()
	}
	if r.cfg.Username != "" {
		req.SetBasicAuth(r.cfg.Username, r.cfg.Password)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Warn("runner/dagu: Jobs() HTTP error", "err", err)
		return r.localJobs()
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
		slog.Warn("runner/dagu: Jobs() decode error", "err", err)
		return r.localJobs()
	}

	out := make([]runner.JobInfo, 0, len(payload.DAGs))
	for _, d := range payload.DAGs {
		// Only include DAGs that were registered through this Runner instance.
		if _, ok := r.handlers.Load(d.DAG.Name); !ok {
			continue
		}
		info := runner.JobInfo{Name: d.DAG.Name}
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

// localJobs returns a name-only JobInfo list from the local handler registry.
// Used as a fallback when the Dagu API is unreachable.
func (r *Runner) localJobs() []runner.JobInfo {
	var out []runner.JobInfo
	r.handlers.Range(func(key, _ any) bool {
		out = append(out, runner.JobInfo{Name: key.(string)})
		return true
	})
	return out
}

// makeHandler returns an HTTP handler that invokes the named job.
func (r *Runner) makeHandler(name string) http.HandlerFunc {
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
		if err := v.(runner.JobFunc)(req.Context()); err != nil {
			slog.Error("runner/dagu: job error", "name", name, "err", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

// dagYAML is the Dagu DAG YAML template written for each registered job.
// The DAG configures Dagu's scheduler and delegates execution to the Go
// callback server via a curl step (compatible with all Dagu deployments).
var dagYAML = template.Must(template.New("dag").Parse(
	`# Generated by astra/runner/dagu — do not edit manually.
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
func (r *Runner) writeDAG(name, expr string) error {
	if err := os.MkdirAll(r.cfg.DAGsDir, 0o755); err != nil {
		return fmt.Errorf("runner/dagu: create DAGs dir: %w", err)
	}
	var buf bytes.Buffer
	if err := dagYAML.Execute(&buf, dagData{
		Name:        name,
		Expr:        expr,
		CallbackURL: r.cfg.CallbackURL,
		TimeoutSec:  int64(r.cfg.Timeout.Seconds()),
	}); err != nil {
		return fmt.Errorf("runner/dagu: render YAML for %q: %w", name, err)
	}
	path := filepath.Join(r.cfg.DAGsDir, name+".yaml")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("runner/dagu: write %s: %w", path, err)
	}
	slog.Info("runner/dagu: wrote DAG YAML", "path", path)
	return nil
}

// Verify Runner implements runner.Runner at compile time.
var _ runner.Runner = (*Runner)(nil)
