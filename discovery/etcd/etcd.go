// Package etcd provides a service registry backed by etcd v3.
//
// Usage:
//
//	cli, err := clientv3.New(clientv3.Config{Endpoints: []string{"localhost:2379"}})
//	if err != nil { ... }
//	reg := etcd.New(cli, "/services")
//	defer reg.Close()
//
//	_ = reg.Register(ctx, &discovery.ServiceInstance{
//	    ID: "user-svc-1", Name: "user-svc", Address: "10.0.0.1:8081",
//	})
package etcd

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/astra-go/astra/discovery"
	clientv3 "go.etcd.io/etcd/client/v3"
)

const defaultTTL = 30 // seconds

// Registry implements discovery.Registry using etcd v3.
// Instances are stored as JSON under "<prefix>/<serviceName>/<instanceID>".
// A keepalive lease ensures entries expire automatically if the process dies.
type Registry struct {
	client   *clientv3.Client
	prefix   string
	mu       sync.Mutex
	leaseIDs map[string]clientv3.LeaseID // instanceID → lease
}

// New creates an etcd-backed registry.
// prefix is the etcd key prefix (e.g. "/services"). Defaults to "/services".
func New(client *clientv3.Client, prefix string) *Registry {
	if prefix == "" {
		prefix = "/services"
	}
	return &Registry{
		client:   client,
		prefix:   prefix,
		leaseIDs: make(map[string]clientv3.LeaseID),
	}
}

func (r *Registry) instanceKey(instance *discovery.ServiceInstance) string {
	return fmt.Sprintf("%s/%s/%s", r.prefix, instance.Name, instance.ID)
}

// Register writes the instance to etcd with a TTL lease and starts a keepalive loop.
func (r *Registry) Register(ctx context.Context, instance *discovery.ServiceInstance) error {
	if instance.ID == "" {
		return discovery.ErrInstanceIDEmpty
	}
	if instance.Weight <= 0 {
		instance.Weight = 1
	}
	if instance.Scheme == "" {
		instance.Scheme = "http"
	}

	data, err := json.Marshal(instance)
	if err != nil {
		return fmt.Errorf("etcd: marshal instance: %w", err)
	}

	// Grant a TTL lease.
	lease, err := r.client.Grant(ctx, defaultTTL)
	if err != nil {
		return fmt.Errorf("etcd: grant lease: %w", err)
	}

	_, err = r.client.Put(ctx, r.instanceKey(instance), string(data),
		clientv3.WithLease(lease.ID))
	if err != nil {
		return fmt.Errorf("etcd: put instance: %w", err)
	}

	r.mu.Lock()
	r.leaseIDs[instance.ID] = lease.ID
	r.mu.Unlock()

	// Background keepalive — runs until the client is closed or the context is cancelled.
	go func() {
		kaCh, err := r.client.KeepAlive(context.Background(), lease.ID)
		if err != nil {
			return
		}
		for range kaCh {
			// Drain responses; KeepAlive stops automatically when the client closes.
		}
	}()

	return nil
}

// Deregister revokes the lease and removes the instance key.
func (r *Registry) Deregister(ctx context.Context, instanceID string) error {
	r.mu.Lock()
	leaseID, ok := r.leaseIDs[instanceID]
	if ok {
		delete(r.leaseIDs, instanceID)
	}
	r.mu.Unlock()

	if ok {
		if _, err := r.client.Revoke(ctx, leaseID); err != nil {
			return fmt.Errorf("etcd: revoke lease: %w", err)
		}
	}
	return nil
}

// Discover returns all instances registered under "<prefix>/<serviceName>/".
func (r *Registry) Discover(ctx context.Context, serviceName string) ([]*discovery.ServiceInstance, error) {
	prefix := fmt.Sprintf("%s/%s/", r.prefix, serviceName)
	resp, err := r.client.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("etcd: get instances: %w", err)
	}

	result := make([]*discovery.ServiceInstance, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		var inst discovery.ServiceInstance
		if err := json.Unmarshal(kv.Value, &inst); err != nil {
			continue // skip malformed entries
		}
		result = append(result, &inst)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("%w: %s", discovery.ErrNotFound, serviceName)
	}
	return result, nil
}

// Watch returns a channel that emits updated instance lists when keys change.
func (r *Registry) Watch(ctx context.Context, serviceName string) (<-chan []*discovery.ServiceInstance, error) {
	ch := make(chan []*discovery.ServiceInstance, 8)
	prefix := fmt.Sprintf("%s/%s/", r.prefix, serviceName)

	// Emit current state.
	if instances, err := r.Discover(ctx, serviceName); err == nil {
		select {
		case ch <- instances:
		default:
		}
	}

	watchCh := r.client.Watch(ctx, prefix, clientv3.WithPrefix())
	go func() {
		defer close(ch)
		for {
			select {
			case <-ctx.Done():
				return
			case resp, ok := <-watchCh:
				if !ok || resp.Err() != nil {
					return
				}
				// Any change under this prefix triggers a full re-read.
				instances, err := r.Discover(context.Background(), serviceName)
				if err != nil {
					instances = nil // service disappeared
				}
				select {
				case ch <- instances:
				case <-ctx.Done():
					return
				default:
				}
			}
		}
	}()

	return ch, nil
}

// Close closes the underlying etcd client.
func (r *Registry) Close() error {
	return r.client.Close()
}

// NewClient is a convenience helper that creates an etcd client.
func NewClient(endpoints []string, timeout time.Duration) (*clientv3.Client, error) {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: timeout,
	})
}
