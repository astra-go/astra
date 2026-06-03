// Package apollo provides backward compatibility for the old config/apollo package.
//
// Deprecated: Use github.com/astra-go/astra/config.NewApolloClient instead.
// This package will be removed in v2.3.0 (November 2026).
//
// # Migration Guide
//
//  1. Update import:
//     - import "github.com/astra-go/astra/config/apollo" // old
//     + import "github.com/astra-go/astra/config"          // new
//
//  2. Update constructor:
//     - src, err := apollo.New(apollo.Config{...})
//     + client, err := config.NewApolloClient(config.ApolloOptions{...})
//
//  3. Update options:
//     - apollo.Options{...}
//     + config.ApolloOptions{...}
//
// For detailed migration steps, see: docs/config/migration-guide.md
package apollo

import (
	"fmt"
	"log"
	"os"
	"sync"

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
║  WARNING: config/apollo is deprecated and will be removed in v2.3.0  ║
║                                                                   ║
║  Migration steps:                                                  ║
║    1. import "github.com/astra-go/astra/config"                   ║
║    2. Use config.NewApolloClient(config.ApolloOptions{...})       ║
║    3. See docs/config/migration-guide.md for details               ║
║                                                                   ║
║  This package will be removed on 2026-11-03.                     ║
╚══════════════════════════════════════════════════════════════╝`
		fmt.Fprint(os.Stderr, msg)
		log.Println("[DEPRECATED] config/apollo is deprecated. See warning on stderr.")
	})
}

// Config is the old configuration struct. Use astraconfig.ApolloOptions instead.
//
// Deprecated: Use astraconfig.ApolloOptions.
type Config struct {
	AppID         string
	Cluster       string
	NamespaceName string
	MetaAddr      string
}

// Options is the old options struct. Use astraconfig.ApolloOptions instead.
//
// Deprecated: Use astraconfig.ApolloOptions.
type Options = astraconfig.ApolloOptions

// Source is the old source struct. Use astraconfig.ConfigClient instead.
//
// Deprecated: Use astraconfig.ConfigClient returned by config.NewApolloClient.
type Source struct {
	client astraconfig.ConfigClient
}

// New creates an Apollo config source.
//
// Deprecated: Use astraconfig.NewApolloClient instead.
func New(cfg Config) (*Source, error) {
	warnDeprecation()

	// Convert old Config to new ApolloOptions
	opts := astraconfig.ApolloOptions{
		AppID:         cfg.AppID,
		Cluster:       cfg.Cluster,
		NamespaceName: cfg.NamespaceName,
		MetaAddr:      cfg.MetaAddr,
		EnableWatch:   true,
	}

	newClient, err := astraconfig.NewApolloClient(opts)
	if err != nil {
		return nil, fmt.Errorf("config/apollo.New: %w", err)
	}

	return &Source{client: newClient}, nil
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
	return fmt.Sprintf("apollo:%s/%s", "default", "application")
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
	log.Println("[DEPRECATED] config/apollo.Source.Watch: Use config.ConfigClient.Watch instead")
	return nil
}

// Close closes the client.
//
// Deprecated: Use astraconfig.ConfigClient.Close instead.
func (s *Source) Close() error {
	warnDeprecation()
	return s.client.Close()
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
	return c.source.Close()
}
