// Package marketplace provides a middleware registry, discovery, and
// configuration-driven loading system for Astra.
//
// Instead of manually importing and wiring middleware in main(), you register
// them with the Catalog and load them from config (YAML, JSON, env, etc.).
//
// # Quick start
//
//	// Register built-in middleware with the catalog
//	catalog := marketplace.NewCatalog()
//	middleware.RegisterBuiltins(catalog)
//
//	// Load from config
//	config := marketplace.Config{
//	    Middleware: []marketplace.MiddlewareEntry{
//	        {Name: "cors", Config: map[string]any{"origins": []string{"*"}}},
//	        {Name: "logger"},
//	        {Name: "recovery"},
//	    },
//	}
//	handlers, warnings := catalog.BuildChain(config)
//	app.Use(handlers...)
package marketplace

import (
	"fmt"
	"log/slog"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/astra-go/astra"
)

// ─── Catalog Entry ──────────────────────────────────────────────────────────

// MiddlewareDescriptor describes a middleware available in the catalog.
type MiddlewareDescriptor struct {
	Name        string   // Unique identifier (e.g. "cors", "jwt", "logger")
	Description string   // Human-readable one-liner
	Category    string   // Grouping: "security", "logging", "performance", "observability", "utility"
	Tags        []string // Searchable tags (e.g. ["rate-limit", "redis", "distributed"])
	ConfigType  string   // Config struct name (e.g. "CORSConfig", "JWTConfig")
	DefaultFn   func() any  // Returns a zero-value config struct for introspection
	Factory     FactoryFunc // Creates the middleware handler from config
}

// FactoryFunc builds an astra.HandlerFunc from a config value.
// The config param is the decoded config map/struct; nil means "use defaults".
type FactoryFunc func(config any) (astra.HandlerFunc, error)

// ─── Catalog ────────────────────────────────────────────────────────────────

// Catalog is a thread-safe registry of available middleware.
type Catalog struct {
	mu      sync.RWMutex
	entries map[string]*MiddlewareDescriptor
}

// NewCatalog creates an empty catalog.
func NewCatalog() *Catalog {
	return &Catalog{
		entries: make(map[string]*MiddlewareDescriptor),
	}
}

// Register adds a middleware descriptor. Panics on duplicate name.
func (c *Catalog) Register(desc *MiddlewareDescriptor) {
	c.mu.Lock()
	defer c.mu.Unlock()

	name := strings.ToLower(desc.Name)
	if _, ok := c.entries[name]; ok {
		panic(fmt.Sprintf("marketplace: duplicate middleware name %q", name))
	}
	desc.Name = name
	c.entries[name] = desc
}

// Lookup returns a descriptor by name, or nil.
func (c *Catalog) Lookup(name string) *MiddlewareDescriptor {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.entries[strings.ToLower(name)]
}

// List returns all registered middleware names, sorted.
func (c *Catalog) List() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	names := make([]string, 0, len(c.entries))
	for n := range c.entries {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Search returns middleware matching any of the given terms (case-insensitive).
// Terms match against name, category, description, and tags.
func (c *Catalog) Search(terms ...string) []*MiddlewareDescriptor {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var results []*MiddlewareDescriptor
	for _, desc := range c.entries {
		if matchDesc(desc, terms...) {
			results = append(results, desc)
		}
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Name < results[j].Name
	})
	return results
}

// ByCategory returns middleware grouped by category.
func (c *Catalog) ByCategory() map[string][]*MiddlewareDescriptor {
	c.mu.RLock()
	defer c.mu.RUnlock()

	groups := make(map[string][]*MiddlewareDescriptor)
	for _, desc := range c.entries {
		cat := desc.Category
		if cat == "" {
			cat = "uncategorized"
		}
		groups[cat] = append(groups[cat], desc)
	}
	for k := range groups {
		sort.Slice(groups[k], func(i, j int) bool {
			return groups[k][i].Name < groups[k][j].Name
		})
	}
	return groups
}

// Len returns the number of registered middleware.
func (c *Catalog) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

func matchDesc(desc *MiddlewareDescriptor, terms ...string) bool {
	for _, term := range terms {
		t := strings.ToLower(term)
		if strings.Contains(desc.Name, t) ||
			strings.Contains(desc.Category, t) ||
			strings.Contains(strings.ToLower(desc.Description), t) {
			return true
		}
		for _, tag := range desc.Tags {
			if strings.Contains(strings.ToLower(tag), t) {
				return true
			}
		}
	}
	return len(terms) == 0
}

// ─── Config-Driven Loading ───────────────────────────────────────────────────

// MiddlewareEntry represents one middleware instance in a config file.
type MiddlewareEntry struct {
	Name     string         // Must match a registered catalog name
	Config   map[string]any // Optional config key-values; nil → use defaults
	Disabled bool           // If true, skip this entry (default: false)
	Before   string         // Place before this named middleware (optional ordering hint)
	After    string         // Place after this named middleware (optional ordering hint)
}

// Config is the top-level middleware configuration structure.
type Config struct {
	Middleware []MiddlewareEntry
}

// BuildChain builds an ordered slice of astra.HandlerFunc from the config.
// Unknown middleware names are skipped with warnings; factory errors are returned.
// Entries with Enabled explicitly set to false are skipped.
func (c *Catalog) BuildChain(cfg Config) ([]astra.HandlerFunc, []string) {
	var handlers []astra.HandlerFunc
	var warnings []string

	// Resolve ordering hints into final order
	ordered := resolveOrder(cfg.Middleware)

	for _, entry := range ordered {
		if entry.Disabled {
			continue
		}

		desc := c.Lookup(entry.Name)
		if desc == nil {
			warnings = append(warnings, fmt.Sprintf("unknown middleware %q, skipped", entry.Name))
			continue
		}

		handler, err := desc.Factory(entry.Config)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("middleware %q: %v", entry.Name, err))
			continue
		}
		handlers = append(handlers, handler)
	}

	return handlers, warnings
}

// resolveOrder sorts middleware entries respecting Before/After hints.
// Entries without hints keep their original order.
func resolveOrder(entries []MiddlewareEntry) []MiddlewareEntry {
	type indexed struct {
		entry MiddlewareEntry
		index int
	}
	items := make([]indexed, len(entries))
	for i, e := range entries {
		items[i] = indexed{e, i}
	}

	// Simple stable sort with Before/After constraints
	// For complex DAGs, users should define exact order.
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i].entry, items[j].entry
		// a.Before == b.Name → a goes first
		if a.Before != "" && strings.EqualFold(a.Before, b.Name) {
			return true
		}
		// b.After == a.Name → a goes first
		if b.After != "" && strings.EqualFold(b.After, a.Name) {
			return true
		}
		// b.Before == a.Name → b goes first
		if b.Before != "" && strings.EqualFold(b.Before, a.Name) {
			return false
		}
		// a.After == b.Name → b goes first
		if a.After != "" && strings.EqualFold(a.After, b.Name) {
			return false
		}
		return items[i].index < items[j].index
	})

	result := make([]MiddlewareEntry, len(items))
	for i, item := range items {
		result[i] = item.entry
	}
	return result
}

// ─── Config → Struct Mapping ───────────────────────────────────────────────

// DecodeConfig fills a struct from a map[string]any using struct field names
// (case-insensitive) and snake_case → CamelCase mapping.
// This avoids a dependency on a heavy reflection library.
func DecodeConfig(data map[string]any, target any) error {
	if data == nil {
		return nil
	}

	v := reflect.ValueOf(target)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("marketplace: DecodeConfig target must be a pointer to struct")
	}

	s := v.Elem()
	t := s.Type()

	for i := 0; i < s.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		// Try exact name, lowercase name, and snake_case
		keys := []string{
			field.Name,
			strings.ToLower(field.Name),
			camelToSnake(field.Name),
		}

		for _, key := range keys {
			if val, ok := data[key]; ok {
				if err := setField(s.Field(i), reflect.ValueOf(val)); err != nil {
					return fmt.Errorf("marketplace: field %q: %w", field.Name, err)
				}
				break
			}
		}
	}

	return nil
}

func camelToSnake(s string) string {
	var b strings.Builder
	for i, c := range s {
		if i > 0 && c >= 'A' && c <= 'Z' {
			b.WriteByte('_')
		}
		b.WriteRune(c)
	}
	return strings.ToLower(b.String())
}

func setField(field, value reflect.Value) error {
	if field.CanSet() && field.Type() == value.Type() {
		field.Set(value)
		return nil
	}

	// Try type conversion for compatible types
	if field.Kind() == value.Kind() {
		if field.Type().ConvertibleTo(value.Type()) {
			field.Set(value.Convert(field.Type()))
			return nil
		}
	}

	slog.Debug("marketplace: skip field type mismatch",
		"field", field.Type().String(),
		"value", value.Type().String(),
	)
	return nil // non-fatal: skip fields with type mismatches
}
