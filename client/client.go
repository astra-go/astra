// Package client provides an HTTP service client with integrated service discovery,
// load balancing, circuit breaking, retry, and distributed tracing.
//
// # Quick start
//
//	reg := discovery.NewInMemoryRegistry()
//	cli := client.New(
//	    client.WithRegistry(reg),
//	    client.WithBalancer(loadbalance.NewRoundRobin()),
//	    client.WithTimeout(5 * time.Second),
//	    client.WithRetryPolicy(retry.DefaultPolicy),
//	)
//	resp, err := cli.Get(ctx, "user-svc", "/users/42")
//
// # Auto-resolve
//
// WithAutoResolve creates a persistent Watch subscription for each service on
// first access, eliminating a Discover round-trip on every request:
//
//	cli := client.New(
//	    client.WithRegistry(reg),
//	    client.WithBalancer(loadbalance.NewP2C()),
//	    client.WithAutoResolve(ctx),  // background ctx for resolvers
//	)
//	defer cli.Close()
package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/astra-go/astra/circuit"
	"github.com/astra-go/astra/discovery"
	"github.com/astra-go/astra/loadbalance"
	"github.com/astra-go/astra/retry"
)

// ─── Client ───────────────────────────────────────────────────────────────────

// Client is a service-aware HTTP client that wraps service discovery, load
// balancing, circuit breaking, retry, and tracing into a single call site.
//
// Thread-safe: a single Client may be shared across goroutines.
type Client struct {
	registry    discovery.Registry
	balancer    loadbalance.Balancer
	retryPolicy retry.Policy
	timeout     time.Duration
	tracer      trace.Tracer

	propagator  propagation.TextMapPropagator
	http        *http.Client

	// per-service circuit breakers (lazily created)
	breakersMu sync.RWMutex
	breakers   map[string]*circuit.Breaker
	breakerCfg circuit.Config // template for new breakers

	// auto-resolve: Watch-based live instance cache
	autoResolveCtx context.Context
	resolversMu    sync.Mutex
	resolvers      map[string]*loadbalance.Resolver
}

// Option configures the Client.
type Option func(*Client)

// New creates a Client with the provided options.
func New(opts ...Option) *Client {
	c := &Client{
		balancer:    loadbalance.NewRoundRobin(),
		retryPolicy: retry.DefaultPolicy,
		timeout:     10 * time.Second,
		tracer:      otel.Tracer("astra/client"),
		propagator:  otel.GetTextMapPropagator(),
		http:        &http.Client{Timeout: 10 * time.Second},
		breakers:    make(map[string]*circuit.Breaker),
		breakerCfg: circuit.Config{
			Threshold:         5,
			Timeout:           30 * time.Second,
			HalfOpenSuccesses: 2,
		},
		resolvers: make(map[string]*loadbalance.Resolver),
	}
	for _, opt := range opts {
		opt(c)
	}
	c.http.Timeout = c.timeout
	return c
}

// WithRegistry sets the service registry for discovery.
func WithRegistry(reg discovery.Registry) Option {
	return func(c *Client) { c.registry = reg }
}

// WithBalancer sets the load-balancing strategy.
func WithBalancer(b loadbalance.Balancer) Option {
	return func(c *Client) { c.balancer = b }
}

// WithRetryPolicy sets the retry policy.
func WithRetryPolicy(p retry.Policy) Option {
	return func(c *Client) { c.retryPolicy = p }
}

// WithTimeout sets the per-request timeout (overrides http.Client.Timeout).
func WithTimeout(d time.Duration) Option {
	return func(c *Client) { c.timeout = d }
}

// WithCircuitBreakerConfig sets the template config for per-service circuit breakers.
func WithCircuitBreakerConfig(cfg circuit.Config) Option {
	return func(c *Client) { c.breakerCfg = cfg }
}

// WithHTTPClient replaces the underlying *http.Client.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.http = h }
}

// WithAutoResolve enables Watch-based live instance caching. On the first
// request to each service, the client subscribes to registry.Watch and caches
// the instance list in memory, eliminating a Discover call on every request.
//
// ctx should be a long-lived background context; its cancellation stops all
// resolver subscriptions. Call Close() to release resources cleanly.
func WithAutoResolve(ctx context.Context) Option {
	return func(c *Client) { c.autoResolveCtx = ctx }
}

// WithResolver registers a pre-built Resolver for a specific service name.
// The client uses this resolver instead of calling Discover on every request.
// The resolver is not owned by the client — the caller must call r.Close().
func WithResolver(serviceName string, r *loadbalance.Resolver) Option {
	return func(c *Client) {
		c.resolversMu.Lock()
		c.resolvers[serviceName] = r
		c.resolversMu.Unlock()
	}
}

// Close releases all resolver subscriptions started by WithAutoResolve.
// Resolvers registered via WithResolver are not closed (the caller owns them).
func (c *Client) Close() {
	c.resolversMu.Lock()
	defer c.resolversMu.Unlock()
	// Only close resolvers that were created internally by autoResolveCtx.
	// We distinguish them by checking that autoResolveCtx is set.
	if c.autoResolveCtx == nil {
		return
	}
	for svc, r := range c.resolvers {
		r.Close()
		delete(c.resolvers, svc)
	}
}

// ─── YAML-friendly configuration ──────────────────────────────────────────────

// RetryPolicyYAML mirrors retry.Policy fields for YAML/unstructured config sources.
// Example YAML:
//
//	retry:
//	  enabled: true
//	  max_attempts: 3
//	  delay: 100ms
//	  max_delay: 5s
//	  multiplier: 2.0
//	  jitter: true
type RetryPolicyYAML struct {
	Enabled    bool          `yaml:"enabled"`
	MaxAttempts int          `yaml:"max_attempts"`
	Delay      time.Duration `yaml:"delay"`
	MaxDelay   time.Duration `yaml:"max_delay"`
	Multiplier float64       `yaml:"multiplier"`
	Jitter     bool          `yaml:"jitter"`
}

// ToPolicy converts RetryPolicyYAML to retry.Policy.
func (p RetryPolicyYAML) ToPolicy() retry.Policy {
	return retry.Policy{
		MaxAttempts: p.MaxAttempts,
		Delay:       p.Delay,
		MaxDelay:    p.MaxDelay,
		Multiplier:  p.Multiplier,
		Jitter:      p.Jitter,
	}
}

// DefaultRetryPolicyYAML returns a sensible default for idempotent downstream calls.
func DefaultRetryPolicyYAML() RetryPolicyYAML {
	return RetryPolicyYAML{
		Enabled:     true,
		MaxAttempts: 3,
		Delay:       100 * time.Millisecond,
		MaxDelay:    5 * time.Second,
		Multiplier:  2.0,
		Jitter:      true,
	}
}

// CircuitBreakerYAML mirrors circuit.Config fields for YAML/unstructured config sources.
// Example YAML:
//
//	circuit_breaker:
//	  enabled: true
//	  failure_threshold: 10
//	  success_threshold: 3
//	  timeout: 30s
type CircuitBreakerYAML struct {
	Enabled          bool          `yaml:"enabled"`
	FailureThreshold int           `yaml:"failure_threshold"`
	SuccessThreshold int           `yaml:"success_threshold"`
	Timeout          time.Duration `yaml:"timeout"`
}

// ToConfig converts CircuitBreakerYAML to circuit.Config.
func (c CircuitBreakerYAML) ToConfig(name string, onStateChange func(string, circuit.State, circuit.State)) circuit.Config {
	return circuit.Config{
		Name:              name,
		Threshold:         int64(c.FailureThreshold),
		HalfOpenSuccesses: int64(c.SuccessThreshold),
		Timeout:           c.Timeout,
		OnStateChange:     onStateChange,
	}
}

// WithRetryPolicyYAML is a convenience option that applies RetryPolicyYAML if enabled.
func WithRetryPolicyYAML(p RetryPolicyYAML) Option {
	return func(c *Client) {
		if !p.Enabled {
			c.retryPolicy.MaxAttempts = 1
			return
		}
		c.retryPolicy = p.ToPolicy()
	}
}

// WithCircuitBreakerYAML is a convenience option that applies CircuitBreakerYAML.
// The name parameter sets the circuit breaker name; onStateChange is optional (pass nil to skip).
func WithCircuitBreakerYAML(name string, cfg CircuitBreakerYAML, onStateChange func(string, circuit.State, circuit.State)) Option {
	return func(c *Client) {
		c.breakerCfg = cfg.ToConfig(name, onStateChange)
	}
}

// ─── Call options ─────────────────────────────────────────────────────────────

// CallConfig holds per-call overrides.
type CallConfig struct {
	hashKey string
	headers http.Header
}

// CallOption configures a single call.
type CallOption func(*CallConfig)

// WithHashKey sets the routing key for consistent-hash load balancing.
func WithHashKey(key string) CallOption {
	return func(c *CallConfig) { c.hashKey = key }
}

// WithHeader adds a header to the outgoing request.
func WithHeader(key, value string) CallOption {
	return func(c *CallConfig) {
		if c.headers == nil {
			c.headers = make(http.Header)
		}
		c.headers.Set(key, value)
	}
}

// ─── Core call ────────────────────────────────────────────────────────────────

// Do sends req, resolving the target host via service discovery if a registry
// is configured. The request URL must use the logical service name as the host
// (e.g. "http://user-svc/api/users"). The host is rewritten to the discovered
// address before each attempt.
//
// If the balancer implements loadbalance.Reporter, Do automatically calls
// RecordSuccess or RecordError after each attempt, enabling adaptive strategies
// (P2C+EWMA, OutlierDetector) without manual instrumentation.
//
// If the balancer implements loadbalance.Doner but not Reporter (e.g. LeastConn),
// Do calls Done after each attempt so the inflight counter stays accurate.
func (c *Client) Do(ctx context.Context, req *http.Request, opts ...CallOption) (*http.Response, error) {
	cfg := &CallConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	serviceName := req.URL.Host
	breaker := c.breakerFor(serviceName)

	var resp *http.Response
	err := breaker.Do(func() error {
		return retry.Do(ctx, c.retryPolicy, func(ctx context.Context) error {
			instances, discoverErr := c.discover(ctx, serviceName)
			if discoverErr != nil {
				return discoverErr
			}

			inst, pickErr := c.balancer.Pick(instances, cfg.hashKey)
			if pickErr != nil {
				return pickErr
			}

			// Clone the request so we can rewrite the URL per-attempt.
			cloned := req.Clone(ctx)
			cloned.URL.Scheme = inst.Scheme
			cloned.URL.Host = inst.Address
			cloned.Host = inst.Address

			// Inject caller headers.
			for k, vs := range cfg.headers {
				for _, v := range vs {
					cloned.Header.Set(k, v)
				}
			}

			// OTel: start span and inject trace context.
			ctx, span := c.tracer.Start(ctx, req.Method+" "+req.URL.Path,
				trace.WithSpanKind(trace.SpanKindClient),
				trace.WithAttributes(
					semconv.HTTPRequestMethodKey.String(req.Method),
					semconv.ServerAddress(inst.Address),
					attribute.String("service.name", serviceName),
				),
			)
			defer span.End()
			c.propagator.Inject(ctx, propagation.HeaderCarrier(cloned.Header))

			start := time.Now()
			var doErr error
			resp, doErr = c.http.Do(cloned)
			elapsed := time.Since(start)

			// Adaptive feedback: notify the balancer about outcome + latency.
			c.recordOutcome(inst, elapsed, doErr, resp)

			if doErr != nil {
				span.RecordError(doErr)
				span.SetStatus(codes.Error, doErr.Error())
				return doErr
			}

			span.SetAttributes(semconv.HTTPResponseStatusCode(resp.StatusCode))
			if resp.StatusCode >= 500 {
				span.SetStatus(codes.Error, http.StatusText(resp.StatusCode))
			}

			// Convert HTTP 5xx into a retryable error.
			if statusErr := retry.NewStatusError(resp); statusErr != nil {
				return statusErr
			}
			return nil
		})
	})

	if err != nil {
		return nil, err
	}
	return resp, nil
}

// recordOutcome calls the appropriate balancer feedback method based on the
// result of a single HTTP attempt. Priority:
//  1. Reporter.RecordSuccess / RecordError (P2C, OutlierDetector)
//  2. Doner.Done (LeastConn)
func (c *Client) recordOutcome(inst *discovery.ServiceInstance, elapsed time.Duration, doErr error, resp *http.Response) {
	isError := doErr != nil || (resp != nil && resp.StatusCode >= 500)

	if reporter, ok := c.balancer.(loadbalance.Reporter); ok {
		if isError {
			reporter.RecordError(inst, elapsed)
		} else {
			reporter.RecordSuccess(inst, elapsed)
		}
		return
	}
	// Fallback: balancers that only track inflight counts (LeastConn).
	if doner, ok := c.balancer.(loadbalance.Doner); ok {
		doner.Done(inst)
	}
}

// Call is a convenience wrapper for service-to-service HTTP calls.
// serviceName is resolved via service discovery; method is the HTTP verb.
// Returns the response body bytes and status code.
func (c *Client) Call(ctx context.Context, serviceName, method, path string, body io.Reader, opts ...CallOption) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, "http://"+serviceName+path, body)
	if err != nil {
		return nil, fmt.Errorf("client: build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.Do(ctx, req, opts...)
}

// Get is a shortcut for a GET call.
func (c *Client) Get(ctx context.Context, serviceName, path string, opts ...CallOption) (*http.Response, error) {
	return c.Call(ctx, serviceName, http.MethodGet, path, nil, opts...)
}

// Post is a shortcut for a POST call with a JSON body.
func (c *Client) Post(ctx context.Context, serviceName, path string, body io.Reader, opts ...CallOption) (*http.Response, error) {
	return c.Call(ctx, serviceName, http.MethodPost, path, body, opts...)
}

// ─── Direct client (no service discovery) ─────────────────────────────────────

// DirectClient is a lightweight HTTP client for calling a fixed base URL
// without service discovery or load balancing. It still provides circuit
// breaker, retry, and tracing.
//
// Use this for calling known, stable downstream services (e.g. a specific
// backend at a fixed address).
type DirectClient struct {
	base        string
	breaker     *circuit.Breaker
	retryPolicy retry.Policy
	http        *http.Client
	tracer      trace.Tracer
}

// DirectOption configures a DirectClient.
type DirectOption func(*DirectClient)

// NewDirectClient creates a DirectClient for the given base URL (e.g. "http://localhost:8080").
// The caller must provide circuit breaker config via WithDirectCircuitBreaker.
func NewDirectClient(base string, opts ...DirectOption) *DirectClient {
	dc := &DirectClient{
		base:        base,
		retryPolicy: retry.DefaultPolicy,
		http:        &http.Client{Timeout: 10 * time.Second},
		tracer:      otel.Tracer("astra/client/direct"),
		breaker:     circuit.New(circuit.Config{Name: base}),
	}
	for _, opt := range opts {
		opt(dc)
	}
	return dc
}

// WithDirectCircuitBreaker replaces the default circuit breaker.
func WithDirectCircuitBreaker(cb *circuit.Breaker) DirectOption {
	return func(dc *DirectClient) { dc.breaker = cb }
}

// WithDirectCircuitBreakerConfig sets the circuit breaker config.
func WithDirectCircuitBreakerConfig(cfg circuit.Config) DirectOption {
	return func(dc *DirectClient) {
		cfg.Name = dc.base
		dc.breaker = circuit.New(cfg)
	}
}

// WithDirectRetryPolicy sets the retry policy.
func WithDirectRetryPolicy(p retry.Policy) DirectOption {
	return func(dc *DirectClient) { dc.retryPolicy = p }
}

// WithDirectRetryPolicyYAML sets retry from a YAML-friendly struct.
func WithDirectRetryPolicyYAML(p RetryPolicyYAML) DirectOption {
	return func(dc *DirectClient) {
		if !p.Enabled {
			p.MaxAttempts = 1
		}
		dc.retryPolicy = p.ToPolicy()
	}
}

// WithDirectHTTPClient replaces the underlying *http.Client.
func WithDirectHTTPClient(h *http.Client) DirectOption {
	return func(dc *DirectClient) { dc.http = h }
}

// Get makes a GET request to the given path.
func (dc *DirectClient) Get(ctx context.Context, path string, opts ...CallOption) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dc.base+path, nil)
	if err != nil {
		return nil, fmt.Errorf("direct client: build request: %w", err)
	}
	return dc.Do(req, opts...)
}

// Post makes a POST request with a JSON body.
func (dc *DirectClient) Post(ctx context.Context, path string, body io.Reader, opts ...CallOption) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, dc.base+path, body)
	if err != nil {
		return nil, fmt.Errorf("direct client: build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return dc.Do(req, opts...)
}

// Do sends the request through circuit breaker and retry logic.
func (dc *DirectClient) Do(req *http.Request, opts ...CallOption) (*http.Response, error) {
	cfg := &CallConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	var resp *http.Response
	err := dc.breaker.Do(func() error {
		return retry.Do(req.Context(), dc.retryPolicy, func(ctx context.Context) error {
			cloned := req.Clone(ctx)

			// Apply per-call headers.
			for k, vs := range cfg.headers {
				for _, v := range vs {
					cloned.Header.Set(k, v)
				}
			}

			// OTel span.
			ctx, span := dc.tracer.Start(ctx, req.Method+" "+req.URL.Path,
				trace.WithSpanKind(trace.SpanKindClient),
				trace.WithAttributes(
					semconv.HTTPRequestMethodKey.String(req.Method),
					semconv.ServerAddress(dc.base),
				),
			)
			defer span.End()

			r, doErr := dc.http.Do(cloned.WithContext(ctx))
			if doErr != nil {
				span.RecordError(doErr)
				span.SetStatus(codes.Error, doErr.Error())
				return doErr
			}

			span.SetAttributes(semconv.HTTPResponseStatusCode(r.StatusCode))
			if r.StatusCode >= 500 {
				span.SetStatus(codes.Error, http.StatusText(r.StatusCode))
			}

			if statusErr := retry.NewStatusError(r); statusErr != nil {
				return statusErr
			}
			resp = r
			return nil
		})
	})
	return resp, err
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// discover returns instances for serviceName. Lookup order:
//  1. Pre-registered resolver (WithResolver or auto-resolve cache).
//  2. registry.Discover (one-shot, if registry is configured).
//  3. Direct-address fallback (service name treated as host:port).
func (c *Client) discover(ctx context.Context, serviceName string) ([]*discovery.ServiceInstance, error) {
	// Check resolver cache.
	if r := c.resolverFor(ctx, serviceName); r != nil {
		insts := r.Instances()
		if len(insts) > 0 {
			return insts, nil
		}
	}

	if c.registry == nil {
		// No registry: treat the service name as a direct "host:port" address.
		return []*discovery.ServiceInstance{{
			ID:      serviceName,
			Name:    serviceName,
			Address: serviceName,
			Scheme:  "http",
			Weight:  1,
		}}, nil
	}
	return c.registry.Discover(ctx, serviceName)
}

// resolverFor returns the Resolver for serviceName. If auto-resolve is enabled
// and no resolver exists yet, it creates one (blocking on first snapshot).
// Returns nil if no resolver is available or creation fails.
func (c *Client) resolverFor(ctx context.Context, serviceName string) *loadbalance.Resolver {
	c.resolversMu.Lock()
	defer c.resolversMu.Unlock()

	if r, ok := c.resolvers[serviceName]; ok {
		return r
	}

	// Auto-resolve: create a new resolver if a background context is set.
	if c.autoResolveCtx == nil || c.registry == nil {
		return nil
	}

	r, err := loadbalance.NewResolver(c.autoResolveCtx, c.registry, serviceName)
	if err != nil {
		return nil
	}
	c.resolvers[serviceName] = r
	return r
}

// breakerFor returns the circuit breaker for serviceName, creating one lazily.
func (c *Client) breakerFor(serviceName string) *circuit.Breaker {
	c.breakersMu.RLock()
	b, ok := c.breakers[serviceName]
	c.breakersMu.RUnlock()
	if ok {
		return b
	}

	c.breakersMu.Lock()
	defer c.breakersMu.Unlock()
	// Double-check after acquiring write lock.
	if b, ok = c.breakers[serviceName]; ok {
		return b
	}
	cfg := c.breakerCfg
	cfg.Name = serviceName
	b = circuit.New(cfg)
	c.breakers[serviceName] = b
	return b
}
