// Package apollo provides an Apollo-backed configuration source for Astra.
//
// It implements both config.Source and config.Watchable, enabling hot-reload
// via Apollo's long-polling change notification mechanism.
//
// # Prerequisites
//
// Apollo Meta Server must be running and reachable:
//
//	docker run -p 8080:8080 apolloconfig/apollo-quick-start
//
// # Usage
//
//	import (
//	    "github.com/astra-go/astra/config"
//	    apollocfg "github.com/astra-go/astra/config/apollo"
//	)
//
//	src, err := apollocfg.New(apollocfg.Config{
//	    AppID:         "my-service",
//	    MetaAddr:      "http://localhost:8080",
//	    NamespaceName: "application",
//	})
//
//	cfg := config.New(src)
//	cfg.StartWatch(ctx, func() {
//	    log.Println("Apollo config changed — reloading")
//	})
//
// # Namespace mapping
//
// Apollo stores key-value pairs under a namespace. This source exposes every
// key in the namespace as a top-level key in the returned map.
// For nested keys, use Apollo's properties format:
//
//	db.host = localhost
//	db.port = 5432
//
// Results in: map["db.host"] = "localhost", map["db.port"] = "5432"
// (dot-delimited nesting is the Apollo convention).
package apollo

import (
	"context"
	"fmt"

	"github.com/apolloconfig/agollo/v4"
	"github.com/apolloconfig/agollo/v4/env/config"
	"github.com/apolloconfig/agollo/v4/storage"

	astracfg "github.com/astra-go/astra/config"
)

// Config configures the Apollo configuration source.
type Config struct {
	// AppID is the Apollo application identifier. Required.
	AppID string

	// Cluster is the Apollo cluster name. Default: "default".
	Cluster string

	// NamespaceName is the Apollo namespace to load.
	// Default: "application".
	NamespaceName string

	// MetaAddr is the Apollo Meta Server address.
	// Example: "http://localhost:8080"
	MetaAddr string

	// BackupConfigPath is the directory for local config file backup.
	// Apollo SDK uses this as a fallback when the server is unreachable.
	BackupConfigPath string
}

func (c *Config) setDefaults() {
	if c.Cluster == "" {
		c.Cluster = "default"
	}
	if c.NamespaceName == "" {
		c.NamespaceName = "application"
	}
}

// Source implements config.Source and config.Watchable backed by Apollo.
type Source struct {
	cfg    Config
	client agollo.Client
}

// New creates a new Apollo Source and connects to the Apollo server.
func New(cfg Config) (*Source, error) {
	cfg.setDefaults()
	if cfg.AppID == "" {
		return nil, fmt.Errorf("apollo: AppID is required")
	}
	if cfg.MetaAddr == "" {
		return nil, fmt.Errorf("apollo: MetaAddr is required")
	}

	appCfg := &config.AppConfig{
		AppID:          cfg.AppID,
		Cluster:        cfg.Cluster,
		NamespaceName:  cfg.NamespaceName,
		IP:             cfg.MetaAddr,
		BackupConfigPath: cfg.BackupConfigPath,
	}

	client, err := agollo.StartWithConfig(func() (*config.AppConfig, error) {
		return appCfg, nil
	})
	if err != nil {
		return nil, fmt.Errorf("apollo: start: %w", err)
	}

	return &Source{cfg: cfg, client: client}, nil
}

// Load returns all key-value pairs from the Apollo namespace as a flat map.
// All values are returned as strings; the caller may use config.Scan to bind
// them into typed structs.
func (s *Source) Load() (map[string]any, error) {
	ns := s.client.GetConfig(s.cfg.NamespaceName)
	if ns == nil {
		return nil, fmt.Errorf("apollo: namespace %q not found", s.cfg.NamespaceName)
	}
	cache := ns.GetCache()
	if cache == nil {
		return map[string]any{}, nil
	}

	out := make(map[string]any)
	cache.Range(func(key, value any) bool {
		out[fmt.Sprintf("%v", key)] = value
		return true
	})
	return out, nil
}

// Name implements config.Source.
func (s *Source) Name() string {
	return fmt.Sprintf("apollo:%s/%s", s.cfg.AppID, s.cfg.NamespaceName)
}

// Watch registers a change listener. notify is called whenever any key in the
// namespace changes. Blocks until ctx is cancelled.
//
// Implements config.Watchable.
func (s *Source) Watch(ctx context.Context, notify func()) error {
	listener := &changeListener{notify: notify}
	s.client.AddChangeListener(listener)

	<-ctx.Done()

	s.client.RemoveChangeListener(listener)
	return nil
}

// Verify Source implements config.Source and config.Watchable.
var _ astracfg.Source = (*Source)(nil)
var _ astracfg.Watchable = (*Source)(nil)

// ─── change listener ─────────────────────────────────────────────────────────

// changeListener adapts the Apollo SDK's listener interface to a simple notify func.
type changeListener struct {
	notify func()
}

// OnChange implements agollo's ChangeListener.
func (l *changeListener) OnChange(event *storage.ChangeEvent) {
	if len(event.Changes) > 0 {
		l.notify()
	}
}

// OnNewestChange implements agollo's ChangeListener.
func (l *changeListener) OnNewestChange(event *storage.FullChangeEvent) {
	// OnChange already triggered; nothing extra to do.
}
