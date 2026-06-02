// Package config - Nacos client implementation
//
// # Usage
//
//	import "github.com/astra-go/astra/config"
//
//	client, err := config.NewNacosClient(config.NacosOptions{
//	    ServerAddr: "localhost:8848",
//	    Namespace:  "public",
//	    Group:      "DEFAULT_GROUP",
//	    DataID:     "myapp.yaml",
//	    Format:     config.YAMLFormat,
//	})
//
//	value, _ := client.Get("db.host")
//	client.Watch(ctx, "db.host", func(newValue string) {
//	    log.Printf("db.host changed to: %s", newValue)
//	})
package config

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/clients/config_client"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
	"gopkg.in/yaml.v3"
)

// ─── Nacos Options ─────────────────────────────────────────────────────

// NacosOptions configures a Nacos configuration client.
type NacosOptions struct {
	Options // Embed common options

	// ServerAddr is the Nacos server address (host:port).
	// Required.
	ServerAddr string

	// Namespace is the Nacos namespace ID.
	// Default: "public"
	Namespace string

	// Group is the Nacos group name.
	// Default: "DEFAULT_GROUP"
	Group string

	// DataID is the Nacos configuration data ID.
	// Required.
	DataID string

	// Format specifies the configuration format (YAML or JSON).
	// Default: YAMLFormat
	Format Format

	// Username for Nacos authentication (optional).
	Username string

	// Password for Nacos authentication (optional).
	Password string

	// DataIDPrefix is prepended to all Get() key lookups.
	// Default: ""
	DataIDPrefix string

	// EnableWatch enables long-polling watch for configuration changes.
	// Default: true
	EnableWatch bool
}

// DefaultNacosOptions returns the default Nacos options.
func DefaultNacosOptions() NacosOptions {
	return NacosOptions{
		Options:     DefaultOptions(),
		Namespace:   "public",
		Group:       "DEFAULT_GROUP",
		Format:      YAMLFormat,
		EnableWatch: true,
	}
}

// ─── Nacos Client ──────────────────────────────────────────────────────

// NacosClient implements ConfigClient for Nacos.
type NacosClient struct {
	client      config_client.IConfigClient
	opts        NacosOptions
	cache       map[string]string
	watchCancel context.CancelFunc
}

// NewNacosClient creates a new Nacos configuration client.
// This is the type-safe constructor (recommended).
func NewNacosClient(opts NacosOptions) (ConfigClient, error) {
	if opts.ServerAddr == "" {
		return nil, fmt.Errorf("config/nacos: ServerAddr is required")
	}
	if opts.DataID == "" {
		return nil, fmt.Errorf("config/nacos: DataID is required")
	}

	// Apply defaults
	if opts.Namespace == "" {
		opts.Namespace = "public"
	}
	if opts.Group == "" {
		opts.Group = "DEFAULT_GROUP"
	}
	if opts.Format == 0 {
		opts.Format = YAMLFormat
	}

	// Parse server address
	host, port, err := parseAddr(opts.ServerAddr)
	if err != nil {
		return nil, fmt.Errorf("config/nacos: invalid ServerAddr: %w", err)
	}

	// Create Nacos client
	sc := []constant.ServerConfig{
		*constant.NewServerConfig(host, port),
	}

	cc := constant.NewClientConfig(
		constant.WithNamespaceId(opts.Namespace),
		constant.WithTimeoutMs(uint64(opts.Timeout.Milliseconds())),
		constant.WithLogLevel(opts.LogLevel),
	)
	if opts.Username != "" {
		cc = constant.NewClientConfig(
			constant.WithUsername(opts.Username),
			constant.WithPassword(opts.Password),
		)
	}

	configClient, err := clients.NewConfigClient(vo.NacosClientParam{
		ClientConfig:  cc,
		ServerConfigs: sc,
	})
	if err != nil {
		return nil, fmt.Errorf("config/nacos: create client: %w", err)
	}

	nc := &NacosClient{
		client: configClient,
		opts:   opts,
		cache:  make(map[string]string),
	}

	// Initial load
	if opts.EnableCache {
		if err := nc.refreshCache(); err != nil {
			slog.Warn("config/nacos: initial cache refresh failed", "error", err)
		}
	}

	return nc, nil
}

// ─── ConfigClient Interface ──────────────────────────────────────────

// Get retrieves a configuration value by key.
func (c *NacosClient) Get(key string) (string, error) {
	if c.opts.EnableCache {
		cached, ok := c.cache[key]
		if ok {
			return cached, nil
		}
	}

	content, err := c.client.GetConfig(vo.ConfigParam{
		DataId: c.opts.DataID,
		Group:  c.opts.Group,
	})
	if err != nil {
		return "", fmt.Errorf("config/nacos: get config: %w", err)
	}

	value, err := extractValue(content, c.opts.Format, key)
	if err != nil {
		return "", err
	}

	if c.opts.EnableCache {
		c.cache[key] = value
	}

	return value, nil
}

// GetWithDefault retrieves a configuration value, returning a default if not found.
func (c *NacosClient) GetWithDefault(key string, defaultValue string) string {
	value, err := c.Get(key)
	if err != nil {
		return defaultValue
	}
	return value
}

// GetInt retrieves an integer configuration value.
func (c *NacosClient) GetInt(key string) (int, error) {
	value, err := c.Get(key)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(value)
}

// GetBool retrieves a boolean configuration value.
func (c *NacosClient) GetBool(key string) (bool, error) {
	value, err := c.Get(key)
	if err != nil {
		return false, err
	}
	return parseBoolNacos(value)
}

// GetAll returns all configuration key-value pairs.
func (c *NacosClient) GetAll() (map[string]string, error) {
	content, err := c.client.GetConfig(vo.ConfigParam{
		DataId: c.opts.DataID,
		Group:  c.opts.Group,
	})
	if err != nil {
		return nil, fmt.Errorf("config/nacos: get config: %w", err)
	}

	return parseContentToMap(content, c.opts.Format)
}

// Watch starts watching for configuration changes.
func (c *NacosClient) Watch(ctx context.Context, key string, callback func(newValue string)) error {
	if !c.opts.EnableWatch {
		return ErrWatchNotSupported
	}

	// Create cancellable context
	watchCtx, cancel := context.WithCancel(ctx)
	c.watchCancel = cancel

	// Register Nacos listener
	err := c.client.ListenConfig(vo.ConfigParam{
		DataId: c.opts.DataID,
		Group:  c.opts.Group,
		OnChange: func(namespace, group, dataId, data string) {
			if ctx.Err() != nil {
				return
			}

			// Extract new value for the key
			newValue, err := extractValue(data, c.opts.Format, key)
			if err != nil {
				slog.Error("config/nacos: extract value on watch", "error", err)
				return
			}

			// Update cache
			if c.opts.EnableCache {
				c.cache[key] = newValue
			}

			// Call user callback
			callback(newValue)
		},
	})
	if err != nil {
		cancel()
		return fmt.Errorf("config/nacos: listen config: %w", err)
	}

	// Wait for context cancellation
	<-watchCtx.Done()

	// Cancel Nacos listener
	_ = c.client.CancelListenConfig(vo.ConfigParam{
		DataId: c.opts.DataID,
		Group:  c.opts.Group,
	})

	return nil
}

// Close closes the client and releases resources.
func (c *NacosClient) Close() error {
	if c.watchCancel != nil {
		c.watchCancel()
	}
	// Nacos client doesn't have an explicit Close() method
	// Resources are released when the process exits
	return nil
}

// ─── Internal Methods ────────────────────────────────────────────────

// refreshCache reloads all configuration into cache.
func (c *NacosClient) refreshCache() error {
	content, err := c.client.GetConfig(vo.ConfigParam{
		DataId: c.opts.DataID,
		Group:  c.opts.Group,
	})
	if err != nil {
		return err
	}

	dataMap, err := parseContentToMap(content, c.opts.Format)
	if err != nil {
		return err
	}

	c.cache = dataMap
	return nil
}

// ─── Helper Functions ─────────────────────────────────────────────────

func parseAddr(addr string) (string, uint64, error) {
	parts := strings.Split(addr, ":")
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("invalid address format: %s", addr)
	}
	host := parts[0]
	port, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return "", 0, fmt.Errorf("invalid port: %w", err)
	}
	return host, port, nil
}

func extractValue(content string, format Format, key string) (string, error) {
	dataMap, err := parseContentToMap(content, format)
	if err != nil {
		return "", err
	}

	value, ok := dataMap[key]
	if !ok {
		return "", ErrKeyNotFound
	}

	return value, nil
}

func parseContentToMap(content string, format Format) (map[string]string, error) {
	result := make(map[string]string)

	switch format {
	case JSONFormat:
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(content), &data); err != nil {
			return nil, fmt.Errorf("config/nacos: parse JSON: %w", err)
		}
		flattenMap(data, "", result)

	default: // YAMLFormat
		var data map[string]interface{}
		if err := yaml.Unmarshal([]byte(content), &data); err != nil {
			return nil, fmt.Errorf("config/nacos: parse YAML: %w", err)
		}
		flattenMap(data, "", result)
	}

	return result, nil
}

func flattenMap(data map[string]interface{}, prefix string, result map[string]string) {
	for k, v := range data {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}

		switch val := v.(type) {
		case map[string]interface{}:
			flattenMap(val, key, result)
		case string:
			result[key] = val
		case int, int64, float64, bool:
			result[key] = fmt.Sprintf("%v", val)
		default:
			result[key] = fmt.Sprintf("%v", val)
		}
	}
}

func parseBoolNacos(s string) (bool, error) {
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

// Compile-time assertion.
var _ ConfigClient = (*NacosClient)(nil)
