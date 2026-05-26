package clickhouse_test

import (
	"testing"

	"github.com/astra-go/astra/orm/clickhouse"
)

// ─── Open — validation (no server required) ──────────────────────────────────

func TestOpen_EmptyDSN_ReturnsError(t *testing.T) {
	_, err := clickhouse.Open(clickhouse.Config{})
	if err == nil {
		t.Fatal("expected error when DSN is empty")
	}
}

// TestOpen_BadDSN_ReturnsError verifies that an unreachable DSN returns an
// error from gorm.Open rather than panicking.
func TestOpen_BadDSN_ReturnsError(t *testing.T) {
	_, err := clickhouse.Open(clickhouse.Config{
		DSN: "clickhouse://127.0.0.1:9999/nonexistent",
	})
	if err == nil {
		t.Fatal("expected error for unreachable ClickHouse DSN")
	}
}

// ─── Config defaults ──────────────────────────────────────────────────────────

// TestConfig_ZeroValues_GetDefaults exercises the defaulting logic indirectly:
// we call Open with a non-empty DSN (which will fail at the ClickHouse dial),
// but the config defaults must be applied before the dial attempt.
// The test just ensures no panic occurs when default-setting runs.
func TestConfig_ZeroValues_NoPanic(t *testing.T) {
	// Open will fail (no server), but must not panic.
	_, _ = clickhouse.Open(clickhouse.Config{
		DSN:             "clickhouse://127.0.0.1:9999/db",
		MaxOpenConns:    0, // should default to 5
		MaxIdleConns:    0, // should default to 2
		ConnMaxLifetime: 0, // should default to 1h
	})
}
