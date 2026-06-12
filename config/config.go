// Package config provides flexible, multi-source configuration management for Astra.
//
// # Sources (loaded left-to-right, later sources override earlier ones)
//
//	config.YAMLFile{Path: "config.yaml"}
//	config.JSONFile{Path: "config.json"}
//	config.TOMLFile{Path: "config.toml"}
//	config.Env{Prefix: "APP"}          // APP__DB__PORT=5432 → db.port=5432
//	config.Memory{Data: map[string]any{...}}
//
// Remote sources (watching included) are in sub-packages:
//
//	config/etcd   — etcd v3 KV source
//	config/consul — Consul KV source
//
// # Hot reload
//
//	cfg.StartWatch(ctx)   // begins watching all file sources for changes
//	cfg.Watch(func() { ... })  // registered hook is called on every reload
//
// # Struct binding with defaults
//
//	type AppCfg struct {
//	    Port    int           `yaml:"port"    default:"8080"`
//	    Debug   bool          `yaml:"debug"   default:"false"`
//	    Timeout time.Duration `yaml:"timeout" default:"30s"`
//	}
//	var app AppCfg
//	cfg.Scan(&app)
package config

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"
)

// ─── Interfaces ───────────────────────────────────────────────────────────────

// Source is the interface for a configuration data source.
type Source interface {
	// Load reads and returns the current configuration as a nested map.
	Load() (map[string]any, error)
	// Name returns a human-readable identifier used in error messages.
	Name() string
}

// Watchable is optionally implemented by Sources that can push change
// notifications (typically remote sources like etcd or Consul).
// File-based sources use fsnotify internally; they do NOT implement Watchable.
type Watchable interface {
	Source
	// Watch starts watching for changes and calls notify whenever the source
	// may have changed. Runs until ctx is cancelled.
	Watch(ctx context.Context, notify func()) error
}

// filePath is optionally implemented by file-based sources so that StartWatch
// can register them with a single shared fsnotify.Watcher.
type filePath interface {
	FilePath() string
}

// ─── Config ───────────────────────────────────────────────────────────────────

// Config manages application configuration from multiple sources.
// All methods are safe for concurrent use.
type Config struct {
	mu      sync.RWMutex
	data    map[string]any
	sources []Source
	hooks   []func()
	hookSem chan struct{} // limits concurrent hook goroutines

	// hot-reload state
	watcherOnce   sync.Once
	watcherCancel context.CancelFunc
	watcherDone   chan struct{}
}

// New creates a Config, loading all sources immediately.
// Sources are merged left-to-right (later sources win on conflicts).
func New(sources ...Source) (*Config, error) {
	c := &Config{
		data:        make(map[string]any),
		sources:     sources,
		watcherDone: make(chan struct{}),
		hookSem:     make(chan struct{}, 8), // max 8 concurrent hook goroutines
	}
	close(c.watcherDone) // pre-close so StopWatch is safe before StartWatch
	if err := c.Load(); err != nil {
		return nil, err
	}
	return c, nil
}

// Load reloads all sources and notifies registered watchers.
// Safe to call from multiple goroutines.
func (c *Config) Load() error {
	c.mu.Lock()

	merged := make(map[string]any)
	for _, src := range c.sources {
		data, err := src.Load()
		if err != nil {
			c.mu.Unlock()
			return fmt.Errorf("config: source %q: %w", src.Name(), err)
		}
		mergeMaps(merged, data)
	}
	c.data = merged
	// Environment variable overrides always win (last applied = highest priority).
	applyEnvOverrides(c.data)

	hooks := make([]func(), len(c.hooks))
	copy(hooks, c.hooks)
	c.mu.Unlock()

	// Notify watchers outside the lock to avoid deadlocks if a hook calls Load.
	// Use a semaphore to limit concurrent hook goroutines and prevent goroutine
	// storms when config changes rapidly (e.g. batch file writes).
	for _, h := range hooks {
		c.hookSem <- struct{}{} // acquire semaphore slot
		go func(hook func()) {
			defer func() { <-c.hookSem }() // release slot
			safeHook(hook)
		}(h)
	}
	return nil
}

// Watch registers fn to be called in a new goroutine whenever the config is reloaded.
// fn must be safe for concurrent use.
func (c *Config) Watch(fn func()) {
	c.mu.Lock()
	c.hooks = append(c.hooks, fn)
	c.mu.Unlock()
}

// StartWatch begins watching all file-based sources and Watchable remote sources
// for changes. When any source changes, Load is called automatically.
//
// StartWatch is idempotent — calling it more than once is a no-op.
// Cancel ctx or call StopWatch to stop watching.
func (c *Config) StartWatch(ctx context.Context) error {
	var err error
	c.watcherOnce.Do(func() {
		err = c.startWatch(ctx)
	})
	return err
}

// StopWatch stops all background watchers started by StartWatch.
func (c *Config) StopWatch() {
	c.mu.RLock()
	cancel := c.watcherCancel
	c.mu.RUnlock()
	if cancel != nil {
		cancel()
		<-c.watcherDone
	}
}

func (c *Config) startWatch(parent context.Context) error {
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})

	c.mu.Lock()
	c.watcherCancel = cancel
	c.watcherDone = done
	c.mu.Unlock()

	// Collect file paths and Watchable remote sources.
	var paths []string
	var watchables []Watchable
	for _, src := range c.sources {
		if fp, ok := src.(filePath); ok {
			paths = append(paths, fp.FilePath())
		}
		if w, ok := src.(Watchable); ok {
			watchables = append(watchables, w)
		}
	}

	// Watch file paths with fsnotify.
	if len(paths) > 0 {
		fw, err := fsnotify.NewWatcher()
		if err != nil {
			cancel()
			close(done)
			return fmt.Errorf("config: create file watcher: %w", err)
		}
		for _, p := range paths {
			if err := fw.Add(p); err != nil {
				fw.Close()
				cancel()
				close(done)
				return fmt.Errorf("config: watch file %s: %w", p, err)
			}
		}
		go func() {
			c.runFileWatcher(ctx, fw)
		}()
	}

	// Watch Watchable remote sources.
	var wg sync.WaitGroup
	for _, w := range watchables {
		wg.Add(1)
		go func(ws Watchable) {
			defer wg.Done()
			if err := ws.Watch(ctx, func() {
				if err := c.Load(); err != nil {
					slog.Warn("config: reload failed after remote change",
						slog.String("source", ws.Name()),
						slog.String("err", err.Error()))
				}
			}); err != nil && ctx.Err() == nil {
				slog.Warn("config: remote watch error",
					slog.String("source", ws.Name()),
					slog.String("err", err.Error()))
			}
		}(w)
	}

	go func() {
		wg.Wait()
		close(done)
	}()

	return nil
}

// runFileWatcher drives the fsnotify loop with a debounce to handle
// editors that do atomic renames (write temp file → rename over target).
func (c *Config) runFileWatcher(ctx context.Context, fw *fsnotify.Watcher) {
	defer fw.Close()

	debounce := time.NewTimer(0)
	debounce.Stop()

	for {
		select {
		case <-ctx.Done():
			debounce.Stop()
			return

		case event, ok := <-fw.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				debounce.Reset(100 * time.Millisecond)
			}
			// Handle atomic writes: editor renames temp → target.
			// Re-add the path so the new inode is watched.
			if event.Has(fsnotify.Rename) || event.Has(fsnotify.Remove) {
				_ = fw.Add(event.Name)
				debounce.Reset(100 * time.Millisecond)
			}

		case err, ok := <-fw.Errors:
			if !ok {
				return
			}
			if err != nil {
				slog.Warn("config: file watcher error", slog.String("err", err.Error()))
			}

		case <-debounce.C:
			if err := c.Load(); err != nil {
				slog.Warn("config: reload failed after file change",
					slog.String("err", err.Error()))
			}
		}
	}
}

// safeHook calls h, recovering from any panic so a misbehaving hook cannot
// crash the server.
func safeHook(h func()) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("config: watch hook panicked", slog.Any("panic", r))
		}
	}()
	h()
}

// ─── Getters ─────────────────────────────────────────────────────────────────

// Get returns the raw value at the dot-separated key path.
func (c *Config) Get(key string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return getNestedKey(c.data, strings.Split(key, "."))
}

// GetString returns the string value at key, or "" if not set.
func (c *Config) GetString(key string) string {
	v, ok := c.Get(key)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

// GetInt returns the int value at key, or 0 if not set or unconvertible.
func (c *Config) GetInt(key string) int {
	v, ok := c.Get(key)
	if !ok {
		return 0
	}
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	case json.Number:
		n, _ := val.Int64()
		return int(n)
	case string:
		n, _ := strconv.Atoi(val)
		return n
	}
	return 0
}

// GetInt64 returns the int64 value at key, or 0 if not set.
// Preserves large integer values that would lose precision as float64.
func (c *Config) GetInt64(key string) int64 {
	v, ok := c.Get(key)
	if !ok {
		return 0
	}
	switch val := v.(type) {
	case int64:
		return val
	case int:
		return int64(val)
	case float64:
		return int64(val)
	case json.Number:
		n, _ := val.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(val, 10, 64)
		return n
	}
	return 0
}

// GetFloat64 returns the float64 value at key, or 0.0 if not set.
func (c *Config) GetFloat64(key string) float64 {
	v, ok := c.Get(key)
	if !ok {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case json.Number:
		f, _ := val.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(val, 64)
		return f
	}
	return 0
}

// GetBool returns the bool value at key, or false if not set.
func (c *Config) GetBool(key string) bool {
	v, ok := c.Get(key)
	if !ok {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case string:
		b, _ := strconv.ParseBool(val)
		return b
	}
	return false
}

// GetDuration returns the time.Duration at key.
// Accepts nanosecond integers or duration strings ("30s", "1m30s", "2h").
func (c *Config) GetDuration(key string) time.Duration {
	v, ok := c.Get(key)
	if !ok {
		return 0
	}
	switch val := v.(type) {
	case time.Duration:
		return val
	case int:
		return time.Duration(val)
	case int64:
		return time.Duration(val)
	case float64:
		return time.Duration(int64(val))
	case json.Number:
		if n, err := val.Int64(); err == nil {
			return time.Duration(n)
		}
	case string:
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return 0
}

// ─── Scan ─────────────────────────────────────────────────────────────────────

// Scan unmarshals the entire config into obj.
// Struct fields tagged with `default:"..."` receive their default value if the
// key is absent from all sources.
// Fields support yaml, json, and toml tags for name mapping.
func (c *Config) Scan(obj any) error {
	// Apply struct tag defaults first; json.Decode only overwrites keys it finds.
	applyDefaultTags(obj)

	c.mu.RLock()
	defer c.mu.RUnlock()

	b, err := json.Marshal(c.data)
	if err != nil {
		return fmt.Errorf("config: marshal: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	return dec.Decode(obj)
}

// ScanAndValidate unmarshals the entire config into obj and then validates it
// against the struct's `validate` tags. Returns the first error encountered
// (either a decode error or a ValidationErrors).
func (c *Config) ScanAndValidate(obj any) error {
	if err := c.Scan(obj); err != nil {
		return err
	}
	return Validate(obj)
}

// ScanKeyAndValidate unmarshals a sub-tree rooted at key into obj and then
// validates it against the struct's `validate` tags.
func (c *Config) ScanKeyAndValidate(key string, obj any) error {
	if err := c.ScanKey(key, obj); err != nil {
		return err
	}
	return Validate(obj)
}

// ScanKey unmarshals a sub-tree rooted at key into obj.
// Supports the same default tag semantics as Scan.
func (c *Config) ScanKey(key string, obj any) error {
	applyDefaultTags(obj)

	v, ok := c.Get(key)
	if !ok {
		// Key absent: still return the object with defaults applied.
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("config: marshal key %q: %w", key, err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	return dec.Decode(obj)
}

// ─── Default tag ─────────────────────────────────────────────────────────────

// applyDefaultTags walks obj (must be a struct pointer) and sets each field
// to its `default:"..."` tag value. Only zero-valued fields are set; non-zero
// fields are left unchanged so existing values are not overwritten.
func applyDefaultTags(obj any) {
	if obj == nil {
		return
	}
	rv := reflect.ValueOf(obj)
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return
	}
	rt := rv.Type()

	for i := range rt.NumField() {
		field := rt.Field(i)
		fv := rv.Field(i)

		if !fv.CanSet() {
			continue
		}

		// Recurse into nested structs.
		switch fv.Kind() {
		case reflect.Struct:
			if fv.CanAddr() {
				applyDefaultTags(fv.Addr().Interface())
			}
			continue
		case reflect.Ptr:
			if fv.Type().Elem().Kind() == reflect.Struct {
				if fv.IsNil() {
					fv.Set(reflect.New(fv.Type().Elem()))
				}
				applyDefaultTags(fv.Interface())
				continue
			}
		}

		tag := field.Tag.Get("default")
		if tag == "" || tag == "-" {
			continue
		}
		// Only apply if the field is currently zero to preserve any value already set.
		if !fv.IsZero() {
			continue
		}
		setDefaultValue(fv, field.Type, tag)
	}
}

// setDefaultValue parses tagVal and sets fv according to fv's Kind.
func setDefaultValue(fv reflect.Value, ft reflect.Type, tagVal string) {
	// Unwrap pointer.
	if ft.Kind() == reflect.Ptr {
		ptr := reflect.New(ft.Elem())
		setDefaultValue(ptr.Elem(), ft.Elem(), tagVal)
		fv.Set(ptr)
		return
	}

	// Special case: time.Duration
	if ft == reflect.TypeOf(time.Duration(0)) {
		if d, err := time.ParseDuration(tagVal); err == nil {
			fv.SetInt(int64(d))
		}
		return
	}

	switch ft.Kind() {
	case reflect.String:
		fv.SetString(tagVal)
	case reflect.Bool:
		if b, err := strconv.ParseBool(tagVal); err == nil {
			fv.SetBool(b)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if n, err := strconv.ParseInt(tagVal, 10, 64); err == nil {
			fv.SetInt(n)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if n, err := strconv.ParseUint(tagVal, 10, 64); err == nil {
			fv.SetUint(n)
		}
	case reflect.Float32, reflect.Float64:
		if f, err := strconv.ParseFloat(tagVal, 64); err == nil {
			fv.SetFloat(f)
		}
	}
}

// ─── File sources ─────────────────────────────────────────────────────────────

// YAMLFile loads configuration from a YAML file.
type YAMLFile struct {
	Path string
}

func (s *YAMLFile) Name() string     { return "yaml:" + s.Path }
func (s *YAMLFile) FilePath() string { return s.Path }

func (s *YAMLFile) Load() (map[string]any, error) {
	b, err := os.ReadFile(s.Path)
	if err != nil {
		return nil, err
	}
	var data map[string]any
	if err := yaml.Unmarshal(b, &data); err != nil {
		return nil, fmt.Errorf("yaml parse: %w", err)
	}
	return data, nil
}

// JSONFile loads configuration from a JSON file.
type JSONFile struct {
	Path string
}

func (s *JSONFile) Name() string     { return "json:" + s.Path }
func (s *JSONFile) FilePath() string { return s.Path }

func (s *JSONFile) Load() (map[string]any, error) {
	b, err := os.ReadFile(s.Path)
	if err != nil {
		return nil, err
	}
	var data map[string]any
	// UseNumber to preserve large integers.
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if err := dec.Decode(&data); err != nil {
		return nil, fmt.Errorf("json parse: %w", err)
	}
	return data, nil
}

// TOMLFile loads configuration from a TOML file.
type TOMLFile struct {
	Path string
}

func (s *TOMLFile) Name() string     { return "toml:" + s.Path }
func (s *TOMLFile) FilePath() string { return s.Path }

func (s *TOMLFile) Load() (map[string]any, error) {
	b, err := os.ReadFile(s.Path)
	if err != nil {
		return nil, err
	}
	var data map[string]any
	if err := toml.Unmarshal(b, &data); err != nil {
		return nil, fmt.Errorf("toml parse: %w", err)
	}
	return data, nil
}

// ─── Env source ───────────────────────────────────────────────────────────────

// Env loads configuration from environment variables.
// Double underscores "__" act as the nesting separator, and the key is lowercased.
//
//	APP__DB__PORT=5432  → {"db": {"port": "5432"}}   (with Prefix="APP")
//	APP__DEBUG=true     → {"debug": "true"}
type Env struct {
	// Prefix filters env vars. Only vars beginning with "<Prefix>_" are loaded.
	// The prefix itself is stripped from the key. If empty, all env vars are loaded.
	Prefix string
}

func (s *Env) Name() string { return "env:" + s.Prefix }

func (s *Env) Load() (map[string]any, error) {
	data := make(map[string]any)
	for _, env := range os.Environ() {
		k, v, ok := strings.Cut(env, "=")
		if !ok {
			continue
		}
		if s.Prefix != "" {
			upper := strings.ToUpper(s.Prefix) + "_"
			if !strings.HasPrefix(strings.ToUpper(k), upper) {
				continue
			}
			k = k[len(upper):]
		}
		// Normalise: "__" → ".", lowercase.
		k = strings.ToLower(strings.ReplaceAll(k, "__", "."))
		setNestedKey(data, strings.Split(k, "."), v)
	}
	return data, nil
}

// ─── Memory source ────────────────────────────────────────────────────────────

// Memory is an in-memory source for testing and programmatic configuration.
type Memory struct {
	Data map[string]any
}

func (s *Memory) Name() string { return "memory" }

func (s *Memory) Load() (map[string]any, error) {
	result := make(map[string]any, len(s.Data))
	for k, v := range s.Data {
		result[k] = v
	}
	return result, nil
}

// ─── Internal helpers ─────────────────────────────────────────────────────────

func getNestedKey(data map[string]any, keys []string) (any, bool) {
	if len(keys) == 0 {
		return nil, false
	}
	v, ok := data[keys[0]]
	if !ok {
		return nil, false
	}
	if len(keys) == 1 {
		return v, true
	}
	nested, ok := v.(map[string]any)
	if !ok {
		return nil, false
	}
	return getNestedKey(nested, keys[1:])
}

func setNestedKey(data map[string]any, keys []string, value any) {
	if len(keys) == 0 {
		return
	}
	if len(keys) == 1 {
		data[keys[0]] = value
		return
	}
	nested, ok := data[keys[0]].(map[string]any)
	if !ok {
		nested = make(map[string]any)
		data[keys[0]] = nested
	}
	setNestedKey(nested, keys[1:], value)
}

// mergeMaps deep-merges src into dst. src values overwrite dst values on conflict
// unless both values are maps, in which case they are recursively merged.
func mergeMaps(dst, src map[string]any) {
	for k, v := range src {
		if existing, ok := dst[k]; ok {
			if dstMap, ok := existing.(map[string]any); ok {
				if srcMap, ok := v.(map[string]any); ok {
					mergeMaps(dstMap, srcMap)
					continue
				}
			}
		}
		dst[k] = v
	}
}

// applyEnvOverrides overrides config values from environment variables.
// The env var name is the uppercase, underscore-joined key path.
// Example: config key "db.port" → env var "DB_PORT".
//
// Security note: this runs after all sources are merged, so env vars always
// win. This is intentional — it allows secrets to be injected via environment
// without appearing in config files.
func applyEnvOverrides(data map[string]any) {
	applyEnvFromMap(data, "")
}

func applyEnvFromMap(data map[string]any, prefix string) {
	for k, v := range data {
		fullKey := k
		if prefix != "" {
			fullKey = prefix + "_" + k
		}
		envKey := strings.ToUpper(fullKey)
		if envVal := os.Getenv(envKey); envVal != "" {
			data[k] = coerceEnvValue(v, envVal)
		}
		if nested, ok := v.(map[string]any); ok {
			applyEnvFromMap(nested, fullKey)
		}
	}
}

func coerceEnvValue(existing any, envVal string) any {
	if existing == nil {
		return envVal
	}
	switch reflect.TypeOf(existing).Kind() {
	case reflect.Bool:
		if b, err := strconv.ParseBool(envVal); err == nil {
			return b
		}
	case reflect.Int, reflect.Int64:
		if n, err := strconv.ParseInt(envVal, 10, 64); err == nil {
			return int(n)
		}
	case reflect.Float64:
		if f, err := strconv.ParseFloat(envVal, 64); err == nil {
			return f
		}
	}
	return envVal
}
