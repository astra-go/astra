// Package discovery provides service registration and discovery interfaces
// and implementations for Astra microservices.
//
// Usage — register a service instance:
//
//	reg := discovery.NewInMemoryRegistry()
//	ctx := context.Background()
//	_ = reg.Register(ctx, &discovery.ServiceInstance{
//	    ID: "user-svc-1", Name: "user-svc", Address: "localhost:8081",
//	})
//
// Usage — discover and watch instances:
//
//	instances, _ := reg.Discover(ctx, "user-svc")
//	ch, _ := reg.Watch(ctx, "user-svc")
//	for update := range ch { ... }
//
// Backend adapters are in sub-packages:
//
//	discovery/etcd   — etcd v3 backend
//	discovery/consul — Consul backend
package discovery

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// ─── Core types ───────────────────────────────────────────────────────────────

// ServiceInstance represents a healthy, registered service endpoint.
type ServiceInstance struct {
	// ID uniquely identifies this instance (e.g. "user-svc-abc123").
	ID string
	// Name is the logical service name (e.g. "user-svc").
	Name string
	// Address is "host:port" (e.g. "10.0.1.5:8081").
	Address string
	// Scheme is the protocol: "http", "https", or "grpc". Default: "http".
	Scheme string
	// Weight controls load-balancer distribution (≥1). Default: 1.
	Weight int
	// Metadata holds arbitrary key-value annotations.
	Metadata map[string]string
}

// Sentinel errors.
var (
	ErrNotFound        = errors.New("discovery: service not found")
	ErrNoInstances     = errors.New("discovery: no healthy instances")
	ErrInstanceIDEmpty = errors.New("discovery: instance ID must not be empty")
)

// ─── Registry interface ───────────────────────────────────────────────────────

// Registry is the interface for service registration and discovery.
// All methods must be safe for concurrent use.
type Registry interface {
	// Register registers a service instance, refreshing the entry on each call.
	Register(ctx context.Context, instance *ServiceInstance) error
	// Deregister removes a service instance by its ID.
	Deregister(ctx context.Context, instanceID string) error
	// Discover returns all healthy instances of the named service.
	Discover(ctx context.Context, serviceName string) ([]*ServiceInstance, error)
	// Watch returns a channel that emits the full instance list whenever it changes.
	// The channel is closed when ctx is cancelled. The first emit is the current state.
	Watch(ctx context.Context, serviceName string) (<-chan []*ServiceInstance, error)
	// Close releases all resources held by the registry.
	Close() error
}

// ─── In-Memory Registry ───────────────────────────────────────────────────────

// InMemoryRegistry is a thread-safe, in-process registry.
// It is designed for testing and single-process service meshes.
type InMemoryRegistry struct {
	mu        sync.RWMutex
	instances map[string]*ServiceInstance              // instanceID → instance
	watchers  map[string][]chan []*ServiceInstance     // serviceName → watcher channels
}

// NewInMemoryRegistry creates an empty in-memory registry.
func NewInMemoryRegistry() *InMemoryRegistry {
	return &InMemoryRegistry{
		instances: make(map[string]*ServiceInstance),
		watchers:  make(map[string][]chan []*ServiceInstance),
	}
}

// Register adds or updates an instance. Thread-safe.
func (r *InMemoryRegistry) Register(_ context.Context, instance *ServiceInstance) error {
	if instance.ID == "" {
		return ErrInstanceIDEmpty
	}
	if instance.Weight <= 0 {
		instance.Weight = 1
	}
	if instance.Scheme == "" {
		instance.Scheme = "http"
	}

	r.mu.Lock()
	r.instances[instance.ID] = instance
	name := instance.Name
	r.mu.Unlock()

	r.notify(name)
	return nil
}

// Deregister removes the instance with the given ID. Thread-safe.
func (r *InMemoryRegistry) Deregister(_ context.Context, instanceID string) error {
	r.mu.Lock()
	inst, ok := r.instances[instanceID]
	if ok {
		delete(r.instances, instanceID)
	}
	r.mu.Unlock()
	if ok {
		r.notify(inst.Name)
	}
	return nil
}

// Discover returns all instances whose Name matches serviceName.
func (r *InMemoryRegistry) Discover(_ context.Context, serviceName string) ([]*ServiceInstance, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*ServiceInstance
	for _, inst := range r.instances {
		if inst.Name == serviceName {
			cp := *inst // shallow copy to prevent caller mutations
			result = append(result, &cp)
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, serviceName)
	}
	return result, nil
}

// Watch returns a channel that receives instance lists when the service changes.
// Cancel ctx to stop watching and close the channel.
func (r *InMemoryRegistry) Watch(ctx context.Context, serviceName string) (<-chan []*ServiceInstance, error) {
	ch := make(chan []*ServiceInstance, 8)

	r.mu.Lock()
	r.watchers[serviceName] = append(r.watchers[serviceName], ch)
	r.mu.Unlock()

	// Emit current state immediately.
	if instances, err := r.Discover(ctx, serviceName); err == nil {
		select {
		case ch <- instances:
		default:
		}
	}

	// Close channel and remove watcher when ctx is cancelled.
	go func() {
		<-ctx.Done()
		r.mu.Lock()
		watchers := r.watchers[serviceName]
		for i, w := range watchers {
			if w == ch {
				r.watchers[serviceName] = append(watchers[:i], watchers[i+1:]...)
				break
			}
		}
		r.mu.Unlock()
		close(ch)
	}()

	return ch, nil
}

// Close is a no-op for the in-memory registry.
func (r *InMemoryRegistry) Close() error { return nil }

// notify sends the current instance list to all watchers of serviceName.
// Called after every Register/Deregister. Does not block on slow consumers.
func (r *InMemoryRegistry) notify(serviceName string) {
	r.mu.RLock()
	var instances []*ServiceInstance
	for _, inst := range r.instances {
		if inst.Name == serviceName {
			cp := *inst
			instances = append(instances, &cp)
		}
	}
	watchers := r.watchers[serviceName]
	r.mu.RUnlock()

	for _, ch := range watchers {
		select {
		case ch <- instances:
		default: // drop update for slow consumers; they will catch the next one
		}
	}
}
