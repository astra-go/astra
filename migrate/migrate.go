// Package migrate provides a lightweight, code-first database migration runner.
//
// Migrations are defined as Go functions (not SQL files), giving full type
// safety and IDE support. Each migration has an ID, an Up function, and an
// optional Down function for rollback.
//
// # Defining migrations
//
//	var migrations = []*migrate.Migration{
//	    {
//	        ID: "001_create_users",
//	        Up: func(db *sql.DB) error {
//	            _, err := db.Exec(`CREATE TABLE IF NOT EXISTS users (
//	                id         BIGSERIAL PRIMARY KEY,
//	                email      TEXT      NOT NULL UNIQUE,
//	                created_at TIMESTAMP NOT NULL DEFAULT NOW()
//	            )`)
//	            return err
//	        },
//	        Down: func(db *sql.DB) error {
//	            _, err := db.Exec("DROP TABLE IF EXISTS users")
//	            return err
//	        },
//	    },
//	}
//
// # Running migrations
//
//	m := migrate.New(db)
//	m.Register(migrations...)
//
//	// Apply all pending migrations
//	if err := m.Up(ctx); err != nil { log.Fatal(err) }
//
//	// Rollback the latest applied migration
//	if err := m.Down(ctx); err != nil { log.Fatal(err) }
//
//	// Print status
//	statuses, _ := m.Status(ctx)
//	for _, s := range statuses {
//	    fmt.Printf("%s  applied=%v\n", s.ID, s.Applied)
//	}
//
// # Idempotency
//
// Write Up migrations to be idempotent where possible (CREATE TABLE IF NOT EXISTS,
// ADD COLUMN IF NOT EXISTS, etc.) because the tracking record and the schema
// change are not committed in a single atomic operation for databases that do
// not support transactional DDL (e.g. MySQL).
//
// # Concurrency
//
// Migrator is safe for concurrent reads after all migrations are registered,
// but Up/Down/Status should not be called concurrently — use an external lock
// (e.g. a database advisory lock) when running in a multi-instance environment.
package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"time"
)

var validTableName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// Migration defines a single database schema change.
type Migration struct {
	// ID uniquely identifies the migration. Use a sortable, descriptive format
	// such as "001_create_users" or "20240101_120000_add_email_index".
	// Migrations are applied in ascending lexicographic order of their IDs.
	ID string

	// Up applies the migration. Must be idempotent where possible.
	Up func(db *sql.DB) error

	// Down reverses the migration. May be nil for irreversible migrations.
	Down func(db *sql.DB) error
}

// Status describes the applied / pending state of one migration.
type Status struct {
	// ID is the migration identifier.
	ID string
	// Applied is true when the migration has been recorded in the tracking table.
	Applied bool
	// AppliedAt is the time the migration was applied (zero when not applied).
	AppliedAt time.Time
}

// Migrator manages and applies database migrations.
// Tracking state is stored in a table named "schema_migrations" by default.
type Migrator struct {
	db         *sql.DB
	migrations []*Migration
	table      string
}

// New creates a Migrator using db.
func New(db *sql.DB) *Migrator {
	return &Migrator{db: db, table: "schema_migrations"}
}

// WithTable overrides the tracking table name.
// The name must match ^[a-zA-Z_][a-zA-Z0-9_]*$ to prevent SQL injection.
func (m *Migrator) WithTable(table string) *Migrator {
	if !validTableName.MatchString(table) {
		panic(fmt.Sprintf("migrate: invalid table name %q: must match ^[a-zA-Z_][a-zA-Z0-9_]*$", table))
	}
	m.table = table
	return m
}

// Register adds migrations to the runner and sorts them by ID.
// Call Register once before calling Up/Down/Status.
func (m *Migrator) Register(migrations ...*Migration) {
	m.migrations = append(m.migrations, migrations...)
	sort.Slice(m.migrations, func(i, j int) bool {
		return m.migrations[i].ID < m.migrations[j].ID
	})
}

// ensureTable creates the tracking table if it does not already exist.
func (m *Migrator) ensureTable(ctx context.Context) error {
	_, err := m.db.ExecContext(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id         VARCHAR(255) NOT NULL PRIMARY KEY,
			applied_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`, m.table))
	if err != nil {
		return fmt.Errorf("migrate: create tracking table %q: %w", m.table, err)
	}
	return nil
}

// appliedSet returns a map of migration IDs that have already been applied.
func (m *Migrator) appliedSet(ctx context.Context) (map[string]time.Time, error) {
	rows, err := m.db.QueryContext(ctx,
		fmt.Sprintf("SELECT id, applied_at FROM %s", m.table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]time.Time)
	for rows.Next() {
		var id string
		var at time.Time
		if err := rows.Scan(&id, &at); err != nil {
			return nil, err
		}
		result[id] = at
	}
	return result, rows.Err()
}

// Up applies all pending migrations in ascending ID order.
// Each migration is applied and then recorded in the tracking table.
// Up stops and returns an error on the first failure.
func (m *Migrator) Up(ctx context.Context) error {
	if err := m.ensureTable(ctx); err != nil {
		return err
	}
	applied, err := m.appliedSet(ctx)
	if err != nil {
		return fmt.Errorf("migrate: query applied: %w", err)
	}

	pending := 0
	for _, mg := range m.migrations {
		if _, ok := applied[mg.ID]; ok {
			continue
		}
		pending++
		slog.Info("migrate: applying", slog.String("id", mg.ID))

		if err := mg.Up(m.db); err != nil {
			return fmt.Errorf("migrate: apply %q: %w", mg.ID, err)
		}

		// Record the migration. This is a separate operation from the migration
		// itself; for databases without transactional DDL (MySQL), the DDL is
		// already committed at this point.
		if _, err := m.db.ExecContext(ctx,
			fmt.Sprintf("INSERT INTO %s (id) VALUES (?)", m.table), mg.ID); err != nil {
			return fmt.Errorf("migrate: record %q: %w", mg.ID, err)
		}
		slog.Info("migrate: applied", slog.String("id", mg.ID))
	}

	if pending == 0 {
		slog.Info("migrate: no pending migrations")
	}
	return nil
}

// Down rolls back the most recently applied migration.
// If the latest migration has no Down function, it returns an error.
func (m *Migrator) Down(ctx context.Context) error {
	if err := m.ensureTable(ctx); err != nil {
		return err
	}
	applied, err := m.appliedSet(ctx)
	if err != nil {
		return fmt.Errorf("migrate: query applied: %w", err)
	}
	if len(applied) == 0 {
		slog.Info("migrate: nothing to roll back")
		return nil
	}

	// Find the latest applied migration (highest ID).
	var latest *Migration
	var latestID string
	for _, mg := range m.migrations {
		if _, ok := applied[mg.ID]; ok && mg.ID > latestID {
			latestID = mg.ID
			latest = mg
		}
	}
	if latest == nil {
		return nil
	}
	if latest.Down == nil {
		return fmt.Errorf("migrate: %q has no Down function; manual rollback required", latest.ID)
	}

	slog.Info("migrate: rolling back", slog.String("id", latest.ID))
	if err := latest.Down(m.db); err != nil {
		return fmt.Errorf("migrate: rollback %q: %w", latest.ID, err)
	}
	if _, err := m.db.ExecContext(ctx,
		fmt.Sprintf("DELETE FROM %s WHERE id = ?", m.table), latest.ID); err != nil {
		return fmt.Errorf("migrate: deregister %q: %w", latest.ID, err)
	}
	slog.Info("migrate: rolled back", slog.String("id", latest.ID))
	return nil
}

// Status returns the current applied/pending state of all registered migrations.
func (m *Migrator) Status(ctx context.Context) ([]Status, error) {
	if err := m.ensureTable(ctx); err != nil {
		return nil, err
	}
	applied, err := m.appliedSet(ctx)
	if err != nil {
		return nil, fmt.Errorf("migrate: query applied: %w", err)
	}

	out := make([]Status, len(m.migrations))
	for i, mg := range m.migrations {
		at, ok := applied[mg.ID]
		out[i] = Status{ID: mg.ID, Applied: ok, AppliedAt: at}
	}
	return out, nil
}
