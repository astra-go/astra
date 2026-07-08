package config

import (
	"os"
	"testing"
	"time"
)

func TestBaseServiceConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     BaseServiceConfig
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: BaseServiceConfig{
				Port: "8080",
				Mode: "dev",
			},
			wantErr: false,
		},
		{
			name: "missing port",
			cfg: BaseServiceConfig{
				Mode: "dev",
			},
			wantErr: true,
		},
		{
			name: "invalid mode",
			cfg: BaseServiceConfig{
				Port: "8080",
				Mode: "invalid",
			},
			wantErr: true,
		},
		{
			name: "invalid storage backend",
			cfg: BaseServiceConfig{
				Port:            "8080",
				Mode:            "dev",
				StorageBackend:  "invalid",
			},
			wantErr: true,
		},
		{
			name: "negative shutdown timeout",
			cfg: BaseServiceConfig{
				Port:            "8080",
				Mode:            "dev",
				ShutdownTimeout: -1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBaseServiceConfig_ModeChecks(t *testing.T) {
	cfg := BaseServiceConfig{Mode: "dev"}
	if !cfg.IsDev() {
		t.Error("IsDev() should be true")
	}
	if cfg.IsProd() {
		t.Error("IsProd() should be false")
	}
	if cfg.IsStaging() {
		t.Error("IsStaging() should be false")
	}

	cfg.Mode = "prod"
	if cfg.IsDev() {
		t.Error("IsDev() should be false")
	}
	if !cfg.IsProd() {
		t.Error("IsProd() should be true")
	}

	cfg.Mode = "staging"
	if !cfg.IsStaging() {
		t.Error("IsStaging() should be true")
	}
}

func TestBaseServiceConfig_SlogLevel(t *testing.T) {
	tests := []struct {
		logLevel string
		want     string
	}{
		{"debug", "DEBUG"},
		{"info", "INFO"},
		{"warn", "WARN"},
		{"warning", "WARN"},
		{"error", "ERROR"},
		{"unknown", "INFO"},
	}

	for _, tt := range tests {
		t.Run(tt.logLevel, func(t *testing.T) {
			cfg := BaseServiceConfig{LogLevel: tt.logLevel}
			got := cfg.SlogLevel().String()
			if got != tt.want {
				t.Errorf("SlogLevel() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBaseServiceConfig_ResolvedLogFormat(t *testing.T) {
	tests := []struct {
		mode      string
		logFormat string
		want      string
	}{
		{"dev", "auto", "text"},
		{"prod", "auto", "json"},
		{"staging", "auto", "json"},
		{"dev", "json", "json"},
		{"prod", "text", "text"},
	}

	for _, tt := range tests {
		t.Run(tt.mode+"_"+tt.logFormat, func(t *testing.T) {
			cfg := BaseServiceConfig{Mode: tt.mode, LogFormat: tt.logFormat}
			got := cfg.ResolvedLogFormat()
			if got != tt.want {
				t.Errorf("ResolvedLogFormat() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDatabaseConfig_BuildDSN(t *testing.T) {
	tests := []struct {
		name string
		cfg  DatabaseConfig
		want string
	}{
		{
			name: "prebuilt DSN",
			cfg:  DatabaseConfig{DSN: "postgres://user:pass@localhost:5432/db"},
			want: "postgres://user:pass@localhost:5432/db",
		},
		{
			name: "build from fields",
			cfg: DatabaseConfig{
				Host:     "localhost",
				Port:     5432,
				User:     "user",
				Password: "pass",
				DBName:   "mydb",
				SSLMode:  "disable",
			},
			want: "host=localhost port=5432 user=user password=pass dbname=mydb sslmode=disable",
		},
		{
			name: "empty host",
			cfg:  DatabaseConfig{Port: 5432},
			want: "",
		},
		{
			name: "default sslmode",
			cfg: DatabaseConfig{
				Host:   "localhost",
				Port:   5432,
				User:   "user",
				DBName: "mydb",
			},
			want: "host=localhost port=5432 user=user password= dbname=mydb sslmode=disable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.BuildDSN()
			if got != tt.want {
				t.Errorf("BuildDSN() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSecurityConfig_Defaults(t *testing.T) {
	cfg := SecurityConfig{}
	if got := cfg.GetMaxAttempts(); got != 5 {
		t.Errorf("GetMaxAttempts() = %v, want 5", got)
	}
	if got := cfg.GetLockDuration(); got != 30*time.Minute {
		t.Errorf("GetLockDuration() = %v, want 30m", got)
	}

	cfg.AccountLockMaxAttempts = 10
	cfg.AccountLockDuration = time.Hour
	if got := cfg.GetMaxAttempts(); got != 10 {
		t.Errorf("GetMaxAttempts() = %v, want 10", got)
	}
	if got := cfg.GetLockDuration(); got != time.Hour {
		t.Errorf("GetLockDuration() = %v, want 1h", got)
	}
}

func TestLoadWithAstra_Defaults(t *testing.T) {
	// No config file, no env vars → should return defaults
	type TestConfig struct {
		BaseServiceConfig `yaml:",inline" json:",inline"`
	}

	cfg, svc, err := LoadWithAstra[TestConfig]("test-service", "/nonexistent", "TEST")
	if err != nil {
		t.Fatalf("LoadWithAstra() error = %v", err)
	}

	// No sources → mgr is nil
	if cfg != nil {
		t.Error("expected nil Config when no sources")
	}

	// Defaults should be applied
	if svc.Name != "unknown-service" {
		t.Errorf("Name = %v, want unknown-service", svc.Name)
	}
	if svc.Port != "8080" {
		t.Errorf("Port = %v, want 8080", svc.Port)
	}
	if svc.Mode != "dev" {
		t.Errorf("Mode = %v, want dev", svc.Mode)
	}
}

func TestLoadWithAstra_EnvOverride(t *testing.T) {
	// Set env vars
	os.Setenv("TC_PORT", "9999")
	os.Setenv("TC_MODE", "prod")
	os.Setenv("TC_LOG_LEVEL", "debug")
	defer os.Unsetenv("TC_PORT")
	defer os.Unsetenv("TC_MODE")
	defer os.Unsetenv("TC_LOG_LEVEL")

	type TestConfig struct {
		BaseServiceConfig `yaml:",inline" json:",inline"`
	}

	_, svc, err := LoadWithAstra[TestConfig]("test-service", "/nonexistent", "TC")
	if err != nil {
		t.Fatalf("LoadWithAstra() error = %v", err)
	}

	if svc.Port != "9999" {
		t.Errorf("Port = %v, want 9999", svc.Port)
	}
	if svc.Mode != "prod" {
		t.Errorf("Mode = %v, want prod", svc.Mode)
	}
	if svc.LogLevel != "debug" {
		t.Errorf("LogLevel = %v, want debug", svc.LogLevel)
	}
}
