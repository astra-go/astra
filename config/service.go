// Package config provides a base service configuration struct that can be embedded
// into service-specific config structs. This eliminates duplication across microservices.
//
// Usage:
//
//	type MyServiceConfig struct {
//	    config.BaseServiceConfig `yaml:",inline" json:",inline"`
//
//	    // Service-specific fields
//	    Database DatabaseConfig `yaml:"database" json:"database"`
//	    Redis   RedisConfig    `yaml:"redis" json:"redis"`
//	}
//
//	cfg, err := config.LoadWithAstra("my-service", "config", "MY")
//	// cfg.Name, cfg.Port, cfg.Mode, etc. are available from BaseServiceConfig
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

// BaseServiceConfig contains common configuration fields shared by all services.
// Embed this struct into your service-specific config to inherit these fields.
//
// All fields support `default` tags for automatic defaults, and can be overridden
// via YAML files or environment variables.
type BaseServiceConfig struct {
	// Name is the service name, used for logging, tracing, and error reporting.
	Name string `yaml:"name" json:"name" default:"unknown-service"`

	// Port is the HTTP server port (without colon), e.g. "8080".
	Port string `yaml:"port" json:"port" default:"8080"`

	// GrpcPort is the gRPC server port. Set to 0 to disable gRPC.
	GrpcPort int `yaml:"grpc_port" json:"grpc_port" default:"0"`

	// Version is the service version, e.g. "1.0.0" or "dev".
	Version string `yaml:"version" json:"version" default:"dev"`

	// Mode is the runtime mode: dev | prod | staging | test.
	Mode string `yaml:"mode" json:"mode" default:"dev"`

	// LogLevel is the logging level: debug | info | warn | error.
	LogLevel string `yaml:"log_level" json:"log_level" default:"info"`

	// LogFormat is the log output format: auto | json | text.
	// auto → text in dev mode, json in prod/staging.
	LogFormat string `yaml:"log_format" json:"log_format" default:"auto"`

	// StorageBackend selects the storage implementation: memory | sql-redis.
	StorageBackend string `yaml:"storage_backend" json:"storage_backend" default:"memory"`

	// ShutdownTimeout is the graceful shutdown timeout in seconds.
	ShutdownTimeout int `yaml:"shutdown_timeout" json:"shutdown_timeout" default:"30"`

	// HealthLivePath is the liveness probe endpoint path.
	HealthLivePath string `yaml:"health_live_path" json:"health_live_path" default:"/health/live"`

	// HealthReadyPath is the readiness probe endpoint path.
	HealthReadyPath string `yaml:"health_ready_path" json:"health_ready_path" default:"/health/ready"`

	// CORSAllowOrigins is the list of allowed CORS origins.
	CORSAllowOrigins []string `yaml:"cors_allow_origins" json:"cors_allow_origins"`

	// TrustedProxies is the list of trusted proxy IPs/CIDRs for client IP extraction.
	TrustedProxies []string `yaml:"trusted_proxies" json:"trusted_proxies"`
}

// Validate validates the base configuration fields.
func (c *BaseServiceConfig) Validate() error {
	if c.Port == "" {
		return fmt.Errorf("port is required")
	}
	validModes := map[string]bool{"dev": true, "prod": true, "staging": true, "test": true}
	if !validModes[c.Mode] {
		return fmt.Errorf("invalid mode: %s (must be dev|prod|staging|test)", c.Mode)
	}
	validBackends := map[string]bool{"": true, "memory": true, "sql-redis": true}
	if !validBackends[c.StorageBackend] {
		return fmt.Errorf("invalid storage_backend: %s (must be memory|sql-redis)", c.StorageBackend)
	}
	if c.ShutdownTimeout < 0 {
		return fmt.Errorf("shutdown_timeout must be >= 0")
	}
	return nil
}

// IsDev returns true if the service is running in development mode.
func (c *BaseServiceConfig) IsDev() bool {
	return c.Mode == "dev"
}

// IsProd returns true if the service is running in production mode.
func (c *BaseServiceConfig) IsProd() bool {
	return c.Mode == "prod"
}

// IsStaging returns true if the service is running in staging mode.
func (c *BaseServiceConfig) IsStaging() bool {
	return c.Mode == "staging"
}

// SlogLevel converts the LogLevel string to slog.Level.
func (c *BaseServiceConfig) SlogLevel() slog.Level {
	switch strings.ToLower(c.LogLevel) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// ResolvedLogFormat returns the effective log format based on mode.
// "auto" resolves to "text" in dev mode, "json" otherwise.
func (c *BaseServiceConfig) ResolvedLogFormat() string {
	if c.LogFormat == "auto" {
		if c.IsDev() {
			return "text"
		}
		return "json"
	}
	return c.LogFormat
}

// LoadWithAstra loads configuration using astra's config package and returns
// both the underlying *Config (for WatchKey hot-reload) and a typed config struct.
//
// serviceName is used to find the config file: config/{serviceName}.yaml
// configDir is the directory containing config files
// envPrefix is the environment variable prefix (e.g. "USC" for USC_PORT)
//
// File selection rules:
//  1. If config/{serviceName}.yaml exists → use it
//  2. Else if config/{APP_MODE}.yaml exists → use mode-based file (dev.yaml, prod.yaml)
//  3. Else use defaults + environment variables only
//
// Example:
//
//	cfg, svcCfg, err := LoadWithAstra[MyServiceConfig]("usercenter-svc", "config", "USC")
//	if err != nil {
//	    // handle error
//	}
//	// Use svcCfg.Name, svcCfg.Port, etc.
//	// Use cfg.WatchKey("database.host", callback) for hot reload
func LoadWithAstra[T any](serviceName, configDir, envPrefix string) (*Config, *T, error) {
	configFile := resolveConfigFile(serviceName, configDir)

	sources := buildSources(configFile, envPrefix)
	if len(sources) == 0 {
		// No sources → return config with defaults applied
		var cfg T
		applyDefaultTags(&cfg)
		return nil, &cfg, nil
	}

	mgr, err := New(sources...)
	if err != nil {
		return nil, nil, fmt.Errorf("load config: %w", err)
	}

	var svcCfg T
	if err := mgr.Scan(&svcCfg); err != nil {
		return nil, nil, fmt.Errorf("unmarshal config: %w", err)
	}

	return mgr, &svcCfg, nil
}

// Load loads configuration and returns a typed config struct (without the *Config manager).
// Use LoadWithAstra if you need hot-reload capabilities via WatchKey.
func Load[T any](serviceName, configDir, envPrefix string) (*T, error) {
	_, cfg, err := LoadWithAstra[T](serviceName, configDir, envPrefix)
	return cfg, err
}

// resolveConfigFile finds the appropriate config file path.
func resolveConfigFile(serviceName, configDir string) string {
	if configDir == "" || serviceName == "" {
		return ""
	}

	// 1. Try service-specific file
	serviceFile := fmt.Sprintf("%s/%s.yaml", configDir, serviceName)
	if _, err := os.Stat(serviceFile); err == nil {
		return serviceFile
	}

	// 2. Try mode-based file (APP_MODE env var, default "dev")
	mode := os.Getenv("APP_MODE")
	if mode == "" {
		mode = "dev"
	}
	modeFile := fmt.Sprintf("%s/%s.yaml", configDir, mode)
	if _, err := os.Stat(modeFile); err == nil {
		return modeFile
	}

	return ""
}

// buildSources constructs the config source list.
func buildSources(configFile, envPrefix string) []Source {
	var sources []Source

	if configFile != "" {
		sources = append(sources, &YAMLFile{Path: configFile})
	}

	if envPrefix != "" {
		envPrefix = strings.TrimRight(envPrefix, "_")
		sources = append(sources, &Env{Prefix: envPrefix})
	}

	return sources
}

// Common configuration structs that can be embedded or used directly.

// DatabaseConfig holds database connection settings.
type DatabaseConfig struct {
	DSN         string        `yaml:"dsn" json:"dsn"`
	Host        string        `yaml:"host" json:"host"`
	Port        int           `yaml:"port" json:"port" default:"5432"`
	User        string        `yaml:"user" json:"user"`
	Password    string        `yaml:"password" json:"password"`
	DBName      string        `yaml:"dbname" json:"dbname"`
	SSLMode     string        `yaml:"sslmode" json:"sslmode" default:"disable"`
	MaxOpen     int           `yaml:"max_open" json:"max_open"`
	MaxIdle     int           `yaml:"max_idle" json:"max_idle"`
	MaxLifetime time.Duration `yaml:"max_lifetime" json:"max_lifetime"`
	AutoMigrate bool          `yaml:"auto_migrate" json:"auto_migrate"`
}

// BuildDSN constructs a PostgreSQL DSN from the config fields.
// If DSN is already set, returns it directly.
func (c *DatabaseConfig) BuildDSN() string {
	if c.DSN != "" {
		return c.DSN
	}
	if c.Host == "" {
		return ""
	}
	sslMode := c.SSLMode
	if sslMode == "" {
		sslMode = "disable"
	}
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.DBName, sslMode)
}

// CacheConfig holds Redis cache settings.
type CacheConfig struct {
	RedisAddr     string `yaml:"redis_addr" json:"redis_addr"`
	RedisPassword string `yaml:"redis_password" json:"redis_password"`
	RedisDB       int    `yaml:"redis_db" json:"redis_db"`
	KeyPrefix     string `yaml:"key_prefix" json:"key_prefix"`
}

// SecurityConfig holds security-related settings.
type SecurityConfig struct {
	AccountLockMaxAttempts int           `yaml:"account_lock_max_attempts" json:"account_lock_max_attempts" default:"5"`
	AccountLockDuration    time.Duration `yaml:"account_lock_duration" json:"account_lock_duration"`
}

// GetMaxAttempts returns the max attempts with a sensible default.
func (c *SecurityConfig) GetMaxAttempts() int {
	if c.AccountLockMaxAttempts <= 0 {
		return 5
	}
	return c.AccountLockMaxAttempts
}

// GetLockDuration returns the lock duration with a sensible default.
func (c *SecurityConfig) GetLockDuration() time.Duration {
	if c.AccountLockDuration <= 0 {
		return 30 * time.Minute
	}
	return c.AccountLockDuration
}
