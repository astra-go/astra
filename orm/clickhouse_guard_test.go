package orm_test

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/migrator"
	"gorm.io/gorm/schema"

	"github.com/astra-go/astra/orm"
)

// ─── fake ClickHouse dialector ────────────────────────────────────────────────
//
// Implements gorm.Dialector with Name()="clickhouse" and a no-op Initialize so
// gorm.Open succeeds without a real server. All other methods are stubs that
// satisfy the interface but are never called in these tests.

type fakeClickHouseDialector struct{}

func (fakeClickHouseDialector) Name() string                                     { return "clickhouse" }
func (fakeClickHouseDialector) Initialize(_ *gorm.DB) error                      { return nil }
func (fakeClickHouseDialector) Migrator(_ *gorm.DB) gorm.Migrator                { return &migrator.Migrator{} }
func (fakeClickHouseDialector) DataTypeOf(_ *schema.Field) string                { return "" }
func (fakeClickHouseDialector) DefaultValueOf(_ *schema.Field) clause.Expression { return nil }
func (fakeClickHouseDialector) BindVarTo(_ clause.Writer, _ *gorm.Statement, _ any) {}
func (fakeClickHouseDialector) QuoteTo(_ clause.Writer, _ string)                {}
func (fakeClickHouseDialector) Explain(sql string, _ ...any) string              { return sql }

// fakeClickHouseDB returns a *gorm.DB whose Dialector.Name() == "clickhouse"
// without requiring a real ClickHouse server.
func fakeClickHouseDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(fakeClickHouseDialector{}, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("fakeClickHouseDB: gorm.Open: %v", err)
	}
	return db
}

// ─── TxMiddleware guard ───────────────────────────────────────────────────────

func TestTxMiddleware_PanicsOnClickHouse(t *testing.T) {
	db := fakeClickHouseDB(t)
	defer func() {
		r := recover()
		if r == nil {
			t.Error("TxMiddleware with ClickHouse should panic at registration time")
		}
	}()
	orm.TxMiddleware(db) // must panic
}

func TestTxMiddleware_DoesNotPanicOnSQLite(t *testing.T) {
	db := testDB(t) // SQLite
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("TxMiddleware with SQLite should not panic, got: %v", r)
		}
	}()
	orm.TxMiddleware(db)
}

// ─── RunTx / RunTxWithOptions guard ──────────────────────────────────────────

func TestRunTx_ReturnsErrorOnClickHouse(t *testing.T) {
	db := fakeClickHouseDB(t)
	err := orm.RunTx(context.Background(), db, func(_ *gorm.DB) error {
		t.Error("fn should never be called on ClickHouse")
		return nil
	})
	if !errors.Is(err, orm.ErrClickHouseTxNotSupported) {
		t.Errorf("want ErrClickHouseTxNotSupported, got %v", err)
	}
}

func TestRunTxWithOptions_ReturnsErrorOnClickHouse(t *testing.T) {
	db := fakeClickHouseDB(t)
	err := orm.RunTxWithOptions(context.Background(), db, nil, func(_ *gorm.DB) error {
		t.Error("fn should never be called on ClickHouse")
		return nil
	})
	if !errors.Is(err, orm.ErrClickHouseTxNotSupported) {
		t.Errorf("want ErrClickHouseTxNotSupported, got %v", err)
	}
}

// ─── SoftDeleteModel.SoftDelete guard ────────────────────────────────────────

func TestSoftDeleteModel_SoftDelete_ReturnsErrorOnClickHouse(t *testing.T) {
	db := fakeClickHouseDB(t)
	var p BlogPost
	err := p.SoftDelete(db, &p)
	if !errors.Is(err, orm.ErrClickHouseMutationNotSupported) {
		t.Errorf("want ErrClickHouseMutationNotSupported, got %v", err)
	}
}

func TestSoftDeleteModel_SoftDelete_DoesNotErrorOnSQLite(t *testing.T) {
	db := testDB(t, &BlogPost{})
	p := BlogPost{Body: "guard test"}
	if err := db.Create(&p).Error; err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := p.SoftDelete(db, &p); err != nil {
		t.Errorf("SoftDelete on SQLite should succeed, got %v", err)
	}
}
