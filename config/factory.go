// Package config - Factory methods for creating configuration clients.
//
// Factory methods provide flexibility when the client type needs to be
// determined at runtime (e.g., from a configuration file).
//
// # When to use factory methods vs type-safe constructors
//
// ✅ Use type-safe constructors (NewNacosClient, NewEtcdClient, etc.):
//   - You know the client type at compile time
//   - You want IDE autocomplete and type checking
//   - You want the safest possible API
//
// ✅ Use factory methods (NewClient):
//   - The client type is determined at runtime (e.g., from config)
//   - You're building a generic configuration wrapper
//   - You need to create multiple clients dynamically
//
// # Example
//
//	clientType := os.Getenv("CONFIG_TYPE") // "nacos", "etcd", "apollo"
//	client, err := config.NewClient(config.ClientType(clientType), config.Options{
//	    // ... options
//	})
package config

import (
	"fmt"
)

// ─── Factory Method ─────────────────────────────────────────────

// NewClient creates a configuration client by type string.
// This is the factory method for runtime type selection.
//
// # Supported client types
//
//   - config.ClientTypeNacos ("nacos")   → NewNacosClient
//   - config.ClientTypeEtcd ("etcd")     → NewEtcdClient
//   - config.ClientTypeApollo ("apollo") → NewApolloClient
//   - config.ClientTypeVault ("vault")   → NewVaultClient (planned)
//
// # Example
//
//	client, err := config.NewClient(config.ClientTypeNacos, config.NacosOptions{
//	    ServerAddr: "localhost:8848",
//	    Namespace:  "public",
//	    DataID:     "myapp.yaml",
//	})
func NewClient(typ ClientType, opts interface{}) (ConfigClient, error) {
	switch typ {
	case ClientTypeNacos:
		nacosOpts, ok := opts.(NacosOptions)
		if !ok {
			return nil, fmt.Errorf("config: invalid options type for Nacos, expected NacosOptions")
		}
		return NewNacosClient(nacosOpts)

	case ClientTypeEtcd:
		etcdOpts, ok := opts.(EtcdOptions)
		if !ok {
			return nil, fmt.Errorf("config: invalid options type for Etcd, expected EtcdOptions")
		}
		return NewEtcdClient(etcdOpts)

	case ClientTypeApollo:
		apolloOpts, ok := opts.(ApolloOptions)
		if !ok {
			return nil, fmt.Errorf("config: invalid options type for Apollo, expected ApolloOptions")
		}
		return NewApolloClient(apolloOpts)

	case ClientTypeVault:
		return nil, fmt.Errorf("config: Vault client not yet implemented")

	default:
		return nil, fmt.Errorf("config: unknown client type %q, supported types: nacos, etcd, apollo", typ)
	}
}

// ─── Helper Functions ────────────────────────────────────────────

// ParseClientType parses a string into a ClientType.
// Returns an error if the string is not a valid client type.
func ParseClientType(s string) (ClientType, error) {
	switch s {
	case "nacos":
		return ClientTypeNacos, nil
	case "etcd":
		return ClientTypeEtcd, nil
	case "apollo":
		return ClientTypeApollo, nil
	case "vault":
		return ClientTypeVault, nil
	default:
		return "", fmt.Errorf("config: unknown client type %q", s)
	}
}

// String returns the string representation of the ClientType.
func (t ClientType) String() string {
	return string(t)
}

// IsValid checks if the ClientType is valid.
func (t ClientType) IsValid() bool {
	switch t {
	case ClientTypeNacos, ClientTypeEtcd, ClientTypeApollo, ClientTypeVault:
		return true
	}
	return false
}

// ─── Batch Creation ─────────────────────────────────────────────

// ClientConfig holds the configuration for creating a single client.
type ClientConfig struct {
	Type  ClientType
	Options interface{}
}

// NewClients creates multiple configuration clients from a config list.
// Returns a map of client name to ConfigClient.
//
// # Example
//
//	clients, err := config.NewClients([]config.ClientConfig{
//	    {
//	        Type: config.ClientTypeNacos,
//	        Options: config.NacosOptions{...},
//	    },
//	    {
//	        Type: config.ClientTypeEtcd,
//	        Options: config.EtcdOptions{...},
//	    },
//	})
func NewClients(configs []ClientConfig) (map[string]ConfigClient, error) {
	result := make(map[string]ConfigClient)

	for i, cfg := range configs {
		client, err := NewClient(cfg.Type, cfg.Options)
		if err != nil {
			return nil, fmt.Errorf("config: failed to create client %d (%s): %w", i, cfg.Type, err)
		}
		result[string(cfg.Type)+"_"+fmt.Sprintf("%d", i)] = client
	}

	return result, nil
}

// ─── Deprecated: Old API Compatibility ─────────────────────────

// Deprecated: Use NewNacosClient instead.
// This function exists for backward compatibility with config/nacos package.
func NewNacosClientLegacy(client interface{}, opts NacosOptions) (ConfigClient, error) {
	return NewNacosClient(opts)
}

// Deprecated: Use NewEtcdClient instead.
// This function exists for backward compatibility with config/etcd package.
func NewEtcdClientLegacy(client interface{}, key string, format Format) (ConfigClient, error) {
	return NewEtcdClient(EtcdOptions{
		Endpoints: []string{"localhost:2379"},
		KeyPrefix: key,
	})
}

// Deprecated: Use NewApolloClient instead.
// This function exists for backward compatibility with config/apollo package.
func NewApolloClientLegacy(opts ApolloOptions) (ConfigClient, error) {
	return NewApolloClient(opts)
}
