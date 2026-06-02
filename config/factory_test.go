package config

import (
	"testing"
	"time"
)

// TestNewClient tests the NewClient factory method.
func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		typ     ClientType
		opts    interface{}
		wantErr bool
	}{
		{
			name:    "nacos with valid options",
			typ:     ClientTypeNacos,
			opts:    NacosOptions{ServerAddr: "localhost:8848", DataID: "test.yaml"},
			wantErr: false,
		},
		{
			name:    "nacos with invalid options type",
			typ:     ClientTypeNacos,
			opts:    EtcdOptions{},
			wantErr: true,
		},
		{
			name:    "etcd with valid options",
			typ:     ClientTypeEtcd,
			opts:    EtcdOptions{Endpoints: []string{"localhost:2379"}},
			wantErr: false,
		},
		{
			name:    "etcd with invalid options type",
			typ:     ClientTypeEtcd,
			opts:    NacosOptions{},
			wantErr: true,
		},
		{
			name:    "apollo with valid options",
			typ:     ClientTypeApollo,
			opts:    ApolloOptions{AppID: "test", MetaAddr: "http://localhost:8080"},
			wantErr: false,
		},
		{
			name:    "apollo with invalid options type",
			typ:     ClientTypeApollo,
			opts:    NacosOptions{},
			wantErr: true,
		},
		{
			name:    "vault (not implemented)",
			typ:     ClientTypeVault,
			opts:    NacosOptions{},
			wantErr: true,
		},
		{
			name:    "invalid client type",
			typ:     ClientType("invalid"),
			opts:    NacosOptions{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.typ, tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && client == nil {
				t.Error("NewClient() returned nil client without error")
			}
			if client != nil {
				client.Close()
			}
		})
	}
}

// TestParseClientType tests the ParseClientType function.
func TestParseClientType(t *testing.T) {
	tests := []struct {
		name    string
		s       string
		want    ClientType
		wantErr bool
	}{
		{
			name:    "nacos",
			s:       "nacos",
			want:    ClientTypeNacos,
			wantErr: false,
		},
		{
			name:    "etcd",
			s:       "etcd",
			want:    ClientTypeEtcd,
			wantErr: false,
		},
		{
			name:    "apollo",
			s:       "apollo",
			want:    ClientTypeApollo,
			wantErr: false,
		},
		{
			name:    "vault",
			s:       "vault",
			want:    ClientTypeVault,
			wantErr: false,
		},
		{
			name:    "invalid",
			s:       "invalid",
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseClientType(tt.s)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseClientType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseClientType() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestClientTypeString tests the String method of ClientType.
func TestClientTypeString(t *testing.T) {
	tests := []struct {
		name string
		typ  ClientType
		want string
	}{
		{
			name: "nacos",
			typ:  ClientTypeNacos,
			want: "nacos",
		},
		{
			name: "etcd",
			typ:  ClientTypeEtcd,
			want: "etcd",
		},
		{
			name: "apollo",
			typ:  ClientTypeApollo,
			want: "apollo",
		},
		{
			name: "vault",
			typ:  ClientTypeVault,
			want: "vault",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.typ.String(); got != tt.want {
				t.Errorf("ClientType.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestClientTypeIsValid tests the IsValid method of ClientType.
func TestClientTypeIsValid(t *testing.T) {
	tests := []struct {
		name string
		typ  ClientType
		want bool
	}{
		{
			name: "nacos (valid)",
			typ:  ClientTypeNacos,
			want: true,
		},
		{
			name: "etcd (valid)",
			typ:  ClientTypeEtcd,
			want: true,
		},
		{
			name: "apollo (valid)",
			typ:  ClientTypeApollo,
			want: true,
		},
		{
			name: "vault (valid)",
			typ:  ClientTypeVault,
			want: true,
		},
		{
			name: "invalid",
			typ:  ClientType("invalid"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.typ.IsValid(); got != tt.want {
				t.Errorf("ClientType.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestNewClients tests the NewClients function.
func TestNewClients(t *testing.T) {
	configs := []ClientConfig{
		{
			Type: ClientTypeNacos,
			Options: NacosOptions{
				ServerAddr: "localhost:8848",
				DataID:     "test.yaml",
			},
		},
		{
			Type: ClientTypeEtcd,
			Options: EtcdOptions{
				Endpoints: []string{"localhost:2379"},
			},
		},
	}

	clients, err := NewClients(configs)
	if err != nil {
		t.Errorf("NewClients() error = %v", err)
		return
	}

	if len(clients) != 2 {
		t.Errorf("NewClients() returned %d clients, want 2", len(clients))
	}

	// Clean up
	for _, client := range clients {
		client.Close()
	}
}

// TestDefaultOptions tests the DefaultOptions function.
func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()

	if opts.Timeout != 5*time.Second {
		t.Errorf("DefaultOptions().Timeout = %v, want %v", opts.Timeout, 5*time.Second)
	}

	if !opts.EnableCache {
		t.Error("DefaultOptions().EnableCache = false, want true")
	}

	if opts.LogLevel != "warn" {
		t.Errorf("DefaultOptions().LogLevel = %v, want %v", opts.LogLevel, "warn")
	}

	if opts.AutoRefresh {
		t.Error("DefaultOptions().AutoRefresh = true, want false")
	}

	if opts.RefreshInterval != 30*time.Second {
		t.Errorf("DefaultOptions().RefreshInterval = %v, want %v", opts.RefreshInterval, 30*time.Second)
	}
}

// TestDefaultNacosOptions tests the DefaultNacosOptions function.
func TestDefaultNacosOptions(t *testing.T) {
	opts := DefaultNacosOptions()

	if opts.Namespace != "public" {
		t.Errorf("DefaultNacosOptions().Namespace = %v, want %v", opts.Namespace, "public")
	}

	if opts.Group != "DEFAULT_GROUP" {
		t.Errorf("DefaultNacosOptions().Group = %v, want %v", opts.Group, "DEFAULT_GROUP")
	}

	if opts.Format != YAMLFormat {
		t.Errorf("DefaultNacosOptions().Format = %v, want %v", opts.Format, YAMLFormat)
	}

	if !opts.EnableWatch {
		t.Error("DefaultNacosOptions().EnableWatch = false, want true")
	}
}

// TestDefaultEtcdOptions tests the DefaultEtcdOptions function.
func TestDefaultEtcdOptions(t *testing.T) {
	opts := DefaultEtcdOptions()

	if len(opts.Endpoints) == 0 {
		t.Error("DefaultEtcdOptions().Endpoints is empty")
	}

	if opts.DialTimeout != 5*time.Second {
		t.Errorf("DefaultEtcdOptions().DialTimeout = %v, want %v", opts.DialTimeout, 5*time.Second)
	}

	if opts.KeyPrefix != "/config" {
		t.Errorf("DefaultEtcdOptions().KeyPrefix = %v, want %v", opts.KeyPrefix, "/config")
	}

	if !opts.WatchEnabled {
		t.Error("DefaultEtcdOptions().WatchEnabled = false, want true")
	}
}

// TestDefaultApolloOptions tests the DefaultApolloOptions function.
func TestDefaultApolloOptions(t *testing.T) {
	opts := DefaultApolloOptions()

	if opts.Cluster != "default" {
		t.Errorf("DefaultApolloOptions().Cluster = %v, want %v", opts.Cluster, "default")
	}

	if opts.NamespaceName != "application" {
		t.Errorf("DefaultApolloOptions().NamespaceName = %v, want %v", opts.NamespaceName, "application")
	}

	if opts.BackupConfigPath != "./config" {
		t.Errorf("DefaultApolloOptions().BackupConfigPath = %v, want %v", opts.BackupConfigPath, "./config")
	}

	if !opts.EnableWatch {
		t.Error("DefaultApolloOptions().EnableWatch = false, want true")
	}
}

// BenchmarkNewClient benchmarks the NewClient function.
func BenchmarkNewClient(b *testing.B) {
	opts := NacosOptions{
		ServerAddr: "localhost:8848",
		DataID:     "test.yaml",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		client, err := NewClient(ClientTypeNacos, opts)
		if err != nil {
			b.Errorf("NewClient() error = %v", err)
		}
		if client != nil {
			client.Close()
		}
	}
}
