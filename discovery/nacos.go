//go:build nacos || alltags

// Package discovery provides service registry implementations.
// This file contains the Nacos backend.
package discovery

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync"

	"github.com/nacos-group/nacos-sdk-go/v2/clients/naming_client"
	"github.com/nacos-group/nacos-sdk-go/v2/model"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
)

// metaKeyID and metaKeyScheme are hidden metadata keys used to carry
// Astra-specific fields through the Nacos naming server.
const (
	nacosMetaKeyID     = "_astra_id"
	nacosMetaKeyScheme = "_astra_scheme"
)

// NacosConfig configures the Nacos registry.
type NacosConfig struct {
	// Group is the Nacos service group. Default: "DEFAULT_GROUP".
	Group string
}

// NacosRegistry implements Registry using the Nacos naming service.
// All methods are safe for concurrent use.
//
// Service instances are registered to the Nacos naming service with a health
// check. The Astra-specific fields (instance ID, scheme, metadata) are stored
// in the Nacos instance Metadata map so they survive round-trips through the
// Nacos server.
//
// Usage:
//
//	import (
//	    "github.com/nacos-group/nacos-sdk-go/v2/clients"
//	    "github.com/nacos-group/nacos-sdk-go/v2/common/constant"
//	    "github.com/nacos-group/nacos-sdk-go/v2/vo"
//	)
//
//	sc := []constant.ServerConfig{{IpAddr: "127.0.0.1", Port: 8848}}
//	cc := constant.NewClientConfig(
//	    constant.WithNamespaceId("public"),
//	    constant.WithTimeoutMs(5000),
//	    constant.WithLogLevel("warn"),
//	)
//	namingClient, _ := clients.NewNamingClient(vo.NacosClientParam{
//	    ClientConfig:  cc,
//	    ServerConfigs: sc,
//	})
//
//	reg := discovery.NewNacosRegistry(namingClient)
//
//	_ = reg.Register(ctx, &discovery.ServiceInstance{
//	    ID:      "user-svc-1",
//	    Name:    "user-svc",
//	    Address: "10.0.0.1:8081",
//	    Scheme:  "http",
//	    Weight:  1,
//	})
type NacosRegistry struct {
	client naming_client.INamingClient
	group  string

	mu        sync.Mutex
	instances map[string]*ServiceInstance // instanceID → instance
}

// NewNacosRegistry creates a Nacos-backed Registry.
func NewNacosRegistry(client naming_client.INamingClient, cfgs ...NacosConfig) *NacosRegistry {
	cfg := NacosConfig{}
	if len(cfgs) > 0 {
		cfg = cfgs[0]
	}
	if cfg.Group == "" {
		cfg.Group = "DEFAULT_GROUP"
	}
	return &NacosRegistry{
		client:    client,
		group:     cfg.Group,
		instances: make(map[string]*ServiceInstance),
	}
}

// Register registers a service instance with the Nacos naming service.
// Ephemeral instances are used so the entry is automatically removed if the
// process dies without calling Deregister.
func (r *NacosRegistry) Register(ctx context.Context, instance *ServiceInstance) error {
	if instance.ID == "" {
		return ErrInstanceIDEmpty
	}
	if instance.Weight <= 0 {
		instance.Weight = 1
	}
	if instance.Scheme == "" {
		instance.Scheme = "http"
	}

	host, port, err := splitHostPortNacos(instance.Address)
	if err != nil {
		return fmt.Errorf("discovery/nacos: invalid address %q: %w", instance.Address, err)
	}

	meta := cloneMetaNacos(instance.Metadata)
	meta[nacosMetaKeyID] = instance.ID
	meta[nacosMetaKeyScheme] = instance.Scheme

	ok, err := r.client.RegisterInstance(vo.RegisterInstanceParam{
		ServiceName: instance.Name,
		GroupName:   r.group,
		Ip:          host,
		Port:        port,
		Weight:      float64(instance.Weight),
		Enable:      true,
		Healthy:     true,
		Ephemeral:   true,
		Metadata:    meta,
	})
	if err != nil {
		return fmt.Errorf("discovery/nacos: register %q: %w", instance.ID, err)
	}
	if !ok {
		return fmt.Errorf("discovery/nacos: register %q: server returned false", instance.ID)
	}

	r.mu.Lock()
	r.instances[instance.ID] = instance
	r.mu.Unlock()
	return nil
}

// Deregister removes a service instance from the Nacos naming service.
func (r *NacosRegistry) Deregister(_ context.Context, instanceID string) error {
	r.mu.Lock()
	inst, ok := r.instances[instanceID]
	if ok {
		delete(r.instances, instanceID)
	}
	r.mu.Unlock()
	if !ok {
		return nil // already gone or never registered through this client
	}

	host, port, err := splitHostPortNacos(inst.Address)
	if err != nil {
		return fmt.Errorf("discovery/nacos: invalid address %q: %w", inst.Address, err)
	}

	ok2, err := r.client.DeregisterInstance(vo.DeregisterInstanceParam{
		ServiceName: inst.Name,
		GroupName:   r.group,
		Ip:          host,
		Port:        port,
		Ephemeral:   true,
	})
	if err != nil {
		return fmt.Errorf("discovery/nacos: deregister %q: %w", instanceID, err)
	}
	if !ok2 {
		return fmt.Errorf("discovery/nacos: deregister %q: server returned false", instanceID)
	}
	return nil
}

// Discover returns all healthy, enabled instances of the named service.
func (r *NacosRegistry) Discover(_ context.Context, serviceName string) ([]*ServiceInstance, error) {
	instances, err := r.client.SelectInstances(vo.SelectInstancesParam{
		ServiceName: serviceName,
		GroupName:   r.group,
		HealthyOnly: true,
	})
	if err != nil {
		return nil, fmt.Errorf("discovery/nacos: discover %q: %w", serviceName, err)
	}
	if len(instances) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, serviceName)
	}
	return nacosToServiceInstances(serviceName, instances), nil
}

// Watch returns a channel that emits the full healthy instance list whenever
// the service changes. The channel is closed when ctx is cancelled.
// The first emit is the current healthy state.
func (r *NacosRegistry) Watch(ctx context.Context, serviceName string) (<-chan []*ServiceInstance, error) {
	ch := make(chan []*ServiceInstance, 8)

	// Emit current state immediately (best-effort; ignore errors on first load).
	if instances, err := r.Discover(ctx, serviceName); err == nil {
		ch <- instances
	}

	// callback must be captured so we can pass the same pointer to Unsubscribe.
	var callback func(services []model.Instance, err error)
	callback = func(services []model.Instance, err error) {
		if err != nil || ctx.Err() != nil {
			return
		}
		updated := nacosToServiceInstances(serviceName, services)
		select {
		case ch <- updated:
		default: // drop if consumer is slow; they will receive the next update
		}
	}

	if err := r.client.Subscribe(&vo.SubscribeParam{
		ServiceName:       serviceName,
		GroupName:         r.group,
		SubscribeCallback: callback,
	}); err != nil {
		close(ch)
		return nil, fmt.Errorf("discovery/nacos: watch %q: %w", serviceName, err)
	}

	go func() {
		<-ctx.Done()
		_ = r.client.Unsubscribe(&vo.SubscribeParam{
			ServiceName:       serviceName,
			GroupName:         r.group,
			SubscribeCallback: callback,
		})
		close(ch)
	}()

	return ch, nil
}

// Close is a no-op for the Nacos registry.
// The underlying naming client lifecycle is managed by the caller.
func (r *NacosRegistry) Close() error { return nil }

// ─── helpers ──────────────────────────────────────────────────────────────────

func nacosToServiceInstances(serviceName string, hosts []model.Instance) []*ServiceInstance {
	result := make([]*ServiceInstance, 0, len(hosts))
	for _, h := range hosts {
		if !h.Enable || !h.Healthy {
			continue
		}
		id := h.Metadata[nacosMetaKeyID]
		if id == "" {
			id = fmt.Sprintf("%s:%d", h.Ip, h.Port)
		}
		scheme := h.Metadata[nacosMetaKeyScheme]
		if scheme == "" {
			scheme = "http"
		}
		meta := make(map[string]string, len(h.Metadata))
		for k, v := range h.Metadata {
			if k != nacosMetaKeyID && k != nacosMetaKeyScheme {
				meta[k] = v
			}
		}
		result = append(result, &ServiceInstance{
			ID:       id,
			Name:     serviceName,
			Address:  fmt.Sprintf("%s:%d", h.Ip, h.Port),
			Scheme:   scheme,
			Weight:   int(h.Weight),
			Metadata: meta,
		})
	}
	return result
}

func splitHostPortNacos(address string) (string, uint64, error) {
	host, portStr, err := net.SplitHostPort(address)
	if err != nil {
		return "", 0, err
	}
	port, err := strconv.ParseUint(portStr, 10, 64)
	if err != nil {
		return "", 0, fmt.Errorf("invalid port %q: %w", portStr, err)
	}
	return host, port, nil
}

func cloneMetaNacos(src map[string]string) map[string]string {
	dst := make(map[string]string, len(src)+2)
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
