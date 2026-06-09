// gRPC connection pool for service-to-service gRPC calls.
// Pools reuse connections per instance address to avoid repeated TLS handshakes.
package client

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	"github.com/astra-go/astra/circuit"
	"github.com/astra-go/astra/discovery"
	"github.com/astra-go/astra/loadbalance"
	"github.com/astra-go/astra/retry"
)

// GRPCPool manages a pool of gRPC client connections across service instances.
// Connections are cached by address and reused across calls.
//
// Thread-safe.
type GRPCPool struct {
	registry    discovery.Registry
	balancer    loadbalance.Balancer
	retryPolicy retry.Policy
	dialOpts    []grpc.DialOption
	tracer      trace.Tracer

	mu      sync.RWMutex
	conns   map[string]*grpc.ClientConn // address → connection

	breakersMu sync.RWMutex
	breakers   map[string]*circuit.Breaker
	breakerCfg circuit.Config
}

// GRPCOption configures a GRPCPool.
type GRPCOption func(*GRPCPool)

// WithGRPCRegistry sets the service registry.
func WithGRPCRegistry(reg discovery.Registry) GRPCOption {
	return func(p *GRPCPool) { p.registry = reg }
}

// WithGRPCBalancer sets the load-balancing strategy.
func WithGRPCBalancer(b loadbalance.Balancer) GRPCOption {
	return func(p *GRPCPool) { p.balancer = b }
}

// WithGRPCRetryPolicy sets the retry policy.
func WithGRPCRetryPolicy(rp retry.Policy) GRPCOption {
	return func(p *GRPCPool) { p.retryPolicy = rp }
}

// WithGRPCDialOptions appends extra grpc.DialOptions (e.g. TLS credentials).
func WithGRPCDialOptions(opts ...grpc.DialOption) GRPCOption {
	return func(p *GRPCPool) { p.dialOpts = append(p.dialOpts, opts...) }
}

// WithGRPCCircuitBreakerConfig sets the circuit breaker template config.
func WithGRPCCircuitBreakerConfig(cfg circuit.Config) GRPCOption {
	return func(p *GRPCPool) { p.breakerCfg = cfg }
}

// NewGRPCPool creates a GRPCPool with the provided options.
func NewGRPCPool(opts ...GRPCOption) *GRPCPool {
	p := &GRPCPool{
		balancer:    loadbalance.NewRoundRobin(),
		retryPolicy: retry.Policy{MaxAttempts: 2, Delay: 50 * time.Millisecond},
		tracer:      otel.Tracer("astra/client/grpc"),
		conns:       make(map[string]*grpc.ClientConn),
		breakers:    make(map[string]*circuit.Breaker),
		breakerCfg: circuit.Config{
			Threshold:         5,
			Timeout:           30 * time.Second,
			HalfOpenSuccesses: 2,
		},
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Conn returns a *grpc.ClientConn for the given service.
// It discovers instances, picks one via the load balancer, and returns a
// cached (or newly dialed) connection for that address.
func (p *GRPCPool) Conn(ctx context.Context, serviceName string, callOpts ...CallOption) (*grpc.ClientConn, error) {
	cfg := &CallConfig{}
	for _, opt := range callOpts {
		opt(cfg)
	}

	instances, err := p.grpcDiscover(ctx, serviceName)
	if err != nil {
		return nil, err
	}

	inst, err := p.balancer.Pick(instances, cfg.hashKey)
	if err != nil {
		return nil, err
	}

	return p.connFor(inst)
}

// Invoke calls a gRPC method on the named service, wrapping the call with a
// circuit breaker and retry policy.
//
//	err := pool.Invoke(ctx, "user-svc", "/user.UserService/GetUser", &req, &resp)
func (p *GRPCPool) Invoke(ctx context.Context, serviceName, method string, req, reply any, opts ...CallOption) error {
	breaker := p.grpcBreakerFor(serviceName)

	return breaker.Do(func() error {
		return retry.Do(ctx, p.retryPolicy, func(ctx context.Context) error {
			conn, err := p.Conn(ctx, serviceName, opts...)
			if err != nil {
				return err
			}
			return conn.Invoke(ctx, method, req, reply)
		})
	})
}

// Close closes all pooled connections.
func (p *GRPCPool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	var firstErr error
	for addr, conn := range p.conns {
		if err := conn.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("grpcpool: close %s: %w", addr, err)
		}
		delete(p.conns, addr)
	}
	return firstErr
}

// connFor returns or creates a *grpc.ClientConn for the given instance address.
func (p *GRPCPool) connFor(inst *discovery.ServiceInstance) (*grpc.ClientConn, error) {
	p.mu.RLock()
	conn, ok := p.conns[inst.Address]
	p.mu.RUnlock()
	if ok {
		return conn, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	// Double-check.
	if conn, ok = p.conns[inst.Address]; ok {
		return conn, nil
	}

	// Default dial options: insecure + keepalive.
	// Use WithGRPCDialOptions to override (e.g. pass TLS credentials).
	// SECURITY WARNING: Default is insecure (plaintext). Use TLS in production.
	defaults := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                2 * time.Minute,
			Timeout:             20 * time.Second,
			PermitWithoutStream: true,
		}),
	}

	// Warn once if using insecure credentials
	if os.Getenv("ASTRA_GRPC_INSECURE_OK") != "1" {
		slog.Warn("SECURITY WARNING: gRPC client using insecure (plaintext) connection. " +
			"Use WithGRPCDialOptions(grpc.WithTransportCredentials(...)) with TLS in production. " +
			"Set ASTRA_GRPC_INSECURE_OK=1 to suppress this warning.")
	}

	dialOpts := append(defaults, p.dialOpts...)

	conn, err := grpc.NewClient(inst.Address, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("grpcpool: dial %s: %w", inst.Address, err)
	}
	p.conns[inst.Address] = conn
	return conn, nil
}

func (p *GRPCPool) grpcDiscover(ctx context.Context, serviceName string) ([]*discovery.ServiceInstance, error) {
	if p.registry == nil {
		return []*discovery.ServiceInstance{{
			ID:      serviceName,
			Name:    serviceName,
			Address: serviceName,
			Scheme:  "grpc",
			Weight:  1,
		}}, nil
	}
	return p.registry.Discover(ctx, serviceName)
}

func (p *GRPCPool) grpcBreakerFor(serviceName string) *circuit.Breaker {
	p.breakersMu.RLock()
	b, ok := p.breakers[serviceName]
	p.breakersMu.RUnlock()
	if ok {
		return b
	}

	p.breakersMu.Lock()
	defer p.breakersMu.Unlock()
	if b, ok = p.breakers[serviceName]; ok {
		return b
	}
	cfg := p.breakerCfg
	cfg.Name = serviceName
	b = circuit.New(cfg)
	p.breakers[serviceName] = b
	return b
}
