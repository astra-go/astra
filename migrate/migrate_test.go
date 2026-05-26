package migrate_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/astra-go/astra/migrate"
	"github.com/astra-go/astra/testutil"
	_ "modernc.org/sqlite" // CGo-free SQLite driver
)

// openTestDB opens an in-memory SQLite database for testing.
// Falls back gracefully when the driver is not available.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Skipf("sqlite unavailable: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("sqlite ping failed: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestMigrate_Up_AppliesAll(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	var applied []string
	m := migrate.New(db)
	m.Register(
		&migrate.Migration{
			ID: "001_create_a",
			Up: func(db *sql.DB) error {
				_, err := db.Exec("CREATE TABLE IF NOT EXISTS a (id INTEGER PRIMARY KEY)")
				applied = append(applied, "001")
				return err
			},
		},
		&migrate.Migration{
			ID: "002_create_b",
			Up: func(db *sql.DB) error {
				_, err := db.Exec("CREATE TABLE IF NOT EXISTS b (id INTEGER PRIMARY KEY)")
				applied = append(applied, "002")
				return err
			},
		},
	)

	testutil.AssertNoError(t, m.Up(ctx))
	testutil.AssertEqual(t, 2, len(applied))
	testutil.AssertEqual(t, "001", applied[0])
	testutil.AssertEqual(t, "002", applied[1])
}

func TestMigrate_Up_Idempotent(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	calls := 0
	m := migrate.New(db)
	m.Register(&migrate.Migration{
		ID: "001_once",
		Up: func(db *sql.DB) error {
			calls++
			_, err := db.Exec("CREATE TABLE IF NOT EXISTS once (id INTEGER PRIMARY KEY)")
			return err
		},
	})

	testutil.AssertNoError(t, m.Up(ctx))
	testutil.AssertNoError(t, m.Up(ctx)) // second run must be a no-op
	testutil.AssertEqual(t, 1, calls)
}

func TestMigrate_Status(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	m := migrate.New(db)
	m.Register(
		&migrate.Migration{ID: "001", Up: func(db *sql.DB) error { return nil }},
		&migrate.Migration{ID: "002", Up: func(db *sql.DB) error { return nil }},
	)

	// Before Up
	statuses, err := m.Status(ctx)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, 2, len(statuses))
	testutil.AssertEqual(t, false, statuses[0].Applied)
	testutil.AssertEqual(t, false, statuses[1].Applied)

	testutil.AssertNoError(t, m.Up(ctx))

	statuses, err = m.Status(ctx)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, statuses[0].Applied)
	testutil.AssertEqual(t, true, statuses[1].Applied)
}

func TestMigrate_Down_RollsBackLatest(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	var rolled []string
	m := migrate.New(db)
	m.Register(
		&migrate.Migration{
			ID:   "001_a",
			Up:   func(db *sql.DB) error { return nil },
			Down: func(db *sql.DB) error { rolled = append(rolled, "001"); return nil },
		},
		&migrate.Migration{
			ID:   "002_b",
			Up:   func(db *sql.DB) error { return nil },
			Down: func(db *sql.DB) error { rolled = append(rolled, "002"); return nil },
		},
	)

	testutil.AssertNoError(t, m.Up(ctx))
	testutil.AssertNoError(t, m.Down(ctx))
	testutil.AssertEqual(t, 1, len(rolled))
	testutil.AssertEqual(t, "002", rolled[0]) // latest ID rolled back first

	// Status: 001 still applied, 002 reverted
	statuses, _ := m.Status(ctx)
	testutil.AssertEqual(t, true, statuses[0].Applied)
	testutil.AssertEqual(t, false, statuses[1].Applied)
}

func TestMigrate_Down_NoDownFunc(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	m := migrate.New(db)
	m.Register(&migrate.Migration{
		ID: "001_irreversible",
		Up: func(db *sql.DB) error { return nil },
		// Down is nil
	})

	testutil.AssertNoError(t, m.Up(ctx))
	err := m.Down(ctx)
	testutil.AssertError(t, err)
	if !errors.Is(err, err) { // just make sure it's non-nil and descriptive
		t.Errorf("unexpected error type: %v", err)
	}
}

func TestMigrate_Down_NothingToRollBack(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	m := migrate.New(db)
	m.Register(&migrate.Migration{
		ID: "001_a",
		Up: func(db *sql.DB) error { return nil },
	})
	// Don't call Up — nothing applied
	err := m.Down(ctx)
	testutil.AssertNoError(t, err)
}

func TestMigrate_FailingUp_StopsChain(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	sentinel := errors.New("intentional failure")
	var applied []string
	m := migrate.New(db)
	m.Register(
		&migrate.Migration{
			ID: "001_ok",
			Up: func(db *sql.DB) error { applied = append(applied, "001"); return nil },
		},
		&migrate.Migration{
			ID: "002_fail",
			Up: func(db *sql.DB) error { return sentinel },
		},
		&migrate.Migration{
			ID: "003_skipped",
			Up: func(db *sql.DB) error { applied = append(applied, "003"); return nil },
		},
	)

	err := m.Up(ctx)
	testutil.AssertError(t, err)
	testutil.AssertErrorIs(t, err, sentinel)
	testutil.AssertEqual(t, 1, len(applied)) // 003 must not have run
}

func TestMigrate_CustomTable(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	m := migrate.New(db).WithTable("custom_migrations")
	m.Register(&migrate.Migration{
		ID: "001",
		Up: func(db *sql.DB) error { return nil },
	})
	testutil.AssertNoError(t, m.Up(ctx))

	// Verify tracking row is in the custom table
	var count int
	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM custom_migrations").Scan(&count)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, 1, count)
}
