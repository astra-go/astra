// Package config provides Nacos as a configuration source for Astra.
//
// # Usage
//
//	src := config.NewNacrosSource(config.NacosSourceConfig{
//	    ServerAddr: "localhost:8848",
//	    DataID:     "myapp.yaml",
//	    Group:      "DEFAULT_GROUP",
//	    Namespace:  "public",
//	    Format:     config.YAMLFormat,
//	})
//	cfg, _ := config.New(src)
//	cfg.StartWatch(ctx)  // Nacos 变更自动触发 Load
//
// # With existing Nacos SDK client
//
//	src := config.NewNacosSourceWithClient(sdkClient, config.NacosSourceConfig{
//	    DataID: "myapp.yaml",
//	    Group:  "DEFAULT_GROUP",
//	    Format: config.YAMLFormat,
//	})
//	cfg, _ := config.New(src)
//	cfg.StartWatch(ctx)
package config

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/clients/config_client"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
	"gopkg.in/yaml.v3"
)

// NacosSourceConfig configures a NacosSource.
type NacosSourceConfig struct {
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
}

// NacosSource reads configuration from a single Nacos data ID.
// The value must be a JSON or YAML document.
//
// It implements both Source (for initial load) and Watchable (for hot reload).
type NacosSource struct {
	client config_client.IConfigClient
	cfg    NacosSourceConfig
}

// NewNacosSource creates a NacosSource with the given config.
// It creates the Nacos SDK client internally.
func NewNacosSource(cfg NacosSourceConfig) (*NacosSource, error) {
	if cfg.ServerAddr == "" {
		return nil, fmt.Errorf("config: NacosSource requires ServerAddr")
	}
	if cfg.DataID == "" {
		return nil, fmt.Errorf("config: NacosSource requires DataID")
	}

	// Apply defaults
	if cfg.Namespace == "" {
		cfg.Namespace = "public"
	}
	if cfg.Group == "" {
		cfg.Group = "DEFAULT_GROUP"
	}
	if cfg.Format == 0 {
		cfg.Format = YAMLFormat
	}

	// Parse server address
	host, port, err := parseNacosAddr(cfg.ServerAddr)
	if err != nil {
		return nil, fmt.Errorf("config: invalid Nacos ServerAddr: %w", err)
	}

	// Create Nacos SDK client
	sc := []constant.ServerConfig{
		*constant.NewServerConfig(host, port),
	}

	cc := constant.NewClientConfig(
		constant.WithNamespaceId(cfg.Namespace),
		constant.WithTimeoutMs(5000),
	)
	if cfg.Username != "" {
		cc = constant.NewClientConfig(
			constant.WithUsername(cfg.Username),
			constant.WithPassword(cfg.Password),
		)
	}

	sdkClient, err := clients.NewConfigClient(vo.NacosClientParam{
		ClientConfig:  cc,
		ServerConfigs: sc,
	})
	if err != nil {
		return nil, fmt.Errorf("config: create Nacos SDK client: %w", err)
	}

	return &NacosSource{
		client: sdkClient,
		cfg:    cfg,
	}, nil
}

// NewNacosSourceWithClient creates a NacosSource with an existing
// Nacos SDK config client. Use this when you already have a Nacos SDK
// client and want to wrap it as a config Source.
func NewNacosSourceWithClient(client config_client.IConfigClient, cfg NacosSourceConfig) (*NacosSource, error) {
	if client == nil {
		return nil, fmt.Errorf("config: Nacos SDK client is required")
	}
	if cfg.DataID == "" {
		return nil, fmt.Errorf("config: NacosSource requires DataID")
	}

	// Apply defaults
	if cfg.Group == "" {
		cfg.Group = "DEFAULT_GROUP"
	}
	if cfg.Format == 0 {
		cfg.Format = YAMLFormat
	}

	return &NacosSource{
		client: client,
		cfg:    cfg,
	}, nil
}

func (s *NacosSource) Name() string {
	return fmt.Sprintf("nacos:%s/%s", s.cfg.Group, s.cfg.DataID)
}

// Load fetches the current value of the Nacos data ID.
func (s *NacosSource) Load() (map[string]any, error) {
	content, err := s.client.GetConfig(vo.ConfigParam{
		DataId: s.cfg.DataID,
		Group:  s.cfg.Group,
	})
	if err != nil {
		return nil, fmt.Errorf("nacos: get config %s: %w", s.cfg.DataID, err)
	}

	if content == "" {
		return make(map[string]any), nil // empty config
	}

	return parseNacosContent(content, s.cfg.Format)
}

// Watch watches the Nacos data ID for changes and calls notify when it changes.
// Runs until ctx is cancelled.
func (s *NacosSource) Watch(ctx context.Context, notify func()) error {
	err := s.client.ListenConfig(vo.ConfigParam{
		DataId: s.cfg.DataID,
		Group:  s.cfg.Group,
		OnChange: func(namespace, group, dataId, data string) {
			if ctx.Err() != nil {
				return
			}
			notify()
		},
	})
	if err != nil {
		return fmt.Errorf("nacos: listen config: %w", err)
	}

	// Wait for context cancellation
	<-ctx.Done()

	// Cancel listener
	_ = s.client.CancelListenConfig(vo.ConfigParam{
		DataId: s.cfg.DataID,
		Group:  s.cfg.Group,
	})

	return nil
}

// parseNacosAddr parses a host:port address.
func parseNacosAddr(addr string) (string, uint64, error) {
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

// parseNacosContent parses Nacos config content (JSON/YAML) into a map.
func parseNacosContent(content string, format Format) (map[string]any, error) {
	switch format {
	case JSONFormat:
		return parseJSONToMap(content)
	case YAMLFormat:
		var result map[string]any
		if err := yaml.Unmarshal([]byte(content), &result); err != nil {
			return nil, fmt.Errorf("nacos: parse YAML: %w", err)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("config: unknown format %d", format)
	}
}

func parseJSONToMap(content string) (map[string]any, error) {
	var result map[string]any
	if err := yaml.Unmarshal([]byte(content), &result); err != nil {
		// Actually try JSON: yaml.v3 can also parse JSON
		return nil, fmt.Errorf("config: parse JSON: %w", err)
	}
	return result, nil
}

// Compile-time assertion: ensure NacosSource implements Source and Watchable.
var _ Source = (*NacosSource)(nil)
var _ Watchable = (*NacosSource)(nil)
