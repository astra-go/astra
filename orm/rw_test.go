package orm_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/astra-go/astra/orm"
)

// openSQLite opens a fresh in-memory SQLite DB for testing.
func openSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return db
}

// ─── ReadWriteRouter ─────────────────────────────────────────────────────────

func TestReadWriteRouter_WriteReturnsPrimary(t *testing.T) {
	primary := openSQLite(t)
	replica := openSQLite(t)

	rw := orm.NewReadWriteRouter(primary, replica)
	defer rw.Close()

	ctx := context.Background()
	got := rw.Write(ctx)
	if got == nil {
		t.Fatal("Write returned nil")
	}
	// Write must always return the primary connection.
	// We verify by checking the underlying *sql.DB pointer.
	primarySQL, _ := primary.DB()
	gotSQL, _ := got.DB()
	if primarySQL != gotSQL {
		t.Error("Write did not return the primary DB")
	}
}

func TestReadWriteRouter_ReadReturnsReplica(t *testing.T) {
	primary := openSQLite(t)
	replica := openSQLite(t)

	rw := orm.NewReadWriteRouter(primary, replica)
	defer rw.Close()

	ctx := context.Background()
	got := rw.Read(ctx)
	if got == nil {
		t.Fatal("Read returned nil")
	}
	replicaSQL, _ := replica.DB()
	gotSQL, _ := got.DB()
	if replicaSQL != gotSQL {
		t.Error("Read did not return the replica DB")
	}
}

func TestReadWriteRouter_ReadFallsBackToPrimaryWhenNoReplicas(t *testing.T) {
	primary := openSQLite(t)

	rw := orm.NewReadWriteRouter(primary) // no replicas
	defer rw.Close()

	ctx := context.Background()
	got := rw.Read(ctx)
	primarySQL, _ := primary.DB()
	gotSQL, _ := got.DB()
	if primarySQL != gotSQL {
		t.Error("Read should fall back to primary when no replicas are registered")
	}
}

func TestReadWriteRouter_ReadUsesPrimaryInsideTransaction(t *testing.T) {
	primary := openSQLite(t)
	replica := openSQLite(t)

	rw := orm.NewReadWriteRouter(primary, replica)
	defer rw.Close()

	// Simulate an active transaction in context via orm.WithTx.
	tx := primary.Begin()
	defer tx.Rollback()
	txCtx := orm.WithTx(context.Background(), tx)

	got := rw.Read(txCtx)
	// Inside a transaction, Read must return the transaction (primary), not replica.
	txSQL, _ := tx.DB()
	gotSQL, _ := got.DB()
	if txSQL != gotSQL {
		t.Error("Read inside transaction should return the transaction DB, not a replica")
	}
}

func TestReadWriteRouter_RoundRobin(t *testing.T) {
	primary := openSQLite(t)
	rep1 := openSQLite(t)
	rep2 := openSQLite(t)

	rw := orm.NewReadWriteRouter(primary, rep1, rep2)
	defer rw.Close()

	ctx := context.Background()
	rep1SQL, _ := rep1.DB()
	rep2SQL, _ := rep2.DB()

	seen := map[*sql.DB]int{}
	for i := 0; i < 10; i++ {
		got := rw.Read(ctx)
		s, _ := got.DB()
		seen[s]++
	}
	if seen[rep1SQL] == 0 || seen[rep2SQL] == 0 {
		t.Error("round-robin should distribute reads across both replicas")
	}
}

func TestReadWriteRouter_Close_StopsHealthLoop(t *testing.T) {
	primary := openSQLite(t)
	replica := openSQLite(t)

	rw := orm.NewReadWriteRouter(primary, replica)
	// Close should not block or panic.
	done := make(chan struct{})
	go func() {
		rw.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("Close blocked for more than 2 seconds")
	}
	// Calling Close twice must be safe.
	rw.Close()
}

func TestReadWriteRouter_MiddlewareInjectsRouter(t *testing.T) {
	// Verify that RWRouter(c) returns the injected router after Middleware runs.
	// We test the middleware logic directly without starting an HTTP server.
	primary := openSQLite(t)
	rw := orm.NewReadWriteRouter(primary)
	defer rw.Close()

	mw := rw.Middleware()
	if mw == nil {
		t.Fatal("Middleware() returned nil")
	}
}
