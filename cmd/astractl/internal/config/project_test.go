package config

import (
	"testing"
)

func TestProjectConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  ProjectConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid simple layout with ORM",
			config: ProjectConfig{
				Name:     "myapp",
				Module:   "github.com/example/myapp",
				Layout:   "simple",
				Features: []string{"orm"},
				Database: "postgres",
			},
			wantErr: false,
		},
		{
			name: "valid ddd layout with cache and auth",
			config: ProjectConfig{
				Name:         "myapp",
				Module:       "github.com/example/myapp",
				Layout:       "ddd",
				Features:     []string{"cache", "auth"},
				CacheBackend: "redis",
				AuthMethod:   "jwt",
			},
			wantErr: false,
		},
		{
			name: "valid microservice template",
			config: ProjectConfig{
				Name:     "myapp",
				Module:   "github.com/example/myapp",
				Template: "microservice",
				Features: []string{"orm", "grpc", "mq"},
				Database: "mysql",
			},
			wantErr: false,
		},
		{
			name: "empty project name",
			config: ProjectConfig{
				Name:   "",
				Module: "github.com/example/myapp",
				Layout: "simple",
			},
			wantErr: true,
			errMsg:  "project name cannot be empty",
		},
		{
			name: "project name with slash",
			config: ProjectConfig{
				Name:   "my/app",
				Module: "github.com/example/myapp",
				Layout: "simple",
			},
			wantErr: true,
			errMsg:  "project name cannot contain /, \\, or .",
		},
		{
			name: "project name with backslash",
			config: ProjectConfig{
				Name:   "my\\app",
				Module: "github.com/example/myapp",
				Layout: "simple",
			},
			wantErr: true,
			errMsg:  "project name cannot contain /, \\, or .",
		},
		{
			name: "project name with dot",
			config: ProjectConfig{
				Name:   "my.app",
				Module: "github.com/example/myapp",
				Layout: "simple",
			},
			wantErr: true,
			errMsg:  "project name cannot contain /, \\, or .",
		},
		{
			name: "layout and template are mutually exclusive",
			config: ProjectConfig{
				Name:     "myapp",
				Module:   "github.com/example/myapp",
				Layout:   "simple",
				Template: "microservice",
			},
			wantErr: true,
			errMsg:  "--layout and --template are mutually exclusive",
		},
		{
			name: "invalid layout",
			config: ProjectConfig{
				Name:   "myapp",
				Module: "github.com/example/myapp",
				Layout: "invalid",
			},
			wantErr: true,
			errMsg:  "invalid layout: invalid (must be 'simple' or 'ddd')",
		},
		{
			name: "invalid template",
			config: ProjectConfig{
				Name:     "myapp",
				Module:   "github.com/example/myapp",
				Template: "invalid",
			},
			wantErr: true,
			errMsg:  "invalid template: invalid (must be 'microservice' or 'grpc-service')",
		},
		{
			name: "database without ORM feature",
			config: ProjectConfig{
				Name:     "myapp",
				Module:   "github.com/example/myapp",
				Layout:   "simple",
				Database: "postgres",
			},
			wantErr: true,
			errMsg:  "--db requires --with=orm",
		},
		{
			name: "invalid database type",
			config: ProjectConfig{
				Name:     "myapp",
				Module:   "github.com/example/myapp",
				Layout:   "simple",
				Features: []string{"orm"},
				Database: "mongodb",
			},
			wantErr: true,
			errMsg:  "invalid database: mongodb (must be 'postgres', 'mysql', or 'sqlite')",
		},
		{
			name: "cache backend without cache feature",
			config: ProjectConfig{
				Name:         "myapp",
				Module:       "github.com/example/myapp",
				Layout:       "simple",
				CacheBackend: "redis",
			},
			wantErr: true,
			errMsg:  "--cache requires --with=cache",
		},
		{
			name: "invalid cache backend",
			config: ProjectConfig{
				Name:         "myapp",
				Module:       "github.com/example/myapp",
				Layout:       "simple",
				Features:     []string{"cache"},
				CacheBackend: "memcached",
			},
			wantErr: true,
			errMsg:  "invalid cache backend: memcached (must be 'redis' or 'memory')",
		},
		{
			name: "auth method without auth feature",
			config: ProjectConfig{
				Name:       "myapp",
				Module:     "github.com/example/myapp",
				Layout:     "simple",
				AuthMethod: "jwt",
			},
			wantErr: true,
			errMsg:  "--auth-method requires --with=auth",
		},
		{
			name: "invalid auth method",
			config: ProjectConfig{
				Name:       "myapp",
				Module:     "github.com/example/myapp",
				Layout:     "simple",
				Features:   []string{"auth"},
				AuthMethod: "basic",
			},
			wantErr: true,
			errMsg:  "invalid auth method: basic (must be 'jwt' or 'oauth2')",
		},
		{
			name: "invalid feature",
			config: ProjectConfig{
				Name:     "myapp",
				Module:   "github.com/example/myapp",
				Layout:   "simple",
				Features: []string{"invalid"},
			},
			wantErr: true,
			errMsg:  "invalid feature: invalid (valid features: orm, cache, auth, grpc, mq)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("Validate() expected error but got nil")
					return
				}
				if err.Error() != tt.errMsg {
					t.Errorf("Validate() error message = %q, want %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestProjectConfig_HasORM(t *testing.T) {
	tests := []struct {
		name     string
		features []string
		want     bool
	}{
		{"with orm", []string{"orm"}, true},
		{"with orm and cache", []string{"orm", "cache"}, true},
		{"without orm", []string{"cache"}, false},
		{"empty features", []string{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &ProjectConfig{Features: tt.features}
			if got := c.HasORM(); got != tt.want {
				t.Errorf("HasORM() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProjectConfig_HasCache(t *testing.T) {
	tests := []struct {
		name     string
		features []string
		want     bool
	}{
		{"with cache", []string{"cache"}, true},
		{"with cache and orm", []string{"cache", "orm"}, true},
		{"without cache", []string{"orm"}, false},
		{"empty features", []string{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &ProjectConfig{Features: tt.features}
			if got := c.HasCache(); got != tt.want {
				t.Errorf("HasCache() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProjectConfig_HasAuth(t *testing.T) {
	tests := []struct {
		name     string
		features []string
		want     bool
	}{
		{"with auth", []string{"auth"}, true},
		{"with auth and orm", []string{"auth", "orm"}, true},
		{"without auth", []string{"orm"}, false},
		{"empty features", []string{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &ProjectConfig{Features: tt.features}
			if got := c.HasAuth(); got != tt.want {
				t.Errorf("HasAuth() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProjectConfig_HasGRPC(t *testing.T) {
	tests := []struct {
		name     string
		features []string
		want     bool
	}{
		{"with grpc", []string{"grpc"}, true},
		{"with grpc and orm", []string{"grpc", "orm"}, true},
		{"without grpc", []string{"orm"}, false},
		{"empty features", []string{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &ProjectConfig{Features: tt.features}
			if got := c.HasGRPC(); got != tt.want {
				t.Errorf("HasGRPC() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProjectConfig_HasMQ(t *testing.T) {
	tests := []struct {
		name     string
		features []string
		want     bool
	}{
		{"with mq", []string{"mq"}, true},
		{"with mq and orm", []string{"mq", "orm"}, true},
		{"without mq", []string{"orm"}, false},
		{"empty features", []string{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &ProjectConfig{Features: tt.features}
			if got := c.HasMQ(); got != tt.want {
				t.Errorf("HasMQ() = %v, want %v", got, tt.want)
			}
		})
	}
}
