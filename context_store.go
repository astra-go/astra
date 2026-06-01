package astra

// context_store.go — per-request key-value store methods for Ctx.
//
// The store starts as a []kvPair slice (linear scan, no mutex). When the number
// of entries exceeds kvStoreMapThreshold, it is promoted to a map[string]any
// for O(1) access. The slice is retained alongside the map so that reset() can
// clear the map by iterating the slice (avoiding a full map iteration).
// RouteKey ("/users/:id" template) bypasses both structures via a dedicated
// string field on Ctx to avoid string→any interface boxing on every request.

// RouteKey is the context store key for the matched route template (e.g. "/users/:id").
// Read by middleware/metrics.go and middleware/tracing.go for low-cardinality labels.
const RouteKey = "astra.route"

// kvStoreMapThreshold is the number of entries above which Set/Get switch from
// linear slice scan to map lookup. Chosen to be larger than the typical
// middleware chain depth (~8–12 keys) while still benefiting from the map for
// unusually deep chains.
const kvStoreMapThreshold = 16

// ─── Context Store ───────────────────────────────────────────────────────────
//
// The key-value store uses a []kvPair slice (kvStore) that grows on demand via
// append and is reset to [:0] on each request to retain the backing array.
//
// Design constraints:
//   - No mutex: Ctx belongs to a single goroutine for the lifetime of a request.
//     Handlers that spawn goroutines must copy values out before launching them.
//   - Adaptive: for ≤kvStoreMapThreshold entries, linear slice scan is faster
//     than map hashing and keeps the working set cache-hot. Above the threshold,
//     kvMap is populated and all subsequent lookups use the map.
//   - RouteKey is a dedicated string field (c.routeKey) that bypasses
//     the store entirely to avoid string→any interface boxing.

// Set stores a key-value pair in the per-request context store.
// If the key already exists it is updated in-place (no duplicate entries).
func (c *Ctx) Set(key string, value any) {
	c.debugCheckConcurrency()

	if key == RouteKey {
		if s, ok := value.(string); ok {
			c.routeKey = s
		}
		return
	}

	if c.kvMap != nil {
		// Map mode: update slice entry in-place for reset() efficiency, then map.
		for i := range c.kvStore {
			if c.kvStore[i].key == key {
				c.kvStore[i].value = value
				c.kvMap[key] = value
				return
			}
		}
		c.kvStore = append(c.kvStore, kvPair{key: key, value: value})
		c.kvMap[key] = value
		return
	}

	for i := range c.kvStore {
		if c.kvStore[i].key == key {
			c.kvStore[i].value = value
			return
		}
	}
	c.kvStore = append(c.kvStore, kvPair{key: key, value: value})

	// Promote to map once the slice exceeds the threshold.
	if len(c.kvStore) > kvStoreMapThreshold {
		if c.kvMap == nil {
			c.kvMap = make(map[string]any, len(c.kvStore)*2)
		}
		for _, kv := range c.kvStore {
			c.kvMap[kv.key] = kv.value
		}
	}
}

// Get retrieves a value from the context store.
// Returns (value, true) if found, (nil, false) otherwise.
func (c *Ctx) Get(key string) (any, bool) {
	c.debugCheckConcurrency()

	if key == RouteKey {
		if c.routeKey != "" {
			return c.routeKey, true
		}
		return nil, false
	}

	if c.kvMap != nil {
		v, ok := c.kvMap[key]
		return v, ok
	}

	for i := range c.kvStore {
		if c.kvStore[i].key == key {
			return c.kvStore[i].value, true
		}
	}
	return nil, false
}

// MustGet retrieves a value and panics if not found.
func (c *Ctx) MustGet(key string) any {
	v, ok := c.Get(key)
	if !ok {
		panic("astra: key not found in context: " + key)
	}
	return v
}

// GetString retrieves a string value from context.
func (c *Ctx) GetString(key string) string {
	if key == RouteKey {
		return c.routeKey
	}
	v, _ := c.Get(key)
	s, _ := v.(string)
	return s
}

// GetInt retrieves an int value from context.
// If the stored value is not an int, returns 0. Use TryGetInt to distinguish
// "not found" from "found but wrong type".
func (c *Ctx) GetInt(key string) int {
	v, _ := c.Get(key)
	i, _ := v.(int)
	return i
}

// GetInt64 retrieves an int64 value from context.
// Middleware commonly stores numeric values as int64; this avoids the silent
// zero that GetInt returns when the stored type is int64 rather than int.
func (c *Ctx) GetInt64(key string) int64 {
	v, _ := c.Get(key)
	i, _ := v.(int64)
	return i
}

// GetFloat64 retrieves a float64 value from context.
func (c *Ctx) GetFloat64(key string) float64 {
	v, _ := c.Get(key)
	f, _ := v.(float64)
	return f
}

// GetBool retrieves a bool value from context.
// If the stored value is not a bool, returns false. Use TryGetBool to
// distinguish "not found" from "found but wrong type".
func (c *Ctx) GetBool(key string) bool {
	v, _ := c.Get(key)
	b, _ := v.(bool)
	return b
}

// TryGetString retrieves a string value and reports whether the key was present
// AND held a string. Unlike GetString, a (_, false) return distinguishes
// "key absent" from "key present but stored as a non-string type".
func (c *Ctx) TryGetString(key string) (string, bool) {
	v, ok := c.Get(key)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// TryGetInt retrieves an int value and reports whether the key was present AND
// held an int. Returns (0, false) for both missing keys and type mismatches.
func (c *Ctx) TryGetInt(key string) (int, bool) {
	v, ok := c.Get(key)
	if !ok {
		return 0, false
	}
	i, ok := v.(int)
	return i, ok
}

// TryGetBool retrieves a bool value and reports whether the key was present AND
// held a bool. Returns (false, false) for both missing keys and type mismatches.
func (c *Ctx) TryGetBool(key string) (bool, bool) {
	v, ok := c.Get(key)
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}
