// Package orm — shard.go provides horizontal sharding (分库分表) for GORM.
//
// ShardRouter maps a sharding key to one of N *gorm.DB instances using a
// configurable hash function. The default hash is xxhash (already a dependency
// of the orm module) with modulo sharding.
//
// # Quick start
//
//	db0, _ := orm.Postgres(dsn0)
//	db1, _ := orm.Postgres(dsn1)
//	db2, _ := orm.Postgres(dsn2)
//	db3, _ := orm.Postgres(dsn3)
//
//	router, err := orm.NewShardRouter(orm.ShardConfig{
//	    ShardCount: 4,
//	    DBs:        []*gorm.DB{db0, db1, db2, db3},
//	})
//	if err != nil { log.Fatal(err) }
//
//	// In a handler — route by user ID:
//	db := router.DB(ctx, userID)
//	orderRepo := orm.NewRepository[Order](db)
//
// # Custom shard function
//
//	router, _ := orm.NewShardRouter(orm.ShardConfig{
//	    ShardCount: 4,
//	    DBs:        dbs,
//	    ShardFunc: func(key any) int {
//	        id, _ := key.(int64)
//	        return int(id % 4)
//	    },
//	})
//
// # Cross-shard operations
//
// ShardRouter does NOT provide distributed transactions. Cross-shard writes
// must be handled at the application layer (e.g. Saga pattern). Each shard
// is an independent *gorm.DB and can use RunTx independently.
package orm

import (
	"context"
	"fmt"

	"github.com/cespare/xxhash/v2"
	"gorm.io/gorm"
)

// ShardConfig configures a ShardRouter.
type ShardConfig struct {
	// ShardCount is the total number of shards. Must be > 0 and equal to len(DBs).
	ShardCount int

	// DBs is the list of *gorm.DB instances, one per shard.
	// DBs[i] handles all keys whose ShardFunc(key) == i.
	DBs []*gorm.DB

	// ShardFunc maps a sharding key to a shard index in [0, ShardCount).
	// If nil, the default xxhash-based function is used.
	//
	// The key may be any comparable value. The built-in implementation handles
	// int, int32, int64, uint, uint32, uint64, and string. Other types are
	// converted to string via fmt.Sprintf("%v", key) before hashing.
	ShardFunc func(key any) int
}

// ShardRouter routes operations to the correct shard based on a sharding key.
type ShardRouter struct {
	cfg       ShardConfig
	shardFunc func(key any) int
}

// NewShardRouter validates cfg and returns a ready-to-use ShardRouter.
func NewShardRouter(cfg ShardConfig) (*ShardRouter, error) {
	if cfg.ShardCount <= 0 {
		return nil, fmt.Errorf("orm: ShardConfig.ShardCount must be > 0")
	}
	if len(cfg.DBs) != cfg.ShardCount {
		return nil, fmt.Errorf("orm: len(ShardConfig.DBs) = %d, want %d (ShardCount)",
			len(cfg.DBs), cfg.ShardCount)
	}
	for i, db := range cfg.DBs {
		if db == nil {
			return nil, fmt.Errorf("orm: ShardConfig.DBs[%d] is nil", i)
		}
	}

	fn := cfg.ShardFunc
	if fn == nil {
		n := cfg.ShardCount
		fn = func(key any) int {
			return defaultShardIndex(key, n)
		}
	}

	return &ShardRouter{cfg: cfg, shardFunc: fn}, nil
}

// DB returns the *gorm.DB for the shard that owns key, scoped to ctx.
//
// key may be any value; the built-in hash handles int*, uint*, and string
// natively. Other types are stringified with fmt.Sprintf("%v", key).
func (r *ShardRouter) DB(ctx context.Context, key any) *gorm.DB {
	idx := r.shardFunc(key)
	return r.cfg.DBs[idx].WithContext(ctx)
}

// ShardIndex returns the shard index for key without opening a DB connection.
// Useful for logging, debugging, and pre-routing decisions.
func (r *ShardRouter) ShardIndex(key any) int {
	return r.shardFunc(key)
}

// ShardCount returns the total number of configured shards.
func (r *ShardRouter) ShardCount() int { return r.cfg.ShardCount }

// defaultShardIndex hashes key with xxhash and returns the result modulo n.
func defaultShardIndex(key any, n int) int {
	var h uint64
	switch k := key.(type) {
	case int:
		h = xxhash.Sum64(uint64ToBytes(uint64(k)))
	case int32:
		h = xxhash.Sum64(uint64ToBytes(uint64(k)))
	case int64:
		h = xxhash.Sum64(uint64ToBytes(uint64(k)))
	case uint:
		h = xxhash.Sum64(uint64ToBytes(uint64(k)))
	case uint32:
		h = xxhash.Sum64(uint64ToBytes(uint64(k)))
	case uint64:
		h = xxhash.Sum64(uint64ToBytes(k))
	case string:
		h = xxhash.Sum64String(k)
	default:
		h = xxhash.Sum64String(fmt.Sprintf("%v", k))
	}
	return int(h % uint64(n))
}

// uint64ToBytes converts a uint64 to an 8-byte big-endian slice for hashing.
func uint64ToBytes(v uint64) []byte {
	return []byte{
		byte(v >> 56), byte(v >> 48), byte(v >> 40), byte(v >> 32),
		byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v),
	}
}
