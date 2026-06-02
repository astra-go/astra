// Package config - Etcd client implementation
//
// # Usage
//
//	import "github.com/astra-go/astra/config"
//
//	client, err := config.NewEtcdClient(config.EtcdOptions{
//	    Endpoints:      []string{"localhost:2379"},
//	    DialTimeout:    5 * time.Second,
//	    KeyPrefix:      "/myapp/config",
//	    WatchEnabled:   true,
//	})
//
//	value, _ := client.Get("db.host")
//	client.Watch(ctx, "db.host", func(newValue string) {
//	    log.Printf("db.host changed to: %s", newValue)
//	})
package config

import (
	"context"
	"fmt"
	"strconv"
	"log/slog"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// ─── Etcd Options ─────────────────────────────────────────────────────

// EtcdOptions configures an Etcd configuration client.
type EtcdOptions struct {
	Options // Embed common options

	// Endpoints is the list of Etcd server endpoints.
	// Required. Example: []string{"localhost:2379", "localhost:2380"}
	Endpoints []string

	// DialTimeout is the timeout for the initial dial.
	// Default: 5s
	DialTimeout time.Duration

	// Username for Etcd authentication (optional).
	Username string

	// Password for Etcd authentication (optional).
	Password string

	// KeyPrefix is the prefix for all configuration keys in Etcd.
	// Default: "/config"
	KeyPrefix string

	// WatchEnabled enables watching for configuration changes.
	// Default: true
	WatchEnabled bool

	// AutoSyncInterval is the interval for auto-syncing the endpoint list.
	// Default: 0 (disabled)
	AutoSyncInterval time.Duration
}

// DefaultEtcdOptions returns the default Etcd options.
func DefaultEtcdOptions() EtcdOptions {
	return EtcdOptions{
		Options:          DefaultOptions(),
		Endpoints:        []string{"localhost:2379"},
		DialTimeout:      5 * time.Second,
		KeyPrefix:        "/config",
		WatchEnabled:     true,
		AutoSyncInterval: 0,
	}
}

// ─── Etcd Client ─────────────────────────────────────────────────────

// EtcdClient implements ConfigClient for Etcd.
type EtcdClient struct {
	client        *clientv3.Client
	opts          EtcdOptions
	cache         map[string]string
	watchCancel   context.CancelFunc
}

// NewEtcdClient creates a new Etcd configuration client.
// This is the type-safe constructor (recommended).
func NewEtcdClient(opts EtcdOptions) (ConfigClient, error) {
	if len(opts.Endpoints) == 0 {
		return nil, fmt.Errorf("config/etcd: Endpoints is required")
	}

	// Apply defaults
	if opts.DialTimeout == 0 {
		opts.DialTimeout = 5 * time.Second
	}
	if opts.KeyPrefix == "" {
		opts.KeyPrefix = "/config"
	}

	// Create Etcd client
	clientCfg := clientv3.Config{
		Endpoints:        opts.Endpoints,
		DialTimeout:      opts.DialTimeout,
		Username:          opts.Username,
		Password:          opts.Password,
		AutoSyncInterval: opts.AutoSyncInterval,
	}

	client, err := clientv3.New(clientCfg)
	if err != nil {
		return nil, fmt.Errorf("config/etcd: create client: %w", err)
	}

	ec := &EtcdClient{
		client: client,
		opts:   opts,
		cache:  make(map[string]string),
	}

	// Initial load
	if opts.EnableCache {
		if err := ec.refreshCache(context.Background()); err != nil {
			slog.Warn("config/etcd: initial cache refresh failed", "error", err)
		}
	}

	return ec, nil
}

// ─── ConfigClient Interface ──────────────────────────────────────────

// Get retrieves a configuration value by key.
func (c *EtcdClient) Get(key string) (string, error) {
	if c.opts.EnableCache {
		cached, ok := c.cache[key]
		if ok {
			return cached, nil
		}
	}

	// Build Etcd key
	etcdKey := c.buildKey(key)

	// Get from Etcd
	ctx, cancel := context.WithTimeout(context.Background(), c.opts.Timeout)
	defer cancel()

	resp, err := c.client.Get(ctx, etcdKey)
	if err != nil {
		return "", fmt.Errorf("config/etcd: get %s: %w", etcdKey, err)
	}

	if len(resp.Kvs) == 0 {
		return "", ErrKeyNotFound
	}

	value := string(resp.Kvs[0].Value)

	if c.opts.EnableCache {
		c.cache[key] = value
	}

	return value, nil
}

// GetWithDefault retrieves a configuration value, returning a default if not found.
func (c *EtcdClient) GetWithDefault(key string, defaultValue string) string {
	value, err := c.Get(key)
	if err != nil {
		return defaultValue
	}
	return value
}

// GetInt retrieves an integer configuration value.
func (c *EtcdClient) GetInt(key string) (int, error) {
	value, err := c.Get(key)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(value)
}

// GetBool retrieves a boolean configuration value.
func (c *EtcdClient) GetBool(key string) (bool, error) {
	value, err := c.Get(key)
	if err != nil {
		return false, err
	}
	return parseBool(value)
}

// GetAll returns all configuration key-value pairs.
func (c *EtcdClient) GetAll() (map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.opts.Timeout)
	defer cancel()

	resp, err := c.client.Get(ctx, c.opts.KeyPrefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("config/etcd: get all: %w", err)
	}

	result := make(map[string]string)
	for _, kv := range resp.Kvs {
		key := c.stripPrefix(string(kv.Key))
		value := string(kv.Value)
		result[key] = value
	}

	return result, nil
}

// Watch starts watching for configuration changes.
func (c *EtcdClient) Watch(ctx context.Context, key string, callback func(newValue string)) error {
	if !c.opts.WatchEnabled {
		return ErrWatchNotSupported
	}

	// Build Etcd key
	etcdKey := c.buildKey(key)

	// Create cancellable context
	watchCtx, cancel := context.WithCancel(ctx)
	c.watchCancel = cancel

	// Start watching
	watchCh := c.client.Watch(watchCtx, etcdKey)

	go func() {
		for {
			select {
			case <-watchCtx.Done():
				return
			case resp, ok := <-watchCh:
				if !ok {
					return
				}
				if resp.Err() != nil {
					slog.Error("config/etcd: watch error", "error", resp.Err())
					continue
				}

				// Process events
				for _, event := range resp.Events {
					if event.Type == clientv3.EventTypePut {
						newValue := string(event.Kv.Value)

						// Update cache
						if c.opts.EnableCache {
							c.cache[key] = newValue
						}

						// Call user callback
						callback(newValue)
					} else if event.Type == clientv3.EventTypeDelete {
						// Key deleted
						if c.opts.EnableCache {
							delete(c.cache, key)
						}
						callback("")
					}
				}
			}
		}
	}()

	return nil
}

// Close closes the client and releases resources.
func (c *EtcdClient) Close() error {
	if c.watchCancel != nil {
		c.watchCancel()
	}
	return c.client.Close()
}

// ─── Internal Methods ────────────────────────────────────────────────

// refreshCache reloads all configuration into cache.
func (c *EtcdClient) refreshCache(ctx context.Context) error {
	resp, err := c.client.Get(ctx, c.opts.KeyPrefix, clientv3.WithPrefix())
	if err != nil {
		return err
	}

	c.cache = make(map[string]string)
	for _, kv := range resp.Kvs {
		key := c.stripPrefix(string(kv.Key))
		value := string(kv.Value)
		c.cache[key] = value
	}

	return nil
}

// buildKey builds the full Etcd key with prefix.
func (c *EtcdClient) buildKey(key string) string {
	return c.opts.KeyPrefix + "/" + key
}

// stripPrefix removes the prefix from an Etcd key.
func (c *EtcdClient) stripPrefix(etcdKey string) string {
	return strings.TrimPrefix(etcdKey, c.opts.KeyPrefix+"/")
}

// Compile-time assertion.
var _ ConfigClient = (*EtcdClient)(nil)
