// Package config provides Apollo as a configuration source for Astra.
//
// # Usage
//
//	src := config.NewApolloSource(config.ApolloSourceConfig{
//	    AppID:         "my-service",
//	    MetaAddr:      "http://localhost:8080",
//	    NamespaceName: "application",
//	    Cluster:       "default",
//	    Format:        config.YAMLFormat,
//	})
//	cfg, _ := config.New(src)
//	cfg.StartWatch(ctx)  // Apollo 变更自动触发 Load
//
// # With existing Apollo SDK client
//
//	src := config.NewApolloSourceWithClient(sdkClient, config.ApolloSourceConfig{
//	    AppID:         "my-service",
//	    NamespaceName: "application",
//	    Format:        config.YAMLFormat,
//	})
//	cfg, _ := config.New(src)
//	cfg.StartWatch(ctx)
package config

import (
	"context"
	"fmt"

	"github.com/apolloconfig/agollo/v4"
	"github.com/apolloconfig/agollo/v4/env/config"
	"github.com/apolloconfig/agollo/v4/storage"

)

// ApolloSourceConfig configures an ApolloSource.
type ApolloSourceConfig struct {
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

	// Format specifies the configuration format (YAML or JSON).
	// Default: YAMLFormat
	Format Format
}

// ApolloSource reads configuration from a single Apollo namespace.
// The value must be a JSON or YAML document.
//
// It implements both Source (for initial load) and Watchable (for hot reload).
type ApolloSource struct {
	client agollo.Client
	opts   ApolloSourceConfig
	loaded bool
}

// NewApolloSource creates an ApolloSource with the given config.
// It creates the Apollo SDK client internally.
func NewApolloSource(cfg ApolloSourceConfig) (*ApolloSource, error) {
	if cfg.AppID == "" {
		return nil, fmt.Errorf("config: ApolloSource requires AppID")
	}
	if cfg.MetaAddr == "" {
		return nil, fmt.Errorf("config: ApolloSource requires MetaAddr")
	}

	// Apply defaults
	if cfg.Cluster == "" {
		cfg.Cluster = "default"
	}
	if cfg.NamespaceName == "" {
		cfg.NamespaceName = "application"
	}
	if cfg.BackupConfigPath == "" {
		cfg.BackupConfigPath = "./config"
	}
	if cfg.Format == 0 {
		cfg.Format = YAMLFormat
	}

	// Create Apollo SDK client
	appCfg := &config.AppConfig{
		AppID:            cfg.AppID,
		Cluster:          cfg.Cluster,
		NamespaceName:    cfg.NamespaceName,
		IP:               cfg.MetaAddr,
		BackupConfigPath: cfg.BackupConfigPath,
	}

	sdkClient, err := agollo.StartWithConfig(func() (*config.AppConfig, error) {
		return appCfg, nil
	})
	if err != nil {
		return nil, fmt.Errorf("config: create Apollo SDK client: %w", err)
	}

	return &ApolloSource{
		client: sdkClient,
		opts:   cfg,
	}, nil
}

// NewApolloSourceWithClient creates an ApolloSource with an existing
// Apollo SDK client. Use this when you already have an Apollo SDK
// client and want to wrap it as a config Source.
func NewApolloSourceWithClient(client agollo.Client, cfg ApolloSourceConfig) (*ApolloSource, error) {
	if client == nil {
		return nil, fmt.Errorf("config: Apollo SDK client is required")
	}
	if cfg.AppID == "" {
		return nil, fmt.Errorf("config: ApolloSource requires AppID")
	}

	// Apply defaults
	if cfg.NamespaceName == "" {
		cfg.NamespaceName = "application"
	}
	if cfg.Format == 0 {
		cfg.Format = YAMLFormat
	}

	return &ApolloSource{
		client: client,
		opts:   cfg,
	}, nil
}

func (s *ApolloSource) Name() string {
	return fmt.Sprintf("apollo:%s/%s", s.opts.AppID, s.opts.NamespaceName)
}

// Load fetches the current value of the Apollo namespace.
func (s *ApolloSource) Load() (map[string]any, error) {
	ns := s.client.GetConfig(s.opts.NamespaceName)
	if ns == nil {
		return make(map[string]any), nil // namespace empty
	}

	cache := ns.GetCache()
	if cache == nil {
		return make(map[string]any), nil // empty config
	}

	// Convert Apollo cache to map[string]any
	result := make(map[string]any)
	cache.Range(func(key, value any) bool {
		k := fmt.Sprintf("%v", key)
		v := fmt.Sprintf("%v", value)
		result[k] = v
		return true
	})

	s.loaded = true

	// If format is YAML, convert from flat map to nested map
	if s.opts.Format == YAMLFormat {
		return flattenToNested(result), nil
	}

	return result, nil
}

// Watch watches the Apollo namespace for changes and calls notify when it changes.
// Runs until ctx is cancelled.
func (s *ApolloSource) Watch(ctx context.Context, notify func()) error {
	listener := &apolloSourceListener{
		notify:    notify,
		namespace: s.opts.NamespaceName,
	}

	s.client.AddChangeListener(listener)

	// Block until context is cancelled
	<-ctx.Done()

	// Remove listener
	s.client.RemoveChangeListener(listener)

	return nil
}

// flattenToNested converts a flat map (from Apollo) to a nested map structure.
// Apollo returns flat key-value pairs like "db.host" -> "localhost".
func flattenToNested(flat map[string]any) map[string]any {
	result := make(map[string]any)

	for k, v := range flat {
		keys := splitKey(k)
		setNestedKey(result, keys, v)
	}

	return result
}

// splitKey splits a dot-separated key into parts.
func splitKey(key string) []string {
	if key == "" {
		return nil
	}
	return splitKeyHelper(key, 0)
}

func splitKeyHelper(key string, idx int) []string {
	if idx >= len(key) {
		return []string{}
	}

	for i := idx; i < len(key); i++ {
		if key[i] == '.' {
			head := key[:i]
			tail := splitKeyHelper(key[i+1:], 0)
			result := make([]string, 0, len(tail)+1)
			result = append(result, head)
			result = append(result, tail...)
			return result
		}
	}

	// No dots found
	return []string{key}
}

// apolloSourceListener listens for Apollo configuration changes.
type apolloSourceListener struct {
	notify    func()
	namespace string
}

// OnChange implements agollo's ChangeListener.
func (l *apolloSourceListener) OnChange(event *storage.ChangeEvent) {
	if l.notify != nil {
		l.notify()
	}
}

// OnNewestChange implements agollo's ChangeListener.
func (l *apolloSourceListener) OnNewestChange(event *storage.FullChangeEvent) {
	// OnChange already triggers notification
}

// Compile-time assertion: ensure ApolloSource implements Source and Watchable.
var _ Source = (*ApolloSource)(nil)
var _ Watchable = (*ApolloSource)(nil)
