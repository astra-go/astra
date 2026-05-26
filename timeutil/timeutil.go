// Package timeutil provides a unified time type and global timezone/format
// configuration for Astra applications.
//
// # Quick start
//
//	// Call once at application startup:
//	timeutil.MustSetTimezone("Asia/Shanghai")
//	timeutil.SetLayout("2006-01-02 15:04:05")
//
// # Time type
//
// [Time] wraps [time.Time] and implements:
//   - JSON marshal/unmarshal using the configured layout
//   - [database/sql/driver.Valuer] and [database/sql.Scanner] for GORM/SQL
//   - [encoding.TextMarshaler] / [encoding.TextUnmarshaler]
//
// A zero [Time] serializes to JSON null and stores as SQL NULL.
//
// # Input flexibility
//
// [UnmarshalJSON] accepts three formats:
//   - JSON null or empty string → zero Time
//   - Unquoted integer → Unix timestamp in seconds
//   - Quoted string → configured layout, with fallback cascade
package timeutil

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// ─── Layout constants ─────────────────────────────────────────────────────────

const (
	// DateLayout formats a date as "2006-01-02".
	DateLayout = "2006-01-02"

	// TimeLayout formats a time-of-day as "15:04:05".
	TimeLayout = "15:04:05"

	// DateTimeLayout formats a datetime as "2006-01-02 15:04:05".
	// This is the package default layout.
	DateTimeLayout = "2006-01-02 15:04:05"
)

// ─── Global configuration ─────────────────────────────────────────────────────

var globalCfg = struct {
	mu       sync.RWMutex
	location *time.Location
	layout   string
}{
	location: time.UTC,
	layout:   DateTimeLayout,
}

// cfg returns a consistent snapshot of the current location and layout under a
// single read lock, preventing TOCTOU between two separate accessor calls.
func cfg() (loc *time.Location, layout string) {
	globalCfg.mu.RLock()
	loc, layout = globalCfg.location, globalCfg.layout
	globalCfg.mu.RUnlock()
	return
}

// SetTimezone sets the global timezone by IANA name (e.g. "Asia/Shanghai",
// "America/New_York", "UTC"). Call once at application startup before serving
// requests. It is safe to call from multiple goroutines.
func SetTimezone(name string) error {
	loc, err := time.LoadLocation(name)
	if err != nil {
		return fmt.Errorf("timeutil: load timezone %q: %w", name, err)
	}
	globalCfg.mu.Lock()
	globalCfg.location = loc
	globalCfg.mu.Unlock()
	return nil
}

// MustSetTimezone is like [SetTimezone] but panics on an invalid timezone name.
func MustSetTimezone(name string) {
	if err := SetTimezone(name); err != nil {
		panic(err)
	}
}

// SetLayout sets the global date-time layout string used for JSON serialization
// and default string formatting. Use Go reference time format.
//
//	timeutil.SetLayout("2006-01-02 15:04:05")  // MySQL-style
//	timeutil.SetLayout(time.RFC3339)            // ISO 8601
func SetLayout(layout string) {
	globalCfg.mu.Lock()
	globalCfg.layout = layout
	globalCfg.mu.Unlock()
}

// Location returns the currently configured global timezone.
func Location() *time.Location {
	globalCfg.mu.RLock()
	defer globalCfg.mu.RUnlock()
	return globalCfg.location
}

// Layout returns the currently configured global date-time format layout.
func Layout() string {
	globalCfg.mu.RLock()
	defer globalCfg.mu.RUnlock()
	return globalCfg.layout
}

// ─── Time type ────────────────────────────────────────────────────────────────

// Time is a time.Time wrapper that serialises to/from JSON using the globally
// configured layout and timezone. It implements database/sql interfaces so it
// works transparently as a GORM model field.
//
// The zero value of Time (valid==false) represents an absent or null time and
// serializes as JSON null and SQL NULL.
type Time struct {
	t     time.Time
	valid bool // false = null / zero
}

// ─── Constructors ─────────────────────────────────────────────────────────────

// Now returns the current time in the globally configured timezone.
func Now() Time {
	loc, _ := cfg()
	return Time{t: time.Now().In(loc), valid: true}
}

// Unix returns the Time corresponding to the given Unix time, sec seconds
// since January 1, 1970 UTC. Note: Unix(0) is the epoch — a valid, non-null time.
func Unix(sec int64) Time {
	loc, _ := cfg()
	return Time{t: time.Unix(sec, 0).In(loc), valid: true}
}

// UnixMilli returns the Time corresponding to the given Unix time in milliseconds.
func UnixMilli(ms int64) Time {
	loc, _ := cfg()
	return Time{t: time.UnixMilli(ms).In(loc), valid: true}
}

// FromTime wraps a stdlib time.Time, converting it to the configured timezone.
// Passing a zero time.Time returns a zero (null) Time.
func FromTime(t time.Time) Time {
	if t.IsZero() {
		return Time{}
	}
	loc, _ := cfg()
	return Time{t: t.In(loc), valid: true}
}

// Parse parses s using the globally configured layout and timezone.
func Parse(s string) (Time, error) {
	_, layout := cfg()
	return ParseLayout(layout, s)
}

// ParseLayout parses s using the given layout and the configured timezone.
func ParseLayout(layout, s string) (Time, error) {
	loc, _ := cfg()
	t, err := time.ParseInLocation(layout, s, loc)
	if err != nil {
		return Time{}, fmt.Errorf("timeutil: parse %q with layout %q: %w", s, layout, err)
	}
	return Time{t: t, valid: true}, nil
}

// Today returns a Time at the start of today (00:00:00) in the configured timezone.
func Today() Time {
	loc, _ := cfg()
	now := time.Now().In(loc)
	y, m, d := now.Date()
	return Time{t: time.Date(y, m, d, 0, 0, 0, 0, loc), valid: true}
}

// ─── Accessors ────────────────────────────────────────────────────────────────

// Std returns the underlying stdlib time.Time.
func (t Time) Std() time.Time { return t.t }

// IsZero reports whether t is the zero (null) time.
func (t Time) IsZero() bool { return !t.valid }

// Unix returns t as a Unix timestamp in seconds.
func (t Time) Unix() int64 { return t.t.Unix() }

// UnixMilli returns t as a Unix timestamp in milliseconds.
func (t Time) UnixMilli() int64 { return t.t.UnixMilli() }

// String formats t using the globally configured layout.
// Returns "" for the zero Time.
func (t Time) String() string {
	return t.Format(Layout())
}

// Format formats t using the given layout string.
// Returns "" for the zero Time.
func (t Time) Format(layout string) string {
	if !t.valid {
		return ""
	}
	return t.t.Format(layout)
}

// Date formats t as "2006-01-02".
func (t Time) Date() string { return t.Format(DateLayout) }

// TimeOfDay formats t as "15:04:05".
func (t Time) TimeOfDay() string { return t.Format(TimeLayout) }

// ─── Comparison and arithmetic ────────────────────────────────────────────────

// Before reports whether t is before u.
func (t Time) Before(u Time) bool { return t.t.Before(u.t) }

// After reports whether t is after u.
func (t Time) After(u Time) bool { return t.t.After(u.t) }

// Equal reports whether t and u represent the same time instant.
func (t Time) Equal(u Time) bool { return t.t.Equal(u.t) }

// Add returns the time t+d. If t is zero, t is returned unchanged.
func (t Time) Add(d time.Duration) Time {
	if !t.valid {
		return t
	}
	return Time{t: t.t.Add(d), valid: true}
}

// Sub returns the duration t-u.
func (t Time) Sub(u Time) time.Duration { return t.t.Sub(u.t) }

// Truncate returns the result of rounding t down to a multiple of d.
// If t is zero, t is returned unchanged.
func (t Time) Truncate(d time.Duration) Time {
	if !t.valid {
		return t
	}
	return Time{t: t.t.Truncate(d), valid: true}
}

// ─── JSON ─────────────────────────────────────────────────────────────────────

// MarshalJSON implements json.Marshaler.
// Zero Time serializes as JSON null; otherwise as a quoted string in the
// configured layout. Uses AppendFormat to avoid extra allocations.
func (t Time) MarshalJSON() ([]byte, error) {
	if !t.valid {
		return []byte("null"), nil
	}
	_, layout := cfg()
	b := make([]byte, 0, len(layout)+2)
	b = append(b, '"')
	b = t.t.AppendFormat(b, layout)
	b = append(b, '"')
	return b, nil
}

// UnmarshalJSON implements json.Unmarshaler. Accepts:
//   - JSON null or empty string literal → zero Time
//   - Unquoted integer (positive or negative) → Unix timestamp in seconds
//   - Quoted string → tries configured layout, then RFC3339 / common fallbacks
func (t *Time) UnmarshalJSON(b []byte) error {
	s := string(b)

	// null or empty string literal → zero Time
	if s == "null" || s == `""` {
		*t = Time{}
		return nil
	}

	// Unquoted number (positive or negative Unix timestamp)
	if len(s) > 0 && (s[0] >= '0' && s[0] <= '9' || s[0] == '-') {
		var sec int64
		if err := json.Unmarshal(b, &sec); err != nil {
			return fmt.Errorf("timeutil: unmarshal unix timestamp: %w", err)
		}
		*t = Unix(sec)
		return nil
	}

	// Quoted string
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		inner := s[1 : len(s)-1]
		if inner == "" {
			*t = Time{}
			return nil
		}
		parsed, err := tryParseString(inner)
		if err != nil {
			return err
		}
		*t = Time{t: parsed, valid: true}
		return nil
	}

	return fmt.Errorf("timeutil: unexpected JSON token: %s", s)
}

// ─── encoding.TextMarshaler / TextUnmarshaler ─────────────────────────────────

// MarshalText implements encoding.TextMarshaler.
// Zero Time returns an empty byte slice (not "null").
func (t Time) MarshalText() ([]byte, error) {
	if !t.valid {
		return []byte{}, nil
	}
	_, layout := cfg()
	return []byte(t.t.Format(layout)), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (t *Time) UnmarshalText(b []byte) error {
	if len(b) == 0 {
		*t = Time{}
		return nil
	}
	parsed, err := tryParseString(string(b))
	if err != nil {
		return err
	}
	*t = Time{t: parsed, valid: true}
	return nil
}

// ─── database/sql ─────────────────────────────────────────────────────────────

// Value implements driver.Valuer.
// Returns nil for zero Time (SQL NULL) and time.Time for valid times.
// Returning time.Time (not a formatted string) ensures proper handling by
// MySQL, PostgreSQL, and SQLite drivers.
func (t Time) Value() (driver.Value, error) {
	if !t.valid {
		return nil, nil
	}
	return t.t, nil
}

// Scan implements sql.Scanner.
// Handles nil (→ zero Time), time.Time, []byte, string, and int64.
func (t *Time) Scan(v any) error {
	if v == nil {
		*t = Time{}
		return nil
	}
	switch val := v.(type) {
	case time.Time:
		*t = FromTime(val)
	case []byte:
		parsed, err := tryParseString(string(val))
		if err != nil {
			return err
		}
		*t = Time{t: parsed, valid: true}
	case string:
		parsed, err := tryParseString(val)
		if err != nil {
			return err
		}
		*t = Time{t: parsed, valid: true}
	case int64:
		*t = Unix(val)
	default:
		return fmt.Errorf("timeutil: cannot scan type %T into Time", v)
	}
	return nil
}

// ─── Internal helpers ─────────────────────────────────────────────────────────

// tryParseString attempts to parse s using a cascade of known layouts,
// starting with the configured layout followed by common fallbacks.
// Duplicate layouts are skipped automatically.
func tryParseString(s string) (time.Time, error) {
	loc, layout := cfg()

	// Build deduped layout list: configured first, then fallbacks.
	fallbacks := []string{
		time.RFC3339,
		time.RFC3339Nano,
		DateTimeLayout,
		DateLayout,
	}
	layouts := make([]string, 0, 1+len(fallbacks))
	layouts = append(layouts, layout)
	for _, fb := range fallbacks {
		if fb != layout {
			layouts = append(layouts, fb)
		}
	}

	var lastErr error
	for _, l := range layouts {
		if parsed, err := time.ParseInLocation(l, s, loc); err == nil {
			return parsed, nil
		} else {
			lastErr = err
		}
	}
	return time.Time{}, fmt.Errorf("timeutil: cannot parse %q (tried %d layouts): %w", s, len(layouts), lastErr)
}
