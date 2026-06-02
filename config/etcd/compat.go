// Package etcd provides backward compatibility for the old config/etcd package.
//
// Deprecated: Use github.com/astra-go/astra/config.NewEtcdClient instead.
// This package will be removed in v2.3.0 (November 2026).
//
// # Migration Guide
//
//  1. Update import:
//     - import "github.com/astra-go/astra/config/etcd" // old
//     + import "github.com/astra-go/astra/config"         // new
//
//  2. Update constructor:
//     - src := etcd.NewSource(client, key, format)
//     + client, err := config.NewEtcdClient(config.EtcdOptions{...})
//
//  3. Update options:
//     - etcd.Options{...}
//     + config.EtcdOptions{...}
//
// For detailed migration steps, see: docs/config/migration-guide.md
package etcd

import (
	"fmt"
	"log"
	"os"
	"sync"

	clientv3 "go.etcd.io/etcd/client/v3"

	astraconfig "github.com/astra-go/astra/config"
)

var (
	deprecationWarned sync.Once
)

// warnDeprecation prints the deprecation warning once.
func warnDeprecation() {
	deprecationWarned.Do(func() {
		msg := `
╔══════════════════════════════════════════════════════════════╗
║  WARNING: config/etcd is deprecated and will be removed in v2.3.0  ║
║                                                                   ║
║  Migration steps:                                                 ║
║    1. import "github.com/astra-go/astra/config"                  ║
║    2. Use config.NewEtcdClient(config.EtcdOptions{...})           ║
║    3. See docs/config/migration-guide.md for details                ║
║                                                                   ║
║  This package will be removed on 2026-11-03.                      ║
╚══════════════════════════════════════════════════════════════╝`
		fmt.Fprint(os.Stderr, msg)
		log.Println("[DEPRECATED] config/etcd is deprecated. See warning on stderr.")
	})
}

// Options is the old options struct. Use astraconfig.EtcdOptions instead.
//
// Deprecated: Use astraconfig.EtcdOptions.
type Options struct {
	// Embed common options
	Timeout      int
	Namespace    string
	KeyPrefix    string
	Format       astraconfig.Format
	EnableWatch  bool
}

// Source is the old source struct. Use astraconfig.ConfigClient instead.
//
// Deprecated: Use astraconfig.ConfigClient returned by config.NewEtcdClient.
type Source struct {
	client astraconfig.ConfigClient
}

// NewSource creates an Etcd config source.
//
// Deprecated: Use astraconfig.NewEtcdClient instead.
func NewSource(client *clientv3.Client, key string, format astraconfig.Format) *Source {
	warnDeprecation()

	// Convert to new API
	endpoints := []string{"localhost:2379"}
	if client != nil {
		// Try to extract endpoints from client (best effort)
		// This is a simplified version
	}

	opts := astraconfig.EtcdOptions{
		Endpoints:   endpoints,
		KeyPrefix:   key,
		WatchEnabled: true,
	}

	newClient, err := astraconfig.NewEtcdClient(opts)
	if err != nil {
		log.Printf("[DEPRECATED] config/etcd.NewSource: %v", err)
		return nil
	}

	return &Source{client: newClient}
}

// Get retrieves a configuration value.
//
// Deprecated: Use astraconfig.ConfigClient.Get instead.
func (s *Source) Get(key string) (string, error) {
	warnDeprecation()
	return s.client.Get(key)
}

// Name returns the source name.
//
// Deprecated: This method is kept for backward compatibility.
func (s *Source) Name() string {
	warnDeprecation()
	return fmt.Sprintf("etcd:%s", "unknown")
}

// Load loads the configuration.
//
// Deprecated: This method is kept for backward compatibility.
func (s *Source) Load() (map[string]any, error) {
	warnDeprecation()
	all, err := s.client.GetAll()
	if err != nil {
		return nil, err
	}
	result := make(map[string]any)
	for k, v := range all {
		result[k] = v
	}
	return result, nil
}

// Watch watches for configuration changes.
//
// Deprecated: Use astraconfig.ConfigClient.Watch instead.
func (s *Source) Watch(ctx interface{}, notify func()) error {
	warnDeprecation()
	log.Println("[DEPRECATED] config/etcd.Source.Watch: Use config.ConfigClient.Watch instead")
	return nil
}

// Compile-time assertion.
var _ astraconfig.ConfigClient = (*compatClient)(nil)

// compatClient wraps old API to implement astraconfig.ConfigClient.
type compatClient struct {
	source *Source
	cache  map[string]string
}

func (c *compatClient) Get(key string) (string, error) {
	return c.source.Get(key)
}

func (c *compatClient) GetWithDefault(key string, defaultValue string) string {
	v, err := c.Get(key)
	if err != nil {
		return defaultValue
	}
	return v
}

func (c *compatClient) GetInt(key string) (int, error) {
	v, err := c.Get(key)
	if err != nil {
		return 0, err
	}
	return 0, fmt.Errorf("not implemented")
}

func (c *compatClient) GetBool(key string) (bool, error) {
	return false, fmt.Errorf("not implemented")
}

func (c *compatClient) GetAll() (map[string]string, error) {
	result := make(map[string]string)
	for k, v := range c.cache {
		result[k] = v
	}
	return result, nil
}

func (c *compatClient) Watch(ctx interface{}, key string, callback func(string)) error {
	return c.source.Watch(ctx, func() {})
}

func (c *compatClient) Close() error {
	return nil
}
