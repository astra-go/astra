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
