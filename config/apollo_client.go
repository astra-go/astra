// Package config - Apollo client implementation
//
// # Prerequisites
//
// Apollo Meta Server must be running and reachable:
//
//	docker run -p 8080:8080 apolloconfig/apollo-quick-start
//
// # Usage
//
//	import "github.com/astra-go/astra/config"
//
//	client, err := config.NewApolloClient(config.ApolloOptions{
//	    AppID:         "my-service",
//	    MetaAddr:      "http://localhost:8080",
//	    NamespaceName: "application",
//	    Cluster:       "default",
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
	"strings"

	"github.com/apolloconfig/agollo/v4"
	"github.com/apolloconfig/agollo/v4/env/config"
	"github.com/apolloconfig/agollo/v4/storage"
)

// ─── Apollo Options ─────────────────────────────────────────────

// ApolloOptions configures an Apollo configuration client.
type ApolloOptions struct {
	Options // Embed common options

	// AppID is the Apollo application identifier.
	// Required.
	AppID string

	// Cluster is the Apollo cluster name.
	// Default: "default"
	Cluster string

	// NamespaceName is the Apollo namespace to load.
	// Default: "application"
	NamespaceName string

	// MetaAddr is the Apollo Meta Server address.
	// Required. Example: "http://localhost:8080"
	MetaAddr string

	// BackupConfigPath is the directory for local config file backup.
	// Apollo SDK uses this as a fallback when the server is unreachable.
	// Default: "./config"
	BackupConfigPath string

	// EnableWatch enables watching for configuration changes.
	// Default: true
	EnableWatch bool
}

// DefaultApolloOptions returns the default Apollo options.
func DefaultApolloOptions() ApolloOptions {
	return ApolloOptions{
		Options:           DefaultOptions(),
		Cluster:           "default",
		NamespaceName:     "application",
		BackupConfigPath:  "./config",
		EnableWatch:       true,
	}
}

// ─── Apollo Client ──────────────────────────────────────────────

// ApolloClient implements ConfigClient for Apollo.
type ApolloClient struct {
	opts    ApolloOptions
	client  agollo.Client
	cache   map[string]string
	nsCache map[string]*storage.Config
}

// NewApolloClient creates a new Apollo configuration client.
// This is the type-safe constructor (recommended).
func NewApolloClient(opts ApolloOptions) (ConfigClient, error) {
	if opts.AppID == "" {
		return nil, fmt.Errorf("config/apollo: AppID is required")
	}
	if opts.MetaAddr == "" {
		return nil, fmt.Errorf("config/apollo: MetaAddr is required")
	}

	// Apply defaults
	if opts.Cluster == "" {
		opts.Cluster = "default"
	}
	if opts.NamespaceName == "" {
		opts.NamespaceName = "application"
	}
	if opts.BackupConfigPath == "" {
		opts.BackupConfigPath = "./config"
	}

	// Create Apollo client
	appCfg := &config.AppConfig{
		AppID:            opts.AppID,
		Cluster:          opts.Cluster,
		NamespaceName:    opts.NamespaceName,
		IP:               opts.MetaAddr,
		BackupConfigPath: opts.BackupConfigPath,
	}

	client, err := agollo.StartWithConfig(func() (*config.AppConfig, error) {
		return appCfg, nil
	})
	if err != nil {
		return nil, fmt.Errorf("config/apollo: start: %w", err)
	}

	ac := &ApolloClient{
		opts:    opts,
		client:  client,
		cache:   make(map[string]string),
		nsCache: make(map[string]*storage.Config),
	}

	// Initial load
	if opts.EnableCache {
		ac.refreshCache()
	}

	// Start watching if enabled
	if opts.EnableWatch {
		ac.startWatch()
	}

	return ac, nil
}

// ─── ConfigClient Interface ──────────────────────────────────

// Get retrieves a configuration value by key.
func (c *ApolloClient) Get(key string) (string, error) {
	if c.opts.EnableCache {
		cached, ok := c.cache[key]
		if ok {
			return cached, nil
		}
	}

	// Get from Apollo
	ns := c.client.GetConfig(c.opts.NamespaceName)
	if ns == nil {
		return "", ErrKeyNotFound
	}

	value := ns.GetStringValue(key, "")
	if value == "" {
		return "", ErrKeyNotFound
	}

	if c.opts.EnableCache {
		c.cache[key] = value
	}

	return value, nil
}

// GetWithDefault retrieves a configuration value, returning a default if not found.
func (c *ApolloClient) GetWithDefault(key string, defaultValue string) string {
	value, err := c.Get(key)
	if err != nil {
		return defaultValue
	}
	return value
}

// GetInt retrieves an integer configuration value.
func (c *ApolloClient) GetInt(key string) (int, error) {
	value, err := c.Get(key)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(value)
}

// GetBool retrieves a boolean configuration value.
func (c *ApolloClient) GetBool(key string) (bool, error) {
	value, err := c.Get(key)
	if err != nil {
		return false, err
	}
	return parseBool(value)
}

// GetAll returns all configuration key-value pairs.
func (c *ApolloClient) GetAll() (map[string]string, error) {
	ns := c.client.GetConfig(c.opts.NamespaceName)
	if ns == nil {
		return nil, fmt.Errorf("config/apollo: namespace %q not found", c.opts.NamespaceName)
	}

	cache := ns.GetCache()
	if cache == nil {
		return make(map[string]string), nil
	}

	result := make(map[string]string)
	cache.Range(func(key, value any) bool {
		result[fmt.Sprintf("%v", key)] = fmt.Sprintf("%v", value)
		return true
	})

	return result, nil
}

// Watch starts watching for configuration changes.
func (c *ApolloClient) Watch(ctx context.Context, key string, callback func(newValue string)) error {
	if !c.opts.EnableWatch {
		return ErrWatchNotSupported
	}

	// Register change listener
	listener := &apolloChangeListener{
		key:      key,
		callback: callback,
		client:   c,
	}

	c.client.AddChangeListener(listener)

	// Wait for context cancellation
	<-ctx.Done()

	// Remove listener
	c.client.RemoveChangeListener(listener)

	return nil
}

// Close closes the client and releases resources.
// Note: The Apollo SDK doesn't provide an explicit Close() method.
// Resources are released when the process exits.
func (c *ApolloClient) Close() error {
	// Apollo client doesn't have an explicit Close() method
	// Just clear the cache
	c.cache = nil
	c.nsCache = nil
	return nil
}

// ─── Internal Methods ──────────────────────────────────────────

// refreshCache reloads all configuration into cache.
func (c *ApolloClient) refreshCache() {
	ns := c.client.GetConfig(c.opts.NamespaceName)
	if ns == nil {
		return
	}

	cache := ns.GetCache()
	if cache == nil {
		return
	}

	newCache := make(map[string]string)
	cache.Range(func(key, value any) bool {
		k := fmt.Sprintf("%v", key)
		v := fmt.Sprintf("%v", value)
		newCache[k] = v
		return true
	})

	c.cache = newCache
	c.nsCache[c.opts.NamespaceName] = ns
}

// startWatch starts watching for configuration changes.
func (c *ApolloClient) startWatch() {
	listener := &apolloGlobalListener{
		client: c,
	}

	c.client.AddChangeListener(listener)
}

// ─── Helper Functions ─────────────────────────────────────────

func parseBool(s string) (bool, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "true", "1", "yes", "on":
		return true, nil
	case "false", "0", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("config: invalid boolean value %q", s)
	}
}

// ─── Change Listeners ─────────────────────────────────────────

// apolloChangeListener listens for changes to a specific key.
type apolloChangeListener struct {
	key      string
	callback func(newValue string)
	client   *ApolloClient
}

// OnChange implements agollo's ChangeListener.
func (l *apolloChangeListener) OnChange(event *storage.ChangeEvent) {
	if len(event.Changes) == 0 {
		return
	}

	// Check if our key changed
	if change, ok := event.Changes[l.key]; ok {
		newValue := fmt.Sprintf("%v", change.NewValue)
		
		// Update cache
		if l.client.opts.EnableCache {
			l.client.cache[l.key] = newValue
		}

		// Call user callback
		l.callback(newValue)
	}
}

// OnNewestChange implements agollo's ChangeListener.
func (l *apolloChangeListener) OnNewestChange(event *storage.FullChangeEvent) {
	// OnChange already triggered; nothing extra to do.
}

// apolloGlobalListener listens for all changes (used for cache refresh).
type apolloGlobalListener struct {
	client *ApolloClient
}

// OnChange implements agollo's ChangeListener.
func (l *apolloGlobalListener) OnChange(event *storage.ChangeEvent) {
	if len(event.Changes) > 0 {
		// Refresh entire cache
		l.client.refreshCache()
	}
}

// OnNewestChange implements agollo's ChangeListener.
func (l *apolloGlobalListener) OnNewestChange(event *storage.FullChangeEvent) {
	// OnChange already triggered; nothing extra to do.
}

// Compile-time assertion.
var _ ConfigClient = (*ApolloClient)(nil)
