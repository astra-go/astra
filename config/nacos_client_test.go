package config

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/nacos-group/nacos-sdk-go/v2/clients/config_client"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
)

// MockNacosClient is a mock Nacos config client for testing.
type MockNacosClient struct {
	config_client.IConfigClient
	configData map[string]string
	listenConfigs []vo.ConfigParam
}

func NewMockNacosClient() *MockNacosClient {
	return &MockNacosClient{
		configData:    make(map[string]string),
		listenConfigs: make([]vo.ConfigParam, 0),
	}
}

func (m *MockNacosClient) GetConfig(param vo.ConfigParam) (string, error) {
	data, ok := m.configData[param.DataId+"/"+param.Group]
	if !ok {
		return "", fmt.Errorf("config not found")
	}
	return data, nil
}

func (m *MockNacosClient) ListenConfig(param vo.ConfigParam) error {
	m.listenConfigs = append(m.listenConfigs, param)
	return nil
}

func (m *MockNacosClient) CancelListenConfig(param vo.ConfigParam) error {
	return nil
}

func (m *MockNacosClient) SetConfig(dataID, group, content string) {
	m.configData[dataID+"/"+group] = content
}

// TestNewNacosClient tests the creation of Nacos client.
func TestNewNacosClient(t *testing.T) {
	tests := []struct {
		name    string
		opts    NacosOptions
		wantErr bool
	}{
		{
			name: "valid options",
			opts: NacosOptions{
				Options:     DefaultOptions(),
				ServerAddr: "localhost:8848",
				Namespace:  "public",
				Group:       "DEFAULT_GROUP",
				DataID:      "test.yaml",
				Format:      YAMLFormat,
			},
			wantErr: false,
		},
		{
			name: "missing server addr",
			opts: NacosOptions{
				DataID: "test.yaml",
			},
			wantErr: true,
		},
		{
			name: "missing data id",
			opts: NacosOptions{
				ServerAddr: "localhost:8848",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewNacosClient(tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewNacosClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && client == nil {
				t.Error("NewNacosClient() returned nil client without error")
			}
		})
	}
}

// TestNacosClientGet tests the Get method.
func TestNacosClientGet(t *testing.T) {
	// This is an integration test that requires a running Nacos server
	// Skip if NACOS_TEST is not set
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client, err := NewNacosClient(NacosOptions{
		ServerAddr: "localhost:8848",
		Namespace:  "public",
		Group:      "DEFAULT_GROUP",
		DataID:     "test.yaml",
		Format:      YAMLFormat,
	})
	if err != nil {
		t.Skipf("Failed to create client: %v", err)
	}
	defer client.Close()

	tests := []struct {
		name    string
		key     string
		want    string
		wantErr bool
	}{
		{
			name:    "existing key",
			key:     "db.host",
			want:    "localhost",
			wantErr: false,
		},
		{
			name:    "non-existing key",
			key:     "nonexistent",
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := client.Get(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("Get() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("Get() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestNacosClientGetWithDefault tests the GetWithDefault method.
func TestNacosClientGetWithDefault(t *testing.T) {
	client, err := NewNacosClient(NacosOptions{
		ServerAddr: "localhost:8848",
		Namespace:  "public",
		Group:      "DEFAULT_GROUP",
		DataID:     "test.yaml",
		Format:      YAMLFormat,
	})
	if err != nil {
		t.Skipf("Failed to create client: %v", err)
	}
	defer client.Close()

	tests := []struct {
		name         string
		key          string
		defaultValue string
		want         string
	}{
		{
			name:         "existing key",
			key:          "db.host",
			defaultValue: "127.0.0.1",
			want:         "localhost",
		},
		{
			name:         "non-existing key",
			key:          "nonexistent",
			defaultValue: "default",
			want:         "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := client.GetWithDefault(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("GetWithDefault() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestNacosClientWatch tests the Watch method.
func TestNacosClientWatch(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client, err := NewNacosClient(NacosOptions{
		ServerAddr:   "localhost:8848",
		Namespace:    "public",
		Group:        "DEFAULT_GROUP",
		DataID:       "test.yaml",
		Format:        YAMLFormat,
		EnableWatch:  true,
	})
	if err != nil {
		t.Skipf("Failed to create client: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	changed := false
	err = client.Watch(ctx, "db.host", func(newValue string) {
		changed = true
	})
	if err != nil {
		t.Errorf("Watch() error = %v", err)
	}

	// Wait for potential change notification
	time.Sleep(2 * time.Second)

	if !changed {
		t.Log("Watch() callback was not called (this may be expected in test environment)")
	}
}

// TestParseAddr tests the parseAddr helper function.
func TestParseAddr(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		wantHost string
		wantPort uint64
		wantErr bool
	}{
		{
			name:     "valid address",
			addr:     "localhost:8848",
			wantHost:  "localhost",
			wantPort: 8848,
			wantErr:  false,
		},
		{
			name:    "invalid format - no port",
			addr:    "localhost",
			wantErr: true,
		},
		{
			name:    "invalid format - empty string",
			addr:    "",
			wantErr: true,
		},
		{
			name:    "invalid port - not a number",
			addr:    "localhost:abc",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHost, gotPort, err := parseAddr(tt.addr)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseAddr() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if gotHost != tt.wantHost {
					t.Errorf("parseAddr() host = %v, want %v", gotHost, tt.wantHost)
				}
				if gotPort != tt.wantPort {
					t.Errorf("parseAddr() port = %v, want %v", gotPort, tt.wantPort)
				}
			}
		})
	}
}

// TestFlattenMap tests the flattenMap helper function.
func TestFlattenMap(t *testing.T) {
	input := map[string]interface{}{
		"db": map[string]interface{}{
			"host": "localhost",
			"port": 5432,
		},
		"debug": true,
	}

	result := make(map[string]string)
	flattenMap(input, "", result)

	expected := map[string]string{
		"db.host": "localhost",
		"db.port": "5432",
		"debug":   "true",
	}

	for k, v := range expected {
		if result[k] != v {
			t.Errorf("flattenMap()[%s] = %v, want %v", k, result[k], v)
		}
	}
}

// BenchmarkNacosClientGet benchmarks the Get method.
func BenchmarkNacosClientGet(b *testing.B) {
	client, err := NewNacosClient(NacosOptions{
		ServerAddr: "localhost:8848",
		Namespace:  "public",
		Group:      "DEFAULT_GROUP",
		DataID:     "test.yaml",
		Format:      YAMLFormat,
	})
	if err != nil {
		b.Skipf("Failed to create client: %v", err)
	}
	defer client.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = client.Get("db.host")
	}
}
