package config

import (
	"context"
	"testing"
	"time"
)

// TestApolloSourceLoad tests the Load method of ApolloSource.
func TestApolloSourceLoad(t *testing.T) {
	// Skip if no Apollo server available
	t.Skip("Requires running Apollo server")

	src, err := NewApolloSource(ApolloSourceConfig{
		AppID:         "test-app",
		MetaAddr:      "http://localhost:8080",
		NamespaceName: "application",
		Cluster:       "default",
		Format:        YAMLFormat,
	})
	if err != nil {
		t.Fatalf("failed to create ApolloSource: %v", err)
	}

	data, err := src.Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	t.Logf("loaded config: %v", data)
}

// TestApolloSourceWatch tests the Watch method of ApolloSource.
func TestApolloSourceWatch(t *testing.T) {
	t.Skip("Requires running Apollo server")

	src, err := NewApolloSource(ApolloSourceConfig{
		AppID:         "test-app",
		MetaAddr:      "http://localhost:8080",
		NamespaceName: "application",
		Cluster:       "default",
		Format:        YAMLFormat,
	})
	if err != nil {
		t.Fatalf("failed to create ApolloSource: %v", err)
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

// TestApolloSourceName tests the Name method.
func TestApolloSourceName(t *testing.T) {
	src, err := NewApolloSource(ApolloSourceConfig{
		AppID:         "my-service",
		MetaAddr:      "http://localhost:8080",
		NamespaceName: "application",
	})
	if err != nil {
		t.Fatalf("failed to create ApolloSource: %v", err)
	}

	name := src.Name()
	expected := "apollo:my-service/application"
	if name != expected {
		t.Errorf("expected name %q, got %q", expected, name)
	}
}

// TestApolloSourceValidation tests validation of required fields.
func TestApolloSourceValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  ApolloSourceConfig
		wantErr bool
	}{
		{
			name:    "missing AppID",
			config:  ApolloSourceConfig{MetaAddr: "http://localhost:8080"},
			wantErr: true,
		},
		{
			name:    "missing MetaAddr",
			config:  ApolloSourceConfig{AppID: "test-app"},
			wantErr: true,
		},
		{
			name:    "valid config",
			config:  ApolloSourceConfig{AppID: "test-app", MetaAddr: "http://localhost:8080"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewApolloSource(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewApolloSource() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestApolloSourceCompileTimeAssertion ensures the type implements the right interfaces.
func TestApolloSourceCompileTimeAssertion(t *testing.T) {
	// Compile-time assertion: ensure ApolloSource implements Source and Watchable.
	var _ Source = (*ApolloSource)(nil)
	var _ Watchable = (*ApolloSource)(nil)
	t.Log("compile-time assertion passed")
}

// TestApolloSourceDefaults tests that defaults are applied correctly.
func TestApolloSourceDefaults(t *testing.T) {
	src, err := NewApolloSource(ApolloSourceConfig{
		AppID:    "test-app",
		MetaAddr: "http://localhost:8080",
	})
	if err != nil {
		t.Fatalf("failed to create ApolloSource: %v", err)
	}

	// These fields should have defaults applied
	if src.opts.Cluster == "" {
		t.Error("expected default cluster, got empty")
	}
	if src.opts.NamespaceName == "" {
		t.Error("expected default namespace, got empty")
	}
	if src.opts.Format != YAMLFormat {
		t.Errorf("expected default format YAMLFormat, got %v", src.opts.Format)
	}
}

// TestFlattenToNested verifies key flattening for nested config in Apollo.
func TestFlattenToNested(t *testing.T) {
	flat := map[string]any{
		"db.host": "localhost",
		"db.port": "5432",
		"db.name": "mydb",
	}

	result := flattenToNested(flat)

	// Verify nested structure
	if result["db"] == nil {
		t.Error("expected 'db' key in result")
		return
	}

	db := result["db"].(map[string]any)
	if db["host"] != "localhost" {
		t.Errorf("expected db.host=localhost, got %v", db["host"])
	}
	if db["port"] != "5432" {
		t.Errorf("expected db.port=5432, got %v", db["port"])
	}
	if db["name"] != "mydb" {
		t.Errorf("expected db.name=mydb, got %v", db["name"])
	}
}
