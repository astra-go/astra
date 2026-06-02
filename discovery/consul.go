//go:build consul || alltags

// Package discovery provides service registry implementations.
// This file contains the Consul backend.
package discovery

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/hashicorp/consul/api"
)

const (
	consulCheckTTL          = "30s"
	consulDeregisterTimeout = "5m"
	consulKeepaliveInterval = 10 * time.Second
)

// ConsulRegistry implements Registry using Consul.
type ConsulRegistry struct {
	client *api.Client
}

// NewConsulRegistry creates a Registry wrapping the given Consul client.
//
// Usage:
//
//	cfg := api.DefaultConfig()
//	cfg.Address = "localhost:8500"
//	reg := discovery.NewConsulRegistry(client)
//	defer reg.Close()
//
//	_ = reg.Register(ctx, &discovery.ServiceInstance{
//	    ID: "user-svc-1", Name: "user-svc", Address: "10.0.0.1:8081",
//	})
func NewConsulRegistry(client *api.Client) *ConsulRegistry {
	return &ConsulRegistry{client: client}
}

// NewConsulRegistryFromConfig creates a Consul client from cfg and returns a Registry.
func NewConsulRegistryFromConfig(cfg *api.Config) (*ConsulRegistry, error) {
	client, err := api.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("consul: create client: %w", err)
	}
	return NewConsulRegistry(client), nil
}

// Register registers an instance with a TTL health check and starts a keepalive goroutine.
func (r *ConsulRegistry) Register(ctx context.Context, instance *ServiceInstance) error {
	if instance.ID == "" {
		return ErrInstanceIDEmpty
	}
	if instance.Weight <= 0 {
		instance.Weight = 1
	}
	if instance.Scheme == "" {
		instance.Scheme = "http"
	}

	host, port, err := splitHostPortConsul(instance.Address)
	if err != nil {
		return fmt.Errorf("consul: invalid address %q: %w", instance.Address, err)
	}

	// Copy metadata and add weight so it can be recovered on Discover.
	meta := make(map[string]string, len(instance.Metadata)+2)
	for k, v := range instance.Metadata {
		meta[k] = v
	}
	meta["weight"] = strconv.Itoa(instance.Weight)
	meta["scheme"] = instance.Scheme

	reg := &api.AgentServiceRegistration{
		ID:      instance.ID,
		Name:    instance.Name,
		Address: host,
		Port:    port,
		Meta:    meta,
		Check: &api.AgentServiceCheck{
			CheckID:                        "service:" + instance.ID,
			TTL:                            consulCheckTTL,
			DeregisterCriticalServiceAfter: consulDeregisterTimeout,
		},
	}

	if err := r.client.Agent().ServiceRegisterOpts(reg, (&api.ServiceRegisterOpts{}).WithContext(ctx)); err != nil {
		return fmt.Errorf("consul: register service: %w", err)
	}

	// Keepalive: pass TTL check on interval.
	go func() {
		checkID := "service:" + instance.ID
		ticker := time.NewTicker(consulKeepaliveInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = r.client.Agent().PassTTL(checkID, "alive")
			}
		}
	}()

	return nil
}

// Deregister deregisters the service instance.
func (r *ConsulRegistry) Deregister(_ context.Context, instanceID string) error {
	if err := r.client.Agent().ServiceDeregister(instanceID); err != nil {
		return fmt.Errorf("consul: deregister service %s: %w", instanceID, err)
	}
	return nil
}

// Discover returns all passing instances of serviceName.
func (r *ConsulRegistry) Discover(_ context.Context, serviceName string) ([]*ServiceInstance, error) {
	services, _, err := r.client.Health().Service(serviceName, "", true, nil)
	if err != nil {
		return nil, fmt.Errorf("consul: discover %s: %w", serviceName, err)
	}
	if len(services) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, serviceName)
	}

	result := make([]*ServiceInstance, 0, len(services))
	for _, s := range services {
		addr := s.Service.Address
		if addr == "" {
			addr = s.Node.Address
		}
		weight := 1
		if w, err := strconv.Atoi(s.Service.Meta["weight"]); err == nil && w > 0 {
			weight = w
		}
		scheme := s.Service.Meta["scheme"]
		if scheme == "" {
			scheme = "http"
		}
		result = append(result, &ServiceInstance{
			ID:       s.Service.ID,
			Name:     s.Service.Service,
			Address:  fmt.Sprintf("%s:%d", addr, s.Service.Port),
			Scheme:   scheme,
			Weight:   weight,
			Metadata: s.Service.Meta,
		})
	}
	return result, nil
}

// Watch returns a channel that emits updated instance lists using Consul blocking queries.
func (r *ConsulRegistry) Watch(ctx context.Context, serviceName string) (<-chan []*ServiceInstance, error) {
	ch := make(chan []*ServiceInstance, 8)

	// Emit current state immediately.
	if instances, err := r.Discover(ctx, serviceName); err == nil {
		select {
		case ch <- instances:
		default:
		}
	}

	go func() {
		defer close(ch)
		var lastIndex uint64
		for {
			if ctx.Err() != nil {
				return
			}
			services, meta, err := r.client.Health().Service(serviceName, "", true,
				&api.QueryOptions{
					WaitIndex: lastIndex,
					WaitTime:  30 * time.Second,
				})
			if err != nil {
				select {
				case <-ctx.Done():
					return
				case <-time.After(2 * time.Second):
					continue
				}
			}
			if meta.LastIndex == lastIndex {
				continue
			}
			lastIndex = meta.LastIndex

			result := make([]*ServiceInstance, 0, len(services))
			for _, s := range services {
				addr := s.Service.Address
				if addr == "" {
					addr = s.Node.Address
				}
				weight := 1
				if w, err := strconv.Atoi(s.Service.Meta["weight"]); err == nil && w > 0 {
					weight = w
				}
				scheme := s.Service.Meta["scheme"]
				if scheme == "" {
					scheme = "http"
				}
				result = append(result, &ServiceInstance{
					ID:       s.Service.ID,
					Name:     s.Service.Service,
					Address:  fmt.Sprintf("%s:%d", addr, s.Service.Port),
					Scheme:   scheme,
					Weight:   weight,
					Metadata: s.Service.Meta,
				})
			}

			select {
			case ch <- result:
			case <-ctx.Done():
				return
			default:
			}
		}
	}()

	return ch, nil
}

// Close is a no-op; the Consul client has no Close method.
func (r *ConsulRegistry) Close() error { return nil }

// splitHostPortConsul splits "host:port" into host and int port.
func splitHostPortConsul(addr string) (string, int, error) {
	var host, portStr string
	var err error
	// Handle IPv6 addresses.
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			host = addr[:i]
			portStr = addr[i+1:]
			break
		}
	}
	if portStr == "" {
		return "", 0, fmt.Errorf("missing port in address")
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, fmt.Errorf("invalid port: %w", err)
	}
	return host, port, nil
}
