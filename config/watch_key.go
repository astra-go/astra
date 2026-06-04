// Package config provides key-level watching for configuration changes.
//
// WatchKey allows watching for changes to a specific dot-separated key path
// rather than the entire config reload.
//
// # Usage
//
//	cfg.WatchKey("app.ratelimit.qps", func(oldVal, newVal string) {
//	    qps, _ := strconv.Atoi(newVal)
//	    rateLimiter.UpdateQPS(qps)
//	})
//
// WatchKey works with all Source types including remote sources (Nacos, Apollo, etcd, Consul).
// It internally uses the full config reload mechanism and compares values.
package config

import (
	"fmt"
	"strings"
)

// keyWatch stores information about a key-level watcher.
type keyWatch struct {
	key      string
	callback func(oldVal, newVal string)
}

// WatchKey registers a callback that fires only when the specified dot-separated
// key path changes value. It internally watches the full config reload and compares
// old vs new values.
//
// The callback receives the old value and new value as strings.
// If the key did not exist before, oldVal is empty.
// If the key does not exist now, newVal is empty.
//
// WatchKey is safer than Watch because it only fires when the specific key changes,
// reducing noise from unrelated config reloads.
//
// Example:
//
//	cfg.WatchKey("app.ratelimit.qps", func(oldVal, newVal string) {
//	    qps, _ := strconv.Atoi(newVal)
//	    rateLimiter.UpdateQPS(qps)
//	})
func (c *Config) WatchKey(key string, callback func(oldVal, newVal string)) {
	// Capture current value before registering the hook
	prevVal, _ := c.Get(key)

	// Register a hook that compares values
	c.Watch(func() {
		currVal, hasCurr := c.Get(key)
		if !hasCurr {
			currVal = ""
		}

		var prevStr string
		if prevVal == nil {
			prevStr = ""
		} else {
			prevStr = fmt.Sprintf("%v", prevVal)
		}

		currStr := fmt.Sprintf("%v", currVal)

		// Only call callback if value actually changed
		if prevStr != currStr {
			callback(prevStr, currStr)
		}

		// Update previous value
		prevVal = currVal
	})
}

// WatchKeyWithDefault registers a WatchKey with a default value that's used
// when the key is not present. This is useful for initializing
// components before the first config load.
func (c *Config) WatchKeyWithDefault(key string, defaultValue string, callback func(oldVal, newVal string)) {
	// Try to get current value, use default if not found
	currentVal, hasCurrent := c.Get(key)
	if !hasCurrent {
		currentVal = defaultValue
	}

	// Capture baseline
	baseline := fmt.Sprintf("%v", currentVal)

	// Register hook
	c.Watch(func() {
		currVal, hasCurr := c.Get(key)
		if !hasCurr {
			currVal = defaultValue
		}

		currStr := fmt.Sprintf("%v", currVal)

		// Only call callback if value actually changed
		if baseline != currStr {
			callback(baseline, currStr)
			baseline = currStr
		}
	})
}

// WatchKeyDistinct simplifies WatchKey by calling callback only with the new value.
// Use this when you don't need the old value (e.g., for simple reload triggers).
func (c *Config) WatchKeyDistinct(key string, callback func(newVal string)) {
	currentVal, _ := c.Get(key)
	currentStr := ""

	if currentVal != nil {
		currentStr = fmt.Sprintf("%v", currentVal)
	}

	// Strip leading dots and normalize
	key = strings.Trim(key, ".")

	callback(currentStr)

	c.Watch(func() {
		newVal, hasNew := c.Get(key)
		newStr := ""

		if newVal != nil {
			newStr = fmt.Sprintf("%v", newVal)
		}

		// Only trigger if the value is different from last time
		if currentStr != newStr {
			callback(newStr)
			currentStr = newStr
		}
	})
}
