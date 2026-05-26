// Package config provides remote configuration sources for Astra.
//
// # etcd source
//
//	cli, _ := clientv3.New(clientv3.Config{Endpoints: []string{"localhost:2379"}})
//	src := config.NewEtcdSource(cli, "/myapp/config", config.JSONFormat)
//	cfg, _ := config.New(src)
//	cfg.StartWatch(ctx) // auto-reload on etcd key change
//
// # Consul KV source
//
//	consulCli, _ := api.NewClient(api.DefaultConfig())
//	src := config.NewConsulKVSource(consulCli, "myapp/config", config.YAMLFormat)
//	cfg, _ := config.New(src)
//	cfg.StartWatch(ctx)
package config

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hashicorp/consul/api"
	clientv3 "go.etcd.io/etcd/client/v3"
	"gopkg.in/yaml.v3"
)

// Format describes how a remote config value is encoded.
type Format int

const (
	// JSONFormat indicates the remote value is a JSON document.
	JSONFormat Format = iota
	// YAMLFormat indicates the remote value is a YAML document.
	YAMLFormat
)

// ─── etcd source ─────────────────────────────────────────────────────────────

// EtcdSource reads configuration from a single etcd key.
// The key value must be a JSON or YAML document.
//
// It implements both Source (for initial load) and Watchable (for hot reload).
type EtcdSource struct {
	client *clientv3.Client
	key    string
	format Format
}

// NewEtcdSource creates an EtcdSource for the given etcd key.
func NewEtcdSource(client *clientv3.Client, key string, format Format) *EtcdSource {
	return &EtcdSource{client: client, key: key, format: format}
}

func (s *EtcdSource) Name() string { return "etcd:" + s.key }

// Load fetches the current value of the etcd key.
func (s *EtcdSource) Load() (map[string]any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := s.client.Get(ctx, s.key)
	if err != nil {
		return nil, fmt.Errorf("etcd: get %s: %w", s.key, err)
	}
	if len(resp.Kvs) == 0 {
		return make(map[string]any), nil // key absent → empty config
	}
	return parseRemoteValue(resp.Kvs[0].Value, s.format)
}

// Watch watches the etcd key for changes and calls notify when it changes.
// Runs until ctx is cancelled.
func (s *EtcdSource) Watch(ctx context.Context, notify func()) error {
	watchCh := s.client.Watch(ctx, s.key)
	for {
		select {
		case <-ctx.Done():
			return nil
		case resp, ok := <-watchCh:
			if !ok {
				return nil
			}
			if resp.Err() != nil {
				return fmt.Errorf("etcd: watch %s: %w", s.key, resp.Err())
			}
			if len(resp.Events) > 0 {
				notify()
			}
		}
	}
}

// ─── Consul KV source ─────────────────────────────────────────────────────────

// ConsulKVSource reads configuration from a Consul KV path.
// The value at the path must be a JSON or YAML document.
//
// It implements both Source and Watchable.
type ConsulKVSource struct {
	client *api.Client
	path   string
	format Format
}

// NewConsulKVSource creates a ConsulKVSource for the given Consul KV path.
func NewConsulKVSource(client *api.Client, path string, format Format) *ConsulKVSource {
	return &ConsulKVSource{client: client, path: path, format: format}
}

func (s *ConsulKVSource) Name() string { return "consul-kv:" + s.path }

// Load fetches the current value at the Consul KV path.
func (s *ConsulKVSource) Load() (map[string]any, error) {
	kv, _, err := s.client.KV().Get(s.path, nil)
	if err != nil {
		return nil, fmt.Errorf("consul: kv get %s: %w", s.path, err)
	}
	if kv == nil {
		return make(map[string]any), nil // key absent → empty config
	}
	return parseRemoteValue(kv.Value, s.format)
}

// Watch watches the Consul KV path using blocking queries.
// Runs until ctx is cancelled.
func (s *ConsulKVSource) Watch(ctx context.Context, notify func()) error {
	var lastIndex uint64
	for {
		if ctx.Err() != nil {
			return nil
		}
		kv, meta, err := s.client.KV().Get(s.path, &api.QueryOptions{
			WaitIndex: lastIndex,
			WaitTime:  30 * time.Second,
		})
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(2 * time.Second):
				continue
			}
		}
		if meta == nil {
			continue
		}
		if meta.LastIndex != lastIndex {
			lastIndex = meta.LastIndex
			if kv != nil {
				notify()
			}
		}
	}
}

// ─── Shared helpers ───────────────────────────────────────────────────────────

// parseRemoteValue parses raw bytes as JSON or YAML into a map.
func parseRemoteValue(data []byte, format Format) (map[string]any, error) {
	var result map[string]any
	switch format {
	case JSONFormat:
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.UseNumber()
		if err := dec.Decode(&result); err != nil {
			return nil, fmt.Errorf("remote config: parse JSON: %w", err)
		}
	case YAMLFormat:
		if err := yaml.Unmarshal(data, &result); err != nil {
			return nil, fmt.Errorf("remote config: parse YAML: %w", err)
		}
	default:
		return nil, fmt.Errorf("remote config: unknown format %d", format)
	}
	return result, nil
}
