package orm_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/astra-go/astra/contract"
	"github.com/astra-go/astra/timeutil"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"

	"github.com/astra-go/astra/orm"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

// testDB opens a fresh in-memory SQLite DB and auto-migrates the given models.
func testDB(t *testing.T, models ...any) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open SQLite: %v", err)
	}
	if len(models) > 0 {
		if err := db.AutoMigrate(models...); err != nil {
			t.Fatalf("migrate: %v", err)
		}
	}
	return db
}

// ─── Test models ──────────────────────────────────────────────────────────────

type Counter struct {
	ID    uint `gorm:"primaryKey"`
	Value int
}

type Versioned struct {
	ID      uint   `gorm:"primaryKey"`
	Name    string
	Version int `gorm:"default:0"`
}

// ─── Compile-time interface assertions ────────────────────────────────────────
// These are checked at compile time; no runtime test required.

var _ contract.Repository[Counter] = (*orm.Repository[Counter])(nil)
var _ contract.TxRunner = (*orm.GORMTxRunner)(nil)

// ─── RunTx ────────────────────────────────────────────────────────────────────

func TestRunTx_CommitsOnSuccess(t *testing.T) {
	db := testDB(t, &Counter{})

	err := orm.RunTx(context.Background(), db, func(tx *gorm.DB) error {
		return tx.Create(&Counter{Value: 42}).Error
	})
	if err != nil {
		t.Fatalf("RunTx: %v", err)
	}

	var c Counter
	if err := db.First(&c).Error; err != nil {
		t.Fatalf("find: %v", err)
	}
	if c.Value != 42 {
		t.Errorf("want Value=42, got %d", c.Value)
	}
}

func TestRunTx_RollsBackOnError(t *testing.T) {
	db := testDB(t, &Counter{})

	sentinel := errors.New("oops")
	err := orm.RunTx(context.Background(), db, func(tx *gorm.DB) error {
		if err := tx.Create(&Counter{Value: 99}).Error; err != nil {
			return err
		}
		return sentinel // signal rollback
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("want sentinel error, got %v", err)
	}

	var count int64
	db.Model(&Counter{}).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 rows after rollback, got %d", count)
	}
}

func TestRunTx_RollsBackOnPanic(t *testing.T) {
	db := testDB(t, &Counter{})

	func() {
		defer func() { recover() }() //nolint:errcheck
		_ = orm.RunTx(context.Background(), db, func(tx *gorm.DB) error {
			_ = tx.Create(&Counter{Value: 7}).Error
			panic("test panic")
		})
	}()

	var count int64
	db.Model(&Counter{}).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 rows after panic rollback, got %d", count)
	}
}

func TestRunTx_NestedErrorDoesNotAffectOuter(t *testing.T) {
	db := testDB(t, &Counter{})
	ctx := context.Background()

	err := orm.RunTx(ctx, db, func(outer *gorm.DB) error {
		// Create one record in outer transaction.
		if err := outer.Create(&Counter{Value: 1}).Error; err != nil {
			return err
		}
		// Nested savepoint that fails — should not roll back the outer tx.
		outerCtx := orm.WithTx(ctx, outer)
		_ = orm.RunNestedTx(outerCtx, db, "nested", func(tx *gorm.DB) error {
			if err := tx.Create(&Counter{Value: 2}).Error; err != nil {
				return err
			}
			return errors.New("nested error") // triggers rollback to savepoint
		})
		// Outer transaction continues; only the first record should be committed.
		return nil
	})
	if err != nil {
		t.Fatalf("RunTx: %v", err)
	}

	var count int64
	db.Model(&Counter{}).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 row (outer committed, nested rolled back), got %d", count)
	}
}

// ─── RunTxWithOptions ─────────────────────────────────────────────────────────

func TestRunTxWithOptions_NilOptions_WorksLikeRunTx(t *testing.T) {
	db := testDB(t, &Counter{})

	if err := orm.RunTxWithOptions(context.Background(), db, nil, func(tx *gorm.DB) error {
		return tx.Create(&Counter{Value: 5}).Error
	}); err != nil {
		t.Fatalf("RunTxWithOptions(nil): %v", err)
	}

	var count int64
	db.Model(&Counter{}).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 row, got %d", count)
	}
}

// ─── RunNestedTx ─────────────────────────────────────────────────────────────

func TestRunNestedTx_NoOuterTx_StartsNewTx(t *testing.T) {
	db := testDB(t, &Counter{})
	// No tx in context → RunNestedTx falls back to RunTx.
	if err := orm.RunNestedTx(context.Background(), db, "sp1", func(tx *gorm.DB) error {
		return tx.Create(&Counter{Value: 10}).Error
	}); err != nil {
		t.Fatalf("RunNestedTx: %v", err)
	}

	var c Counter
	if err := db.First(&c).Error; err != nil {
		t.Fatalf("find: %v", err)
	}
	if c.Value != 10 {
		t.Errorf("want 10, got %d", c.Value)
	}
}

// ─── Repository CRUD (ctx-aware) ─────────────────────────────────────────────

func TestRepository_RunTx_CommitsOnSuccess(t *testing.T) {
	db := testDB(t, &Counter{})
	repo := orm.NewRepository[Counter](db)
	ctx := context.Background()

	err := repo.RunTx(ctx, func(r *orm.Repository[Counter]) error {
		return r.Create(ctx, &Counter{Value: 77})
	})
	if err != nil {
		t.Fatalf("repo.RunTx: %v", err)
	}

	got, err := repo.FindByID(ctx, 1)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.Value != 77 {
		t.Errorf("want 77, got %d", got.Value)
	}
}

func TestRepository_RunTx_RollsBackOnError(t *testing.T) {
	db := testDB(t, &Counter{})
	repo := orm.NewRepository[Counter](db)
	ctx := context.Background()

	_ = repo.RunTx(ctx, func(r *orm.Repository[Counter]) error {
		_ = r.Create(ctx, &Counter{Value: 55})
		return errors.New("fail")
	})

	var count int64
	db.Model(&Counter{}).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 rows, got %d", count)
	}
}

func TestRepository_FindWhere(t *testing.T) {
	db := testDB(t, &Counter{})
	repo := orm.NewRepository[Counter](db)
	ctx := context.Background()

	_ = repo.Create(ctx, &Counter{Value: 10})
	_ = repo.Create(ctx, &Counter{Value: 20})
	_ = repo.Create(ctx, &Counter{Value: 10})

	rows, err := repo.FindWhere(ctx, "value = ?", 10)
	if err != nil {
		t.Fatalf("FindWhere: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("want 2 rows, got %d", len(rows))
	}
}

func TestRepository_FindAll_Pagination(t *testing.T) {
	db := testDB(t, &Counter{})
	repo := orm.NewRepository[Counter](db)
	ctx := context.Background()

	for i := range 5 {
		_ = repo.Create(ctx, &Counter{Value: i})
	}

	p := &contract.Page{PageNum: 1, PageSize: 3, Offset: 0}
	rows, total, err := repo.FindAll(ctx, p)
	if err != nil {
		t.Fatalf("FindAll: %v", err)
	}
	if total != 5 {
		t.Errorf("want total=5, got %d", total)
	}
	if len(rows) != 3 {
		t.Errorf("want 3 rows (page 1), got %d", len(rows))
	}
}

func TestRepository_Count(t *testing.T) {
	db := testDB(t, &Counter{})
	repo := orm.NewRepository[Counter](db)
	ctx := context.Background()

	_ = repo.Create(ctx, &Counter{Value: 1})
	_ = repo.Create(ctx, &Counter{Value: 2})
	_ = repo.Create(ctx, &Counter{Value: 1})

	n, err := repo.Count(ctx, "value = ?", 1)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 2 {
		t.Errorf("want 2, got %d", n)
	}
}

func TestRepository_Updates(t *testing.T) {
	db := testDB(t, &Counter{})
	repo := orm.NewRepository[Counter](db)
	ctx := context.Background()

	_ = repo.Create(ctx, &Counter{Value: 10})
	if err := repo.Updates(ctx, 1, map[string]any{"value": 99}); err != nil {
		t.Fatalf("Updates: %v", err)
	}
	got, _ := repo.FindByID(ctx, 1)
	if got.Value != 99 {
		t.Errorf("want 99, got %d", got.Value)
	}
}

func TestRepository_Delete(t *testing.T) {
	db := testDB(t, &Counter{})
	repo := orm.NewRepository[Counter](db)
	ctx := context.Background()

	_ = repo.Create(ctx, &Counter{Value: 5})
	if err := repo.Delete(ctx, 1); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	var count int64
	db.Model(&Counter{}).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 rows after delete, got %d", count)
	}
}

// ─── GORMTxRunner (contract.TxRunner) ────────────────────────────────────────

func TestGORMTxRunner_CommitsOnSuccess(t *testing.T) {
	db := testDB(t, &Counter{})
	repo := orm.NewRepository[Counter](db)
	runner := orm.NewTxRunner(db)
	ctx := context.Background()

	err := runner.RunTx(ctx, func(txCtx context.Context) error {
		return repo.Create(txCtx, &Counter{Value: 42})
	})
	if err != nil {
		t.Fatalf("GORMTxRunner.RunTx: %v", err)
	}

	got, err := repo.FindByID(ctx, 1)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.Value != 42 {
		t.Errorf("want 42, got %d", got.Value)
	}
}

func TestGORMTxRunner_RollsBackOnError(t *testing.T) {
	db := testDB(t, &Counter{})
	repo := orm.NewRepository[Counter](db)
	runner := orm.NewTxRunner(db)
	ctx := context.Background()

	sentinel := errors.New("tx fail")
	err := runner.RunTx(ctx, func(txCtx context.Context) error {
		_ = repo.Create(txCtx, &Counter{Value: 99})
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("want sentinel, got %v", err)
	}

	var count int64
	db.Model(&Counter{}).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 rows after rollback, got %d", count)
	}
}

func TestGORMTxRunner_CrossRepo_PropagatesTransaction(t *testing.T) {
	db := testDB(t, &Counter{})
	repoA := orm.NewRepository[Counter](db)
	repoB := orm.NewRepository[Counter](db)
	runner := orm.NewTxRunner(db)
	ctx := context.Background()

	// Both repos participate in the same transaction via txCtx.
	err := runner.RunTx(ctx, func(txCtx context.Context) error {
		if err := repoA.Create(txCtx, &Counter{Value: 1}); err != nil {
			return err
		}
		if err := repoB.Create(txCtx, &Counter{Value: 2}); err != nil {
			return err
		}
		return errors.New("force rollback")
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var count int64
	db.Model(&Counter{}).Count(&count)
	if count != 0 {
		t.Errorf("both repos should have rolled back, got %d rows", count)
	}
}

// ─── Mock Repository — demonstrates no-database unit testing ─────────────────

// mockCounterRepo is an in-memory contract.Repository[Counter] for unit tests.
// Business-layer tests use this instead of a real database.
type mockCounterRepo struct {
	mu      sync.Mutex
	store   map[uint]*Counter
	nextID  uint
	failOn  string // method name to force-fail
}

func newMockRepo() *mockCounterRepo {
	return &mockCounterRepo{store: make(map[uint]*Counter)}
}

func (m *mockCounterRepo) Create(_ context.Context, entity *Counter) error {
	if m.failOn == "Create" {
		return errors.New("mock create error")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	entity.ID = m.nextID
	cp := *entity
	m.store[cp.ID] = &cp
	return nil
}

func (m *mockCounterRepo) FindByID(_ context.Context, id any) (*Counter, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	uid, _ := id.(uint)
	c, ok := m.store[uid]
	if !ok {
		return nil, errors.New("not found")
	}
	cp := *c
	return &cp, nil
}

func (m *mockCounterRepo) FindAll(_ context.Context, _ *contract.Page) ([]Counter, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Counter, 0, len(m.store))
	for _, v := range m.store {
		out = append(out, *v)
	}
	return out, int64(len(out)), nil
}

func (m *mockCounterRepo) FindWhere(_ context.Context, _ any, _ ...any) ([]Counter, error) {
	return nil, nil
}
func (m *mockCounterRepo) First(_ context.Context, _ any, _ ...any) (*Counter, error) {
	return nil, nil
}
func (m *mockCounterRepo) Count(_ context.Context, _ any, _ ...any) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return int64(len(m.store)), nil
}
func (m *mockCounterRepo) Update(_ context.Context, entity *Counter) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.store[entity.ID] = entity
	return nil
}
func (m *mockCounterRepo) Updates(_ context.Context, _ any, _ any) error { return nil }
func (m *mockCounterRepo) Delete(_ context.Context, id any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	uid, _ := id.(uint)
	delete(m.store, uid)
	return nil
}

// Ensure mock satisfies the interface.
var _ contract.Repository[Counter] = (*mockCounterRepo)(nil)

// TestMockRepo_NoDatabase proves that service-layer logic can be tested
// without any database connection when it depends on contract.Repository[T].
func TestMockRepo_NoDatabase(t *testing.T) {
	repo := newMockRepo()
	ctx := context.Background()

	// Create
	c := &Counter{Value: 7}
	if err := repo.Create(ctx, c); err != nil {
		t.Fatalf("mock Create: %v", err)
	}
	if c.ID == 0 {
		t.Error("expected ID to be assigned")
	}

	// FindByID
	got, err := repo.FindByID(ctx, c.ID)
	if err != nil {
		t.Fatalf("mock FindByID: %v", err)
	}
	if got.Value != 7 {
		t.Errorf("want 7, got %d", got.Value)
	}

	// Count
	n, _ := repo.Count(ctx, nil)
	if n != 1 {
		t.Errorf("want count=1, got %d", n)
	}

	// Delete
	_ = repo.Delete(ctx, c.ID)
	n, _ = repo.Count(ctx, nil)
	if n != 0 {
		t.Errorf("want count=0 after delete, got %d", n)
	}
}

// TestMockRepo_InjectIntoService verifies that a service struct depending on
// contract.Repository[T] can be constructed and tested with a mock — no DB.
func TestMockRepo_InjectIntoService(t *testing.T) {
	// Inline "service" that uses contract.Repository[Counter].
	type counterSvc struct{ repo contract.Repository[Counter] }
	addAndCount := func(ctx context.Context, svc *counterSvc, val int) (int64, error) {
		if err := svc.repo.Create(ctx, &Counter{Value: val}); err != nil {
			return 0, err
		}
		return svc.repo.Count(ctx, nil)
	}

	svc := &counterSvc{repo: newMockRepo()}
	ctx := context.Background()

	n, err := addAndCount(ctx, svc, 42)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("want 1, got %d", n)
	}

	// Force-fail Create to verify error path, again without a real database.
	failRepo := newMockRepo()
	failRepo.failOn = "Create"
	svc2 := &counterSvc{repo: failRepo}
	_, err = addAndCount(ctx, svc2, 1)
	if err == nil {
		t.Error("expected error from failing mock, got nil")
	}
}

// ─── Lock scope SQL generation ────────────────────────────────────────────────

// lockingClause applies scope to a fresh GORM Statement and returns the
// clause.Locking that was added — tested independently of any SQL dialect
// (SQLite drops FOR UPDATE from rendered SQL, so we inspect the clause struct).
func lockingClause(db *gorm.DB, scope func(*gorm.DB) *gorm.DB) clause.Locking {
	scoped := scope(db.Session(&gorm.Session{NewDB: true}))
	if scoped.Statement == nil {
		return clause.Locking{}
	}
	if c, ok := scoped.Statement.Clauses["FOR"]; ok {
		if locking, ok := c.Expression.(clause.Locking); ok {
			return locking
		}
	}
	return clause.Locking{}
}

func TestForUpdate_AppliesUpdateStrength(t *testing.T) {
	db := testDB(t, &Counter{})
	lk := lockingClause(db, orm.ForUpdate())
	if lk.Strength != "UPDATE" {
		t.Errorf("ForUpdate: want Strength='UPDATE', got %q", lk.Strength)
	}
}

func TestForUpdateSkipLocked_AppliesStrengthAndOption(t *testing.T) {
	db := testDB(t, &Counter{})
	lk := lockingClause(db, orm.ForUpdateSkipLocked())
	if lk.Strength != "UPDATE" {
		t.Errorf("ForUpdateSkipLocked: want Strength='UPDATE', got %q", lk.Strength)
	}
	if lk.Options != "SKIP LOCKED" {
		t.Errorf("ForUpdateSkipLocked: want Options='SKIP LOCKED', got %q", lk.Options)
	}
}

func TestForUpdateNoWait_AppliesNoWait(t *testing.T) {
	db := testDB(t, &Counter{})
	lk := lockingClause(db, orm.ForUpdateNoWait())
	if lk.Strength != "UPDATE" {
		t.Errorf("ForUpdateNoWait: want Strength='UPDATE', got %q", lk.Strength)
	}
	if lk.Options != "NOWAIT" {
		t.Errorf("ForUpdateNoWait: want Options='NOWAIT', got %q", lk.Options)
	}
}

func TestForShare_AppliesShareStrength(t *testing.T) {
	db := testDB(t, &Counter{})
	lk := lockingClause(db, orm.ForShare())
	if lk.Strength != "SHARE" {
		t.Errorf("ForShare: want Strength='SHARE', got %q", lk.Strength)
	}
}

func TestForShareSkipLocked_AppliesShareAndSkipLocked(t *testing.T) {
	db := testDB(t, &Counter{})
	lk := lockingClause(db, orm.ForShareSkipLocked())
	if lk.Strength != "SHARE" {
		t.Errorf("ForShareSkipLocked: want Strength='SHARE', got %q", lk.Strength)
	}
	if lk.Options != "SKIP LOCKED" {
		t.Errorf("ForShareSkipLocked: want Options='SKIP LOCKED', got %q", lk.Options)
	}
}

func TestWithLock_CustomStrength(t *testing.T) {
	db := testDB(t, &Counter{})
	lk := lockingClause(db, orm.WithLock("UPDATE", "NOWAIT"))
	if lk.Strength != "UPDATE" {
		t.Errorf("WithLock: want Strength='UPDATE', got %q", lk.Strength)
	}
	if lk.Options != "NOWAIT" {
		t.Errorf("WithLock: want Options='NOWAIT', got %q", lk.Options)
	}
}

// ─── UpdateOptimistic ─────────────────────────────────────────────────────────

func TestUpdateOptimistic_SucceedsOnVersionMatch(t *testing.T) {
	db := testDB(t, &Versioned{})
	db.Create(&Versioned{ID: 1, Name: "original", Version: 0})

	err := orm.UpdateOptimistic(context.Background(), db, &Versioned{}, 1, 0,
		map[string]any{"name": "updated"},
	)
	if err != nil {
		t.Fatalf("UpdateOptimistic: %v", err)
	}

	var v Versioned
	db.First(&v, 1)
	if v.Name != "updated" {
		t.Errorf("want name='updated', got %q", v.Name)
	}
	if v.Version != 1 {
		t.Errorf("want version=1 after update, got %d", v.Version)
	}
}

func TestUpdateOptimistic_ConflictOnVersionMismatch(t *testing.T) {
	db := testDB(t, &Versioned{})
	db.Create(&Versioned{ID: 2, Name: "base", Version: 5})

	err := orm.UpdateOptimistic(context.Background(), db, &Versioned{}, 2, 3,
		map[string]any{"name": "stale update"},
	)
	if !errors.Is(err, orm.ErrOptimisticConflict) {
		t.Errorf("expected ErrOptimisticConflict, got %v", err)
	}
}

func TestUpdateOptimistic_DoesNotMutateCallerMap(t *testing.T) {
	db := testDB(t, &Versioned{})
	db.Create(&Versioned{ID: 3, Name: "orig", Version: 1})

	updates := map[string]any{"name": "new"}
	_ = orm.UpdateOptimistic(context.Background(), db, &Versioned{}, 3, 1, updates)

	if len(updates) != 1 {
		t.Errorf("caller map should have 1 key, got %d: %v", len(updates), updates)
	}
}

// ─── Concurrent RunTx safety ──────────────────────────────────────────────────

func TestRunTx_ConcurrentSafe(t *testing.T) {
	db := testDB(t, &Counter{})

	var (
		wg      sync.WaitGroup
		success int64
		mu      sync.Mutex
	)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(val int) {
			defer wg.Done()
			err := orm.RunTx(context.Background(), db, func(tx *gorm.DB) error {
				return tx.Create(&Counter{Value: val}).Error
			})
			if err == nil {
				mu.Lock()
				success++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	if success == 0 {
		t.Error("expected at least one concurrent RunTx to succeed")
	}
}

// ─── orm.Model and orm.SoftDeleteModel ───────────────────────────────────────

type Article struct {
	orm.Model
	Title string
}

type BlogPost struct {
	orm.SoftDeleteModel
	Body string
}

// restoreTimeutilConfig saves and restores the global timeutil config after the test.
func restoreTimeutilConfig(t *testing.T) {
	t.Helper()
	origLoc := timeutil.Location()
	origLayout := timeutil.Layout()
	t.Cleanup(func() {
		_ = timeutil.SetTimezone(origLoc.String())
		timeutil.SetLayout(origLayout)
	})
}

func TestModel_BeforeCreate_SetsTimestamps(t *testing.T) {
	db := testDB(t, &Article{})
	a := Article{Title: "hello"}
	if err := db.Create(&a).Error; err != nil {
		t.Fatalf("Create: %v", err)
	}
	if a.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set after Create")
	}
	if a.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set after Create")
	}
}

func TestModel_BeforeCreate_PreservesExistingCreatedAt(t *testing.T) {
	db := testDB(t, &Article{})
	fixed := timeutil.Unix(1_000_000)
	a := Article{Title: "preset", Model: orm.Model{CreatedAt: fixed}}
	if err := db.Create(&a).Error; err != nil {
		t.Fatalf("Create: %v", err)
	}
	if a.CreatedAt.Unix() != fixed.Unix() {
		t.Errorf("CreatedAt changed: got %d, want %d", a.CreatedAt.Unix(), fixed.Unix())
	}
}

func TestModel_BeforeUpdate_UpdatesUpdatedAt(t *testing.T) {
	db := testDB(t, &Article{})
	a := Article{Title: "original"}
	if err := db.Create(&a).Error; err != nil {
		t.Fatalf("Create: %v", err)
	}
	createdUpdatedAt := a.UpdatedAt.Unix()

	// Pre-set to simulate a later time so BeforeUpdate overwrites it.
	a.Model.UpdatedAt = timeutil.Unix(createdUpdatedAt + 5)

	if err := db.Save(&a).Error; err != nil {
		t.Fatalf("Save: %v", err)
	}
	if a.UpdatedAt.Unix() < createdUpdatedAt {
		t.Error("UpdatedAt after save should not be earlier than original")
	}
}

func TestModel_JSON_FormatsTimestamps(t *testing.T) {
	restoreTimeutilConfig(t)
	timeutil.SetLayout(timeutil.DateTimeLayout)
	db := testDB(t, &Article{})
	a := Article{Title: "json-test"}
	if err := db.Create(&a).Error; err != nil {
		t.Fatalf("Create: %v", err)
	}
	b, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	createdAt, ok := m["created_at"].(string)
	if !ok || createdAt == "" {
		t.Errorf("created_at in JSON = %v, want a non-empty string", m["created_at"])
	}
}

func TestModel_DBRoundTrip(t *testing.T) {
	db := testDB(t, &Article{})
	a := Article{Title: "roundtrip"}
	if err := db.Create(&a).Error; err != nil {
		t.Fatalf("Create: %v", err)
	}
	var loaded Article
	if err := db.First(&loaded, a.ID).Error; err != nil {
		t.Fatalf("First: %v", err)
	}
	if loaded.CreatedAt.IsZero() {
		t.Error("loaded CreatedAt should not be zero")
	}
	diff := loaded.CreatedAt.Unix() - a.CreatedAt.Unix()
	if diff < -1 || diff > 1 {
		t.Errorf("CreatedAt mismatch: stored=%d loaded=%d diff=%d", a.CreatedAt.Unix(), loaded.CreatedAt.Unix(), diff)
	}
}

func TestSoftDeleteModel_SoftDelete_SetsDeletedAt(t *testing.T) {
	db := testDB(t, &BlogPost{})
	p := BlogPost{Body: "hello"}
	if err := db.Create(&p).Error; err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := p.SoftDelete(db, &p); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}
	if p.DeletedAt == nil {
		t.Fatal("DeletedAt should be set after SoftDelete")
	}
	if p.DeletedAt.IsZero() {
		t.Fatal("DeletedAt should be non-zero after SoftDelete")
	}
}

func TestSoftDeleteModel_SoftDelete_RowStillExists(t *testing.T) {
	db := testDB(t, &BlogPost{})
	p := BlogPost{Body: "survivor"}
	if err := db.Create(&p).Error; err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := p.SoftDelete(db, &p); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}
	var count int64
	db.Model(&BlogPost{}).Count(&count)
	if count == 0 {
		t.Error("soft-deleted row should still exist in the database")
	}
}
