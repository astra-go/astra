// Package nacos provides a Nacos-backed configuration source implementing
// config.Source and config.Watchable.
//
// The source reads a single DataID/Group key from the Nacos configuration
// center. It supports JSON and YAML encoding. When used with config.StartWatch,
// configuration changes in Nacos are pushed automatically via the Nacos long-
// polling protocol.
//
// # Usage
//
//	import (
//	    "github.com/nacos-group/nacos-sdk-go/v2/clients"
//	    "github.com/nacos-group/nacos-sdk-go/v2/common/constant"
//	    "github.com/nacos-group/nacos-sdk-go/v2/vo"
//	    confignacos "github.com/astra-go/astra/config/nacos"
//	    "github.com/astra-go/astra/config"
//	)
//
//	sc := []constant.ServerConfig{{IpAddr: "127.0.0.1", Port: 8848}}
//	cc := constant.NewClientConfig(
//	    constant.WithNamespaceId("public"),
//	    constant.WithTimeoutMs(5000),
//	    constant.WithLogLevel("warn"),
//	)
//	configClient, _ := clients.NewConfigClient(vo.NacosClientParam{
//	    ClientConfig:  cc,
//	    ServerConfigs: sc,
//	})
//
//	src := confignacos.New(configClient, confignacos.Config{
//	    DataID: "myapp.yaml",
//	    Group:  "DEFAULT_GROUP",
//	    Format: config.YAMLFormat,
//	})
//
//	cfg, _ := config.New(
//	    &config.YAMLFile{Path: "config.yaml"}, // local baseline
//	    src,                                   // Nacos overrides (higher priority)
//	)
//	cfg.StartWatch(ctx) // Nacos long-polling hot-reload
package nacos

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/nacos-group/nacos-sdk-go/v2/clients/config_client"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
	"gopkg.in/yaml.v3"

	"github.com/astra-go/astra/config"
)

// Config configures the Nacos config source.
type Config struct {
	// DataID is the Nacos configuration data ID. Required.
	DataID string

	// Group is the Nacos configuration group. Default: "DEFAULT_GROUP".
	Group string

	// Format controls how the config value is parsed.
	// Use config.JSONFormat or config.YAMLFormat (default).
	Format config.Format
}

// Source reads configuration from a Nacos configuration center key.
// It implements both config.Source (for initial load) and config.Watchable
// (for hot reload via Nacos long-polling).
type Source struct {
	client config_client.IConfigClient
	dataID string
	group  string
	format config.Format
}

// New creates a Nacos-backed config source.
func New(client config_client.IConfigClient, cfg Config) *Source {
	if cfg.Group == "" {
		cfg.Group = "DEFAULT_GROUP"
	}
	return &Source{
		client: client,
		dataID: cfg.DataID,
		group:  cfg.Group,
		format: cfg.Format,
	}
}

// Name returns a human-readable source identifier.
func (s *Source) Name() string {
	return fmt.Sprintf("nacos:%s/%s", s.group, s.dataID)
}

// Load fetches the current configuration value from Nacos and parses it.
// Returns an empty map (not an error) when the key does not exist yet.
func (s *Source) Load() (map[string]any, error) {
	content, err := s.client.GetConfig(vo.ConfigParam{
		DataId: s.dataID,
		Group:  s.group,
	})
	if err != nil {
		return nil, fmt.Errorf("config/nacos: get %s/%s: %w", s.group, s.dataID, err)
	}
	if content == "" {
		return make(map[string]any), nil // key absent or empty → no override
	}
	return parseContent([]byte(content), s.format)
}

// Watch registers a Nacos long-polling listener that calls notify whenever the
// configuration value changes. Blocks until ctx is cancelled, then cancels the
// listener and returns nil.
//
// config.StartWatch calls Watch in a dedicated goroutine; the notify callback
// triggers a full config.Load() to merge all sources again.
func (s *Source) Watch(ctx context.Context, notify func()) error {
	if err := s.client.ListenConfig(vo.ConfigParam{
		DataId: s.dataID,
		Group:  s.group,
		OnChange: func(namespace, group, dataId, data string) {
			if ctx.Err() == nil {
				notify()
			}
		},
	}); err != nil {
		return fmt.Errorf("config/nacos: listen %s/%s: %w", s.group, s.dataID, err)
	}

	<-ctx.Done()
	_ = s.client.CancelListenConfig(vo.ConfigParam{
		DataId: s.dataID,
		Group:  s.group,
	})
	return nil
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func parseContent(data []byte, format config.Format) (map[string]any, error) {
	var result map[string]any
	switch format {
	case config.JSONFormat:
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.UseNumber()
		if err := dec.Decode(&result); err != nil {
			return nil, fmt.Errorf("config/nacos: parse JSON: %w", err)
		}
	default: // config.YAMLFormat (and any unknown value — default to YAML)
		if err := yaml.Unmarshal(data, &result); err != nil {
			return nil, fmt.Errorf("config/nacos: parse YAML: %w", err)
		}
	}
	return result, nil
}

// Compile-time assertions.
var _ config.Source = (*Source)(nil)
var _ config.Watchable = (*Source)(nil)
