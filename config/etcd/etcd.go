// Package etcd provides an etcd-backed configuration source for Astra.
//
// # Usage
//
//	cli, _ := clientv3.New(clientv3.Config{Endpoints: []string{"localhost:2379"}})
//	src := etcd.NewSource(cli, "/myapp/config", config.JSONFormat)
//	cfg, _ := config.New(src)
//	cfg.StartWatch(ctx) // auto-reload on etcd key change
package etcd

import (
	"context"
	"fmt"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/astra-go/astra/config"
)

// Source reads configuration from a single etcd key.
// The key value must be a JSON or YAML document.
//
// It implements both config.Source (for initial load) and config.Watchable (for hot reload).
type Source struct {
	client *clientv3.Client
	key    string
	format config.Format
}

// NewSource creates a Source for the given etcd key.
func NewSource(client *clientv3.Client, key string, format config.Format) *Source {
	return &Source{client: client, key: key, format: format}
}

func (s *Source) Name() string { return "etcd:" + s.key }

// Load fetches the current value of the etcd key.
func (s *Source) Load() (map[string]any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := s.client.Get(ctx, s.key)
	if err != nil {
		return nil, fmt.Errorf("etcd: get %s: %w", s.key, err)
	}
	if len(resp.Kvs) == 0 {
		return make(map[string]any), nil
	}
	return config.ParseRemoteValue(resp.Kvs[0].Value, s.format)
}

// Watch watches the etcd key for changes and calls notify when it changes.
// Runs until ctx is cancelled.
func (s *Source) Watch(ctx context.Context, notify func()) error {
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

// Compile-time assertions.
var _ config.Source = (*Source)(nil)
var _ config.Watchable = (*Source)(nil)
