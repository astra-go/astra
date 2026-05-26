//go:build integration

package orm_test

// Integration tests for orm/clickhouse.
//
// Two modes are supported:
//
//  1. CI mode — set CLICKHOUSE_DSN to point at an externally managed container:
//     CLICKHOUSE_DSN=clickhouse://default:@localhost:9000/testdb \
//       go test -tags integration -v ./e2e/orm/...
//
//  2. Local-dev mode (testcontainers) — no env-var needed; Docker must be available:
//     go test -tags integration -v ./e2e/orm/...

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	chtc "github.com/testcontainers/testcontainers-go/modules/clickhouse"

	"github.com/astra-go/astra/orm/clickhouse"
)

// containerDSN is set by TestMain and shared across all tests.
var containerDSN string

// TestMain prepares the ClickHouse DSN. If CLICKHOUSE_DSN is set (CI mode)
// that value is used directly; otherwise a testcontainers-managed container
// is started for the duration of the test run.
func TestMain(m *testing.M) {
	if dsn := os.Getenv("CLICKHOUSE_DSN"); dsn != "" {
		containerDSN = dsn
		os.Exit(m.Run())
	}

	// --- testcontainers local-dev path ---
	ctx := context.Background()

	ctr, err := chtc.Run(ctx,
		"clickhouse/clickhouse-server:24-alpine",
		chtc.WithDatabase("testdb"),
		chtc.WithUsername("default"),
		chtc.WithPassword(""),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "testcontainers: start ClickHouse: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = ctr.Terminate(ctx) }()

	dsn, err := ctr.ConnectionString(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "testcontainers: ClickHouse DSN: %v\n", err)
		os.Exit(1)
	}
	containerDSN = dsn

	os.Exit(m.Run())
}

// ─── Open / connectivity ──────────────────────────────────────────────────────

func TestIntegration_Open_RealServer(t *testing.T) {
	db, err := clickhouse.Open(clickhouse.Config{DSN: containerDSN})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	sqlDB, _ := db.DB()
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

// ─── DDL + batch insert + query ──────────────────────────────────────────────

type event struct {
	EventDate time.Time `gorm:"column:event_date"`
	UserID    uint64    `gorm:"column:user_id"`
	Action    string    `gorm:"column:action"`
}

func (event) TableName() string { return "astra_test_events" }

func TestIntegration_CRUD(t *testing.T) {
	db, err := clickhouse.Open(clickhouse.Config{DSN: containerDSN})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS astra_test_events (
			event_date  Date,
			user_id     UInt64,
			action      String
		) ENGINE = MergeTree()
		ORDER BY (event_date, user_id)
	`).Error; err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}
	t.Cleanup(func() {
		db.Exec("DROP TABLE IF EXISTS astra_test_events")
	})

	rows := []event{
		{EventDate: time.Now(), UserID: 1, Action: "login"},
		{EventDate: time.Now(), UserID: 2, Action: "purchase"},
		{EventDate: time.Now(), UserID: 3, Action: "logout"},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("Create: %v", err)
	}

	// ClickHouse MergeTree parts are merged asynchronously.
	time.Sleep(300 * time.Millisecond)

	var results []event
	if err := db.Where("action = ?", "login").Find(&results).Error; err != nil {
		t.Fatalf("Find: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected at least one row with action=login")
	}
}

// TestIntegration_BatchInsert verifies that creating many rows in one call
// succeeds and all rows are queryable (exercises the bulk-insert path).
func TestIntegration_BatchInsert(t *testing.T) {
	db, err := clickhouse.Open(clickhouse.Config{DSN: containerDSN})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS astra_batch_events (
			event_date  Date,
			user_id     UInt64,
			action      String
		) ENGINE = MergeTree()
		ORDER BY (event_date, user_id)
	`).Error; err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}
	t.Cleanup(func() {
		db.Exec("DROP TABLE IF EXISTS astra_batch_events")
	})

	const n = 100
	batch := make([]event, n)
	for i := range batch {
		batch[i] = event{EventDate: time.Now(), UserID: uint64(i), Action: fmt.Sprintf("act-%d", i)}
	}
	if err := db.Table("astra_batch_events").Create(&batch).Error; err != nil {
		t.Fatalf("batch Create: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	var count int64
	if err := db.Table("astra_batch_events").Count(&count).Error; err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != n {
		t.Errorf("expected %d rows, got %d", n, count)
	}
}

// TestIntegration_RawQuery exercises a raw SQL query with a parameter.
func TestIntegration_RawQuery(t *testing.T) {
	db, err := clickhouse.Open(clickhouse.Config{DSN: containerDSN})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS astra_raw_events (
			event_date  Date,
			user_id     UInt64,
			action      String
		) ENGINE = MergeTree()
		ORDER BY (event_date, user_id)
	`).Error; err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}
	t.Cleanup(func() { db.Exec("DROP TABLE IF EXISTS astra_raw_events") })

	rows := []event{
		{EventDate: time.Now(), UserID: 10, Action: "view"},
		{EventDate: time.Now(), UserID: 11, Action: "view"},
		{EventDate: time.Now(), UserID: 12, Action: "click"},
	}
	db.Table("astra_raw_events").Create(&rows)
	time.Sleep(300 * time.Millisecond)

	var count int64
	db.Table("astra_raw_events").Where("action = ?", "view").Count(&count)
	if count != 2 {
		t.Errorf("expected 2 'view' rows, got %d", count)
	}
}

// ─── Connection pool settings ─────────────────────────────────────────────────

func TestIntegration_PoolSettings(t *testing.T) {
	db, err := clickhouse.Open(clickhouse.Config{
		DSN:             containerDSN,
		MaxOpenConns:    3,
		MaxIdleConns:    1,
		ConnMaxLifetime: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	sqlDB, _ := db.DB()
	stats := sqlDB.Stats()
	if stats.MaxOpenConnections != 3 {
		t.Errorf("MaxOpenConns: want 3, got %d", stats.MaxOpenConnections)
	}
}

// ─── Edge cases ───────────────────────────────────────────────────────────────

// TestIntegration_CreateTable_Idempotent verifies IF NOT EXISTS is idempotent.
func TestIntegration_CreateTable_Idempotent(t *testing.T) {
	db, err := clickhouse.Open(clickhouse.Config{DSN: containerDSN})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	ddl := `CREATE TABLE IF NOT EXISTS astra_idem (id UInt64) ENGINE = MergeTree() ORDER BY id`
	t.Cleanup(func() { db.Exec("DROP TABLE IF EXISTS astra_idem") })

	for i := 0; i < 3; i++ {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("idempotent CREATE on attempt %d: %v", i+1, err)
		}
	}
}

// TestIntegration_QueryEmptyTable verifies that querying an empty table returns
// zero rows without error (ClickHouse returns an empty result set, not an error).
func TestIntegration_QueryEmptyTable(t *testing.T) {
	db, err := clickhouse.Open(clickhouse.Config{DSN: containerDSN})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := db.Exec(`CREATE TABLE IF NOT EXISTS astra_empty (id UInt64) ENGINE = MergeTree() ORDER BY id`).Error; err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}
	t.Cleanup(func() { db.Exec("DROP TABLE IF EXISTS astra_empty") })

	var rows []struct{ ID uint64 }
	if err := db.Table("astra_empty").Find(&rows).Error; err != nil {
		t.Fatalf("Find on empty table: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(rows))
	}
}
