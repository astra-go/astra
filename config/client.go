// Package config provides flexible, multi-source configuration management for Astra.
//
// # Remote Sources (merged into this package)
//
// - Nacos:   config.NewNacosClient(...)
// - Etcd:     config.NewEtcdClient(...)
// - Apollo:   config.NewApolloClient(...)
// - Vault:    config.NewVaultClient(...) (planned)
//
// All remote clients implement the ConfigClient interface and support hot-reload
// via Watch().
//
// # Local Sources
//
// Sources are merged left-to-right (later sources override earlier ones):
//
//	config.YAMLFile{Path: "config.yaml"}
//	config.JSONFile{Path: "config.json"}
//	config.TOMLFile{Path: "config.toml"}
//	config.Env{Prefix: "APP"}          // APP__DB__PORT=5432 → db.port=5432
//	config.Memory{Data: map[string]any{...}}
//
// # Hot reload
//
//	cfg.StartWatch(ctx)   // begins watching all file sources for changes
//	cfg.Watch(func() { ... })  // registered hook is called on every reload
//
// # Struct binding with defaults
//
//	type AppCfg struct {
//	    Port    int           `yaml:"port"    default:"8080"`
//	    Debug   bool          `yaml:"debug"   default:"false"`
//	    Timeout time.Duration `yaml:"timeout" default:"30s"`
//	}
//	var app AppCfg
//	cfg.Scan(&app)
package config

import (
	"context"
	"errors"
	"time"
)

// ─── ConfigClient Interface ─────────────────────────────────────────────────

// ConfigClient is the unified interface for all configuration clients.
// It provides a consistent API for reading and watching configuration values.
//
// Implementations:
//   - NacosClient (Nacos)
//   - EtcdClient (Etcd)
//   - ApolloClient (Apollo)
//   - VaultClient (HashiCorp Vault, planned)
type ConfigClient interface {
	// Get retrieves a configuration value by key.
	// Returns an error if the key does not exist.
	Get(key string) (string, error)

	// GetWithDefault retrieves a configuration value by key, returning a default
	// value if the key does not exist.
	GetWithDefault(key string, defaultValue string) string

	// GetInt retrieves an integer configuration value.
	// Returns an error if the key does not exist or is not a valid integer.
	GetInt(key string) (int, error)

	// GetBool retrieves a boolean configuration value.
	// Accepts: "true", "false", "1", "0", "yes", "no"
	GetBool(key string) (bool, error)

	// Watch starts watching for configuration changes and calls the callback
	// whenever a change is detected. Blocks until ctx is cancelled.
	Watch(ctx context.Context, key string, callback func(newValue string)) error

	// GetAll returns all configuration key-value pairs as a map.
	GetAll() (map[string]string, error)

	// Close closes the client and releases any resources.
	Close() error
}

// ─── Common Options ───────────────────────────────────────────────────────

// Options provides common configuration options for all clients.
type Options struct {
	// Timeout for client operations (default: 5s)
	Timeout time.Duration

	// EnableCache enables local caching of configuration values (default: true)
	EnableCache bool

	// LogLevel controls the logging verbosity (default: "warn")
	// Valid values: "debug", "info", "warn", "error"
	LogLevel string

	// AutoRefresh enables automatic refresh of configuration values (default: false)
	AutoRefresh bool

	// RefreshInterval is the interval for auto-refresh (default: 30s)
	// Only used when AutoRefresh is true
	RefreshInterval time.Duration
}

// DefaultOptions returns the default options for all configuration clients.
func DefaultOptions() Options {
	return Options{
		Timeout:         5 * time.Second,
		EnableCache:     true,
		LogLevel:        "warn",
		AutoRefresh:     false,
		RefreshInterval: 30 * time.Second,
	}
}

// ─── Client Type ─────────────────────────────────────────────────────────

// ClientType represents the type of configuration client.
type ClientType string

const (
	// ClientTypeNacos represents a Nacos configuration client.
	ClientTypeNacos ClientType = "nacos"

	// ClientTypeEtcd represents an Etcd configuration client.
	ClientTypeEtcd ClientType = "etcd"

	// ClientTypeApollo represents an Apollo configuration client.
	ClientTypeApollo ClientType = "apollo"

	// ClientTypeVault represents a Vault configuration client.
	ClientTypeVault ClientType = "vault"
)

// ─── Errors ──────────────────────────────────────────────────────────────

// Common errors for configuration clients.
var (
	// ErrKeyNotFound is returned when a configuration key is not found.
	ErrKeyNotFound = errors.New("config: key not found")

	// ErrInvalidValue is returned when a configuration value cannot be parsed.
	ErrInvalidValue = errors.New("config: invalid value")

	// ErrClientClosed is returned when an operation is performed on a closed client.
	ErrClientClosed = errors.New("config: client closed")

	// ErrWatchNotSupported is returned when Watch is not supported by the client.
	ErrWatchNotSupported = errors.New("config: watch not supported")
)
