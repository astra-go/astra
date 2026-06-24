package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

// ─── Mode helpers ─────────────────────────────────────────────────────────────

// keyAppMode is the canonical config key for the application mode.
const keyAppMode = "app.mode"

// Mode returns the application mode from the config (key "app.mode").
// Returns "dev" when not set.
func (c *Config) Mode() string {
	if m := c.GetString(keyAppMode); m != "" {
		return m
	}
	return "dev"
}

// IsDev reports whether the config mode is "dev" or not set.
func (c *Config) IsDev() bool { return c.Mode() == "dev" }

// IsProd reports whether the config mode is "prod".
func (c *Config) IsProd() bool { return c.Mode() == "prod" }

// IsStaging reports whether the config mode is "staging".
func (c *Config) IsStaging() bool { return c.Mode() == "staging" }

// IsTest reports whether the config mode is "test".
func (c *Config) IsTest() bool { return c.Mode() == "test" }

// IsStagingOrProd reports whether the config is a production-like mode.
func (c *Config) IsStagingOrProd() bool {
	m := c.Mode()
	return m == "prod" || m == "staging"
}

// ─── Optional file loading ────────────────────────────────────────────────────

// YAMLFileOptional loads from a YAML file if it exists; returns empty data on
// ENOTFOUND. All other I/O errors still propagate.
type YAMLFileOptional struct{ Path string }

func (s *YAMLFileOptional) Name() string     { return "yaml+optional:" + s.Path }
func (s *YAMLFileOptional) FilePath() string { return s.Path }

func (s *YAMLFileOptional) Load() (map[string]any, error) {
	b, err := os.ReadFile(s.Path)
	if err != nil {
		if os.IsNotExist(err) || os.IsPermission(err) {
			return make(map[string]any), nil
		}
		return nil, err
	}
	var data map[string]any
	if err := yaml.Unmarshal(b, &data); err != nil {
		return nil, fmt.Errorf("yaml parse: %w", err)
	}
	return data, nil
}

// JSONFileOptional loads from a JSON file if it exists; returns empty data on
// ENOTFOUND. All other I/O errors still propagate.
type JSONFileOptional struct{ Path string }

func (s *JSONFileOptional) Name() string     { return "json+optional:" + s.Path }
func (s *JSONFileOptional) FilePath() string { return s.Path }

func (s *JSONFileOptional) Load() (map[string]any, error) {
	b, err := os.ReadFile(s.Path)
	if err != nil {
		if os.IsNotExist(err) || os.IsPermission(err) {
			return make(map[string]any), nil
		}
		return nil, err
	}
	var data map[string]any
	if err := yaml.Unmarshal(b, &data); err != nil {
		// Try JSON fallback for .json files
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.UseNumber()
		if err := dec.Decode(&data); err != nil {
			return nil, fmt.Errorf("json parse: %w", err)
		}
	}
	return data, nil
}

// TOMLFileOptional loads from a TOML file if it exists; returns empty data on
// ENOTFOUND. All other I/O errors still propagate.
type TOMLFileOptional struct{ Path string }

func (s *TOMLFileOptional) Name() string     { return "toml+optional:" + s.Path }
func (s *TOMLFileOptional) FilePath() string { return s.Path }

func (s *TOMLFileOptional) Load() (map[string]any, error) {
	b, err := os.ReadFile(s.Path)
	if err != nil {
		if os.IsNotExist(err) || os.IsPermission(err) {
			return make(map[string]any), nil
		}
		return nil, err
	}
	var data map[string]any
	if err := toml.Unmarshal(b, &data); err != nil {
		return nil, fmt.Errorf("toml parse: %w", err)
	}
	return data, nil
}

// ─── Convention-based loading ────────────────────────────────────────────────

// LoadWithDefaults loads config using a convention-over-configuration approach.
//
// Resolution order:
//  1. Optional file: {configDir}/{serviceName}.yaml (if serviceName is non-empty)
//  2. Fallback file: {configDir}/{mode}.yaml        (mode from APP_MODE env or "dev")
//  3. Environment variables with the given prefix
//
// This mirrors the GMS `LoadWithDefault` pattern, allowing services to provide
// a defaults file per environment without per-service file duplication.
//
// Usage:
//
//	cfg, err := config.LoadWithDefaults("usercenter-svc", "./config", "USC")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println(cfg.Mode())     // "dev" | "prod"
//	fmt.Println(cfg.IsProd())   // true | false
func LoadWithDefaults(serviceName, configDir, envPrefix string) (*Config, error) {
	var sources []Source
	resolvedMode := os.Getenv("APP_MODE")
	if resolvedMode == "" {
		resolvedMode = "dev"
	}

	// 1. Service-specific config (highest file priority)
	if serviceName != "" && configDir != "" {
		serviceFile := filepath.Join(configDir, serviceName+".yaml")
		if _, err := os.Stat(serviceFile); err == nil {
			slog.Debug("config: loading service config file", "file", serviceFile)
			sources = append(sources, &YAMLFile{Path: serviceFile})
		} else {
			slog.Debug("config: no service-specific config file, trying mode file",
				"file", serviceFile, "err", err)
		}
	}

	// 2. Environment mode config (lower file priority)
	if configDir != "" {
		modeFile := filepath.Join(configDir, resolvedMode+".yaml")
		if _, err := os.Stat(modeFile); err == nil {
			slog.Debug("config: loading mode config file", "file", modeFile)
			sources = append(sources, &YAMLFile{Path: modeFile})
		} else {
			slog.Debug("config: no mode config file", "file", modeFile, "err", err)
		}
	}

	// 3. Environment variables (highest priority, applied last in New's merge)
	envPrefix = strings.TrimRight(envPrefix, "_")
	if envPrefix != "" {
		sources = append(sources, &Env{Prefix: envPrefix})
	}

	return New(sources...)
}
