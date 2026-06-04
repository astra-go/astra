package config

import (
	"context"
	"testing"
	"time"
)

// TestNacosSourceLoad tests the Load method of NacosSource.
func TestNacosSourceLoad(t *testing.T) {
	// Skip if no Nacos server available
	t.Skip("Requires running Nacos server")

	src, err := NewNacosSource(NacosSourceConfig{
		ServerAddr: "localhost:8848",
		DataID:    "test-config.yaml",
		Group:     "DEFAULT_GROUP",
		Namespace: "public",
		Format:    YAMLFormat,
	})
	if err != nil {
		t.Fatalf("failed to create NacosSource: %v", err)
	}

	data, err := src.Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	t.Logf("loaded config: %v", data)
}

// TestNacosSourceWatch tests the Watch method of NacosSource.
func TestNacosSourceWatch(t *testing.T) {
	t.Skip("Requires running Nacos server")

	src, err := NewNacosSource(NacosSourceConfig{
		ServerAddr: "localhost:8848",
		DataID:    "test-config.yaml",
		Group:     "DEFAULT_GROUP",
		Namespace: "public",
		Format:    YAMLFormat,
	})
	if err != nil {
		t.Fatalf("failed to create NacosSource: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	notifyCount := 0
	err = src.Watch(ctx, func() {
		notifyCount++
		t.Logf("notification received, count: %d", notifyCount)
	})
	if err != nil {
		t.Fatalf("watch failed: %v", err)
	}

	// Wait for some notifications or timeout
	select {
	case <-time.After(5 * time.Second):
	case <-ctx.Done():
	}

	t.Logf("final notification count: %d", notifyCount)
}

// TestNacosSourceName tests the Name method.
func TestNacosSourceName(t *testing.T) {
	src, err := NewNacosSource(NacosSourceConfig{
		ServerAddr: "localhost:8848",
		DataID:    "myapp.yaml",
		Group:    "DEFAULT_GROUP",
	})
	if err != nil {
		t.Fatalf("failed to create NacosSource: %v", err)
	}

	name := src.Name()
	expected := "nacos:DEFAULT_GROUP/myapp.yaml"
	if name != expected {
		t.Errorf("expected name %q, got %q", expected, name)
	}
}

// TestNacosSourceValidation tests validation of required fields.
func TestNacosSourceValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  NacosSourceConfig
		wantErr bool
	}{
		{
			name:    "missing ServerAddr",
			config:  NacosSourceConfig{DataID: "test.yaml"},
			wantErr: true,
		},
		{
			name:    "missing DataID",
			config:  NacosSourceConfig{ServerAddr: "localhost:8848"},
			wantErr: true,
		},
		{
			name:    "valid config",
			config:  NacosSourceConfig{ServerAddr: "localhost:8848", DataID: "test.yaml"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewNacosSource(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewNacosSource() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestNacosSourceCompileTimeAssertion ensures the type implements the right interfaces.
func TestNacosSourceCompileTimeAssertion(t *testing.T) {
	// Compile-time assertion: ensure NacosSource implements Source and Watchable.
	var _ Source = (*NacosSource)(nil)
	var _ Watchable = (*NacosSource)(nil)
	t.Log("compile-time assertion passed")
}

// TestNacosSourceDefaults tests that defaults are applied correctly.
func TestNacosSourceDefaults(t *testing.T) {
	src, err := NewNacosSource(NacosSourceConfig{
		ServerAddr: "localhost:8848",
		DataID:    "test.yaml",
	})
	if err != nil {
		t.Fatalf("failed to create NacosSource: %v", err)
	}

	// These fields should have defaults applied
	if src.cfg.Namespace == "" {
		t.Error("expected default namespace, got empty")
	}
	if src.cfg.Group == "" {
		t.Error("expected default group, got empty")
	}
	if src.cfg.Format != YAMLFormat {
		t.Errorf("expected default format YAMLFormat, got %v", src.cfg.Format)
	}
}