package orm

import (
	"context"
	"log/slog"
	"testing"
)

func TestNPlusOneDetector_ExtractTable(t *testing.T) {
	d := NewNPlusOneDetector()

	tests := []struct {
		query    string
		expected string
	}{
		{"SELECT * FROM users", "USERS"},
		{"SELECT id, name FROM users WHERE id = 1", "USERS"},
		{"INSERT INTO users (name) VALUES ('test')", "USERS"},
		{"UPDATE users SET name = 'test' WHERE id = 1", "USERS"},
		{"DELETE FROM users WHERE id = 1", "USERS"},
		{"SELECT * FROM orders JOIN users ON orders.user_id = users.id", "ORDERS"}, // First table
		{"select * from products", "PRODUCTS"},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			result := d.extractTable(tt.query)
			if result != tt.expected {
				t.Errorf("extractTable(%q) = %q, want %q", tt.query, result, tt.expected)
			}
		})
	}
}

func TestNPlusOneDetector_Stats(t *testing.T) {
	d := NewNPlusOneDetector()

	stats := d.Stats()
	if stats.CachedPatterns != 0 {
		t.Errorf("expected 0 cached patterns, got %d", stats.CachedPatterns)
	}
}

func TestNPlusOneDetector_Reset(t *testing.T) {
	d := NewNPlusOneDetector()
	d.Reset()

	stats := d.Stats()
	if stats.CachedPatterns != 0 {
		t.Errorf("expected 0 after reset, got %d", stats.CachedPatterns)
	}
}

func TestQueryCounter_Count(t *testing.T) {
	c := NewQueryCounter()
	ctx := context.Background()

	count := c.Count(ctx)
	if count != 0 {
		t.Errorf("expected 0 count, got %d", count)
	}
}

func TestNewNPlusOneDetector_Defaults(t *testing.T) {
	d := NewNPlusOneDetector()

	if d.opts.LogLevel != slog.LevelWarn {
		t.Errorf("expected LogLevel=Warn, got %v", d.opts.LogLevel)
	}
	if d.opts.Threshold != 3 {
		t.Errorf("expected Threshold=3, got %d", d.opts.Threshold)
	}
	if d.opts.MaxCache != 1000 {
		t.Errorf("expected MaxCache=1000, got %d", d.opts.MaxCache)
	}
	if !d.opts.Enabled {
		t.Error("expected Enabled=true")
	}
}

func TestNPlusOneOptions_Funcs(t *testing.T) {
	o := DefaultNPlusOneOptions()

	WithNPlusOneLogLevel(slog.LevelDebug)(&o)
	if o.LogLevel != slog.LevelDebug {
		t.Errorf("expected LogLevel=Debug, got %v", o.LogLevel)
	}

	WithNPlusOneThreshold(5)(&o)
	if o.Threshold != 5 {
		t.Errorf("expected Threshold=5, got %d", o.Threshold)
	}

	WithNPlusOneMaxCache(500)(&o)
	if o.MaxCache != 500 {
		t.Errorf("expected MaxCache=500, got %d", o.MaxCache)
	}
}