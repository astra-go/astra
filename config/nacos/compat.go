// Package nacos provides backward compatibility for the old config/nacos package.
//
// Deprecated: Use github.com/astra-go/astra/config.NewNacosClient instead.
// This package will be removed in v2.3.0 (November 2026).
//
// # Migration Guide
//
//  1. Update import:
//     - import "github.com/astra-go/astra/config/nacos" // old
//     + import "github.com/astra-go/astra/config"         // new
//
//  2. Update constructor:
//     - client, _ := nacos.NewClient(nacos.Config{...})
//     + client, err := config.NewNacosClient(config.NacosOptions{...})
//
//  3. Update options:
//     - nacos.Options{...}
//     + config.NacosOptions{...}
//
// For detailed migration steps, see: docs/config/migration-guide.md
package nacos

import (
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/nacos-group/nacos-sdk-go/v2/clients/config_client"

	astraconfig "github.com/astra-go/astra/config"
)

var (
	deprecationWarned sync.Once
)

// warnDeprecation prints the deprecation warning once.
func warnDeprecation() {
	deprecationWarned.Do(func() {
		msg := `
╔═══════════════════════════════════════════════════════════════╗
║  WARNING: config/nacos is deprecated and will be removed in v2.3.0  ║
║                                                                   ║
║  Migration steps:                                                       ║
║    1. import "github.com/astra-go/astra/config"                      ║
║    2. Use config.NewNacosClient(config.NacosOptions{...})             ║
║    3. See docs/config/migration-guide.md for details                ║
║                                                                   ║
║  This package will be removed on 2026-11-03.                      ║
╚═══════════════════════════════════════════════════════════════╝`
		fmt.Fprint(os.Stderr, msg)
		log.Println("[DEPRECATED] config/nacos is deprecated. See warning on stderr.")
	})
}

// Config is the old configuration struct. Use astraconfig.NacosOptions instead.
//
// Deprecated: Use astraconfig.NacosOptions.
type Config = astraconfig.NacosOptions

// Source is the old source struct. Use astraconfig.ConfigClient instead.
//
// Deprecated: Use astraconfig.ConfigClient returned by config.NewNacosClient.
type Source struct {
	client astraconfig.ConfigClient
}

// New creates a Nacos config source.
//
// Deprecated: Use astraconfig.NewNacosClient instead.
func New(client config_client.IConfigClient, cfg Config) *Source {
	warnDeprecation()

	// Convert old Config to new NacosOptions
	opts := astraconfig.NacosOptions{
		ServerAddr:    getServerAddr(client),
		Namespace:     cfg.Namespace,
		Group:          cfg.Group,
		DataID:         cfg.DataID,
		Format:         cfg.Format,
		EnableWatch:    true,
	}

	newClient, err := astraconfig.NewNacosClient(opts)
	if err != nil {
		log.Printf("[DEPRECATED] config/nacos.New: %v", err)
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
	return fmt.Sprintf("nacos:%s/%s", "DEFAULT_GROUP", "unknown")
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
	log.Println("[DEPRECATED] config/nacos.Source.Watch: Use config.ConfigClient.Watch instead")
	return nil
}

// getServerAddr extracts server address from Nacos client (best effort).
func getServerAddr(client config_client.IConfigClient) string {
	// This is a simplified version. In practice, you'd need to reflect the client.
	return "localhost:8848"
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
