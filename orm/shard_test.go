package orm_test

import (
	"context"
	"testing"

	"gorm.io/gorm"

	"github.com/astra-go/astra/orm"
)

// ─── ShardRouter ─────────────────────────────────────────────────────────────

func TestNewShardRouter_InvalidConfig(t *testing.T) {
	cases := []struct {
		name string
		cfg  orm.ShardConfig
	}{
		{
			name: "zero ShardCount",
			cfg:  orm.ShardConfig{ShardCount: 0, DBs: nil},
		},
		{
			name: "DBs length mismatch",
			cfg: orm.ShardConfig{
				ShardCount: 2,
				DBs:        []*gorm.DB{openSQLite(t)}, // only 1, want 2
			},
		},
		{
			name: "nil DB in list",
			cfg: orm.ShardConfig{
				ShardCount: 2,
				DBs:        []*gorm.DB{openSQLite(t), nil},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := orm.NewShardRouter(tc.cfg)
			if err == nil {
				t.Errorf("expected error for %s, got nil", tc.name)
			}
		})
	}
}

func TestShardRouter_SameKeyAlwaysSameShard(t *testing.T) {
	dbs := make([]*gorm.DB, 4)
	for i := range dbs {
		dbs[i] = openSQLite(t)
	}
	router, err := orm.NewShardRouter(orm.ShardConfig{ShardCount: 4, DBs: dbs})
	if err != nil {
		t.Fatalf("NewShardRouter: %v", err)
	}

	ctx := context.Background()
	keys := []any{int64(1), int64(42), "user-abc", uint64(999)}
	for _, key := range keys {
		firstIdx := router.ShardIndex(key)
		firstSQL, _ := router.DB(ctx, key).DB()
		for i := 0; i < 5; i++ {
			gotIdx := router.ShardIndex(key)
			gotSQL, _ := router.DB(ctx, key).DB()
			if firstIdx != gotIdx || firstSQL != gotSQL {
				t.Errorf("key %v: shard changed between calls (first=%d, got=%d)", key, firstIdx, gotIdx)
			}
		}
	}
}

func TestShardRouter_ShardIndexInRange(t *testing.T) {
	n := 8
	dbs := make([]*gorm.DB, n)
	for i := range dbs {
		dbs[i] = openSQLite(t)
	}
	router, err := orm.NewShardRouter(orm.ShardConfig{ShardCount: n, DBs: dbs})
	if err != nil {
		t.Fatalf("NewShardRouter: %v", err)
	}

	keys := []any{0, -1, int64(1 << 40), "hello", uint32(12345), ""}
	for _, key := range keys {
		idx := router.ShardIndex(key)
		if idx < 0 || idx >= n {
			t.Errorf("ShardIndex(%v) = %d, want [0, %d)", key, idx, n)
		}
	}
}

func TestShardRouter_DistributionIsReasonable(t *testing.T) {
	n := 4
	dbs := make([]*gorm.DB, n)
	for i := range dbs {
		dbs[i] = openSQLite(t)
	}
	router, err := orm.NewShardRouter(orm.ShardConfig{ShardCount: n, DBs: dbs})
	if err != nil {
		t.Fatalf("NewShardRouter: %v", err)
	}

	counts := make([]int, n)
	total := 1000
	for i := 0; i < total; i++ {
		idx := router.ShardIndex(int64(i))
		counts[idx]++
	}

	// Each shard should receive between 15% and 35% of keys (expected ~25%).
	low := total * 15 / 100
	high := total * 35 / 100
	for i, c := range counts {
		if c < low || c > high {
			t.Errorf("shard %d received %d/%d keys (want %d–%d)", i, c, total, low, high)
		}
	}
}

func TestShardRouter_CustomShardFunc(t *testing.T) {
	dbs := make([]*gorm.DB, 3)
	for i := range dbs {
		dbs[i] = openSQLite(t)
	}
	// Custom func: always route to shard 2.
	router, err := orm.NewShardRouter(orm.ShardConfig{
		ShardCount: 3,
		DBs:        dbs,
		ShardFunc:  func(_ any) int { return 2 },
	})
	if err != nil {
		t.Fatalf("NewShardRouter: %v", err)
	}

	ctx := context.Background()
	for _, key := range []any{1, "x", int64(99)} {
		db := router.DB(ctx, key)
		want, _ := dbs[2].DB()
		got, _ := db.DB()
		if want != got {
			t.Errorf("custom ShardFunc: key %v should route to shard 2", key)
		}
	}
}

func TestShardRouter_ShardCount(t *testing.T) {
	dbs := []*gorm.DB{openSQLite(t), openSQLite(t)}
	router, _ := orm.NewShardRouter(orm.ShardConfig{ShardCount: 2, DBs: dbs})
	if router.ShardCount() != 2 {
		t.Errorf("ShardCount() = %d, want 2", router.ShardCount())
	}
}
