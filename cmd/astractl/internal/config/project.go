package config

import (
	"errors"
	"fmt"
	"strings"
)

// ProjectConfig represents the configuration for a new project.
type ProjectConfig struct {
	Name         string
	Module       string
	Layout       string   // "simple" | "ddd"
	Template     string   // "microservice" | "grpc-service" | ""
	Features     []string // ["orm", "cache", "auth", "grpc", "mq"]
	Database     string   // "postgres" | "mysql" | "sqlite"
	CacheBackend string   // "redis" | "memory"
	AuthMethod   string   // "jwt" | "oauth2"
}

// HasORM returns true if ORM feature is enabled.
func (c *ProjectConfig) HasORM() bool {
	return contains(c.Features, "orm")
}

// HasCache returns true if Cache feature is enabled.
func (c *ProjectConfig) HasCache() bool {
	return contains(c.Features, "cache")
}

// HasAuth returns true if Auth feature is enabled.
func (c *ProjectConfig) HasAuth() bool {
	return contains(c.Features, "auth")
}

// HasGRPC returns true if gRPC feature is enabled.
func (c *ProjectConfig) HasGRPC() bool {
	return contains(c.Features, "grpc")
}

// HasMQ returns true if Message Queue feature is enabled.
func (c *ProjectConfig) HasMQ() bool {
	return contains(c.Features, "mq")
}

// Validate checks if the project configuration is valid.
func (c *ProjectConfig) Validate() error {
	if c.Name == "" {
		return errors.New("project name cannot be empty")
	}

	if strings.ContainsAny(c.Name, "/\\.") {
		return errors.New("project name cannot contain /, \\, or .")
	}

	if c.Layout != "" && c.Template != "" {
		return errors.New("--layout and --template are mutually exclusive")
	}

	if c.Layout != "" && c.Layout != "simple" && c.Layout != "ddd" {
		return fmt.Errorf("invalid layout: %s (must be 'simple' or 'ddd')", c.Layout)
	}

	if c.Template != "" && c.Template != "microservice" && c.Template != "grpc-service" {
		return fmt.Errorf("invalid template: %s (must be 'microservice' or 'grpc-service')", c.Template)
	}

	if c.Database != "" && !c.HasORM() {
		return errors.New("--db requires --with=orm")
	}

	if c.Database != "" && c.Database != "postgres" && c.Database != "mysql" && c.Database != "sqlite" {
		return fmt.Errorf("invalid database: %s (must be 'postgres', 'mysql', or 'sqlite')", c.Database)
	}

	if c.CacheBackend != "" && !c.HasCache() {
		return errors.New("--cache requires --with=cache")
	}

	if c.CacheBackend != "" && c.CacheBackend != "redis" && c.CacheBackend != "memory" {
		return fmt.Errorf("invalid cache backend: %s (must be 'redis' or 'memory')", c.CacheBackend)
	}

	if c.AuthMethod != "" && !c.HasAuth() {
		return errors.New("--auth-method requires --with=auth")
	}

	if c.AuthMethod != "" && c.AuthMethod != "jwt" && c.AuthMethod != "oauth2" {
		return fmt.Errorf("invalid auth method: %s (must be 'jwt' or 'oauth2')", c.AuthMethod)
	}

	for _, feature := range c.Features {
		if !isValidFeature(feature) {
			return fmt.Errorf("invalid feature: %s (valid features: orm, cache, auth, grpc, mq)", feature)
		}
	}

	return nil
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func isValidFeature(feature string) bool {
	validFeatures := []string{"orm", "cache", "auth", "grpc", "mq"}
	return contains(validFeatures, feature)
}
