// Package redis provides a Redis-backed implementation of the taskqueue.Broker
// interface using go-redis/v9.
//
// All state transitions are performed via Lua scripts to guarantee atomicity
// without multi-step WATCH/MULTI/EXEC transactions.
//
// # Key layout
//
//	tq:queues                  SET  — known queue names
//	tq:{queue}:pending         LIST — task IDs ready for processing (RPOP to consume)
//	tq:{queue}:active          ZSET — task IDs in flight (score = lease deadline unix)
//	tq:{queue}:scheduled       ZSET — future tasks     (score = process_at unix)
//	tq:{queue}:retry           ZSET — failed, waiting  (score = next_retry unix)
//	tq:{queue}:dead            ZSET — exhausted tasks  (score = failed_at unix)
//	tq:task:{id}               STRING — JSON-encoded Task
//	tq:unique:{key}            STRING — deduplication lock (TTL = UniqueFor)
//
// # Config
//
//	broker, err := tqredis.New(tqredis.Config{Addr: "localhost:6379"})
//	defer broker.Close()
package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/astra-go/astra/taskqueue"
)

// Config configures the Redis broker.
type Config struct {
	// Addr is the Redis server address. e.g. "localhost:6379".
	Addr string

	// Password for AUTH. Empty means no authentication.
	Password string

	// DB is the Redis database index. Default: 0.
	DB int

	// PoolSize is the maximum number of connections. Default: 10.
	PoolSize int

	// MinIdleConns is the minimum number of idle connections kept open.
	MinIdleConns int

	// DialTimeout, ReadTimeout, WriteTimeout override the client defaults.
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration

	// KeyPrefix is prepended to all Redis keys. Default: "tq".
	KeyPrefix string
}

func (c *Config) setDefaults() {
	if c.KeyPrefix == "" {
		c.KeyPrefix = "tq"
	}
	if c.PoolSize == 0 {
		c.PoolSize = 10
	}
}

// Broker is a Redis-backed taskqueue.Broker.
type Broker struct {
	rdb    goredis.UniversalClient
	prefix string

	scriptEnqueue  *goredis.Script
	scriptDequeue  *goredis.Script
	scriptAck      *goredis.Script
	scriptNack     *goredis.Script
	scriptSchedule *goredis.Script
	scriptReap     *goredis.Script
}

// New creates a Broker using a new Redis client built from cfg.
func New(cfg Config) (*Broker, error) {
	cfg.setDefaults()
	rdb := goredis.NewClient(&goredis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	})
	return NewFromClient(rdb, cfg.KeyPrefix), nil
}

// NewFromClient creates a Broker from an existing Redis client.
// prefix is the key namespace (default "tq" if empty).
func NewFromClient(rdb goredis.UniversalClient, prefix string) *Broker {
	if prefix == "" {
		prefix = "tq"
	}
	b := &Broker{rdb: rdb, prefix: prefix}
	b.initScripts()
	return b
}

// ─── key helpers ──────────────────────────────────────────────────────────────

func (b *Broker) keyQueues() string               { return b.prefix + ":queues" }
func (b *Broker) keyTask(id string) string        { return b.prefix + ":task:" + id }
func (b *Broker) keyUnique(key string) string     { return b.prefix + ":unique:" + key }
func (b *Broker) keyPending(q string) string      { return b.prefix + ":" + q + ":pending" }
func (b *Broker) keyActive(q string) string       { return b.prefix + ":" + q + ":active" }
func (b *Broker) keyScheduled(q string) string    { return b.prefix + ":" + q + ":scheduled" }
func (b *Broker) keyRetry(q string) string        { return b.prefix + ":" + q + ":retry" }
func (b *Broker) keyDead(q string) string         { return b.prefix + ":" + q + ":dead" }

// ─── Lua scripts ──────────────────────────────────────────────────────────────

// enqueue.lua
//
// KEYS[1] = tq:queues
// KEYS[2] = tq:task:{id}
// KEYS[3] = tq:{queue}:pending  OR  tq:{queue}:scheduled
// KEYS[4] = tq:unique:{key}      (empty string if no dedup)
//
// ARGV[1] = task JSON
// ARGV[2] = task ID
// ARGV[3] = "pending" or "scheduled"
// ARGV[4] = process_at unix (int64) — used when state==scheduled
// ARGV[5] = queue name
// ARGV[6] = unique TTL seconds (0 = no dedup)
const luaEnqueue = `
local queues_key  = KEYS[1]
local task_key    = KEYS[2]
local dest_key    = KEYS[3]
local unique_key  = KEYS[4]
local task_json   = ARGV[1]
local task_id     = ARGV[2]
local state       = ARGV[3]
local process_at  = tonumber(ARGV[4])
local queue_name  = ARGV[5]
local unique_ttl  = tonumber(ARGV[6])

-- Dedup check
if unique_key ~= "" and unique_ttl > 0 then
    if redis.call("EXISTS", unique_key) == 1 then
        return redis.error_reply("DUPLICATE")
    end
    redis.call("SET", unique_key, task_id, "EX", unique_ttl)
end

-- Store task
redis.call("SET", task_key, task_json)
-- Track known queues
redis.call("SADD", queues_key, queue_name)

-- Enqueue
if state == "pending" then
    redis.call("LPUSH", dest_key, task_id)
else
    redis.call("ZADD", dest_key, process_at, task_id)
end

return "OK"
`

// dequeue.lua
//
// KEYS[1] = tq:{queue}:pending
// KEYS[2] = tq:{queue}:active
// KEYS[3] = prefix  (used to build tq:task:{id})
//
// ARGV[1] = active deadline unix (int64)
// ARGV[2] = prefix string
//
// Returns: {task_id, task_json} or nil
const luaDequeue = `
local pending_key = KEYS[1]
local active_key  = KEYS[2]
local prefix      = ARGV[1]
local deadline    = tonumber(ARGV[2])

local task_id = redis.call("RPOP", pending_key)
if not task_id then
    return nil
end

local task_key  = prefix .. ":task:" .. task_id
local task_json = redis.call("GET", task_key)
if not task_json then
    -- orphan id, skip
    return nil
end

redis.call("ZADD", active_key, deadline, task_id)
return {task_id, task_json}
`

// ack.lua
//
// KEYS[1] = tq:{queue}:active
// KEYS[2] = tq:task:{id}
// KEYS[3] = tq:unique:{key}  (empty string if no dedup key)
//
// ARGV[1] = task_id
// ARGV[2] = unique_key (may be "")
const luaAck = `
local active_key = KEYS[1]
local task_key   = KEYS[2]
local unique_key = KEYS[3]
local task_id    = ARGV[1]

redis.call("ZREM", active_key, task_id)
redis.call("UNLINK", task_key)
if unique_key ~= "" then
    redis.call("UNLINK", unique_key)
end
return "OK"
`

// nack.lua
//
// KEYS[1] = tq:{queue}:active
// KEYS[2] = tq:task:{id}
// KEYS[3] = tq:{queue}:retry  OR  tq:{queue}:dead
//
// ARGV[1] = task_id
// ARGV[2] = updated task JSON
// ARGV[3] = "retry" or "dead"
// ARGV[4] = retry_at unix (int64) — used when state==retry
const luaNack = `
local active_key = KEYS[1]
local task_key   = KEYS[2]
local dest_key   = KEYS[3]
local task_id    = ARGV[1]
local task_json  = ARGV[2]
local dest_state = ARGV[3]
local retry_at   = tonumber(ARGV[4])

redis.call("ZREM", active_key, task_id)
redis.call("SET",  task_key, task_json)

if dest_state == "retry" then
    redis.call("ZADD", dest_key, retry_at, task_id)
else
    redis.call("ZADD", dest_key, retry_at, task_id)
end
return "OK"
`

// schedule.lua — promotes due scheduled/retry tasks to pending.
//
// KEYS[1] = tq:queues
// ARGV[1] = prefix
// ARGV[2] = now unix (int64)
const luaSchedule = `
local queues_key = KEYS[1]
local prefix     = ARGV[1]
local now        = tonumber(ARGV[2])

local queues = redis.call("SMEMBERS", queues_key)
for _, q in ipairs(queues) do
    local pending_key   = prefix .. ":" .. q .. ":pending"
    local scheduled_key = prefix .. ":" .. q .. ":scheduled"
    local retry_key     = prefix .. ":" .. q .. ":retry"

    for _, src in ipairs({scheduled_key, retry_key}) do
        local ids = redis.call("ZRANGEBYSCORE", src, "-inf", now, "LIMIT", 0, 500)
        for _, id in ipairs(ids) do
            redis.call("ZREM", src, id)
            redis.call("LPUSH", pending_key, id)
        end
    end
end
return "OK"
`

// reap.lua — recovers stale active tasks (lease expired).
//
// KEYS[1] = tq:queues
// ARGV[1] = prefix
// ARGV[2] = now unix (int64)
const luaReap = `
local queues_key = KEYS[1]
local prefix     = ARGV[1]
local now        = tonumber(ARGV[2])

local queues = redis.call("SMEMBERS", queues_key)
for _, q in ipairs(queues) do
    local pending_key = prefix .. ":" .. q .. ":pending"
    local active_key  = prefix .. ":" .. q .. ":active"

    local ids = redis.call("ZRANGEBYSCORE", active_key, "-inf", now, "LIMIT", 0, 500)
    for _, id in ipairs(ids) do
        redis.call("ZREM", active_key, id)
        redis.call("LPUSH", pending_key, id)
    end
end
return "OK"
`

func (b *Broker) initScripts() {
	b.scriptEnqueue = goredis.NewScript(luaEnqueue)
	b.scriptDequeue = goredis.NewScript(luaDequeue)
	b.scriptAck = goredis.NewScript(luaAck)
	b.scriptNack = goredis.NewScript(luaNack)
	b.scriptSchedule = goredis.NewScript(luaSchedule)
	b.scriptReap = goredis.NewScript(luaReap)
}

// ─── Broker interface implementation ─────────────────────────────────────────

// Enqueue atomically stores and enqueues the task.
// Returns taskqueue.ErrDuplicateTask on unique key collision.
func (b *Broker) Enqueue(ctx context.Context, task *taskqueue.Task) error {
	now := time.Now()
	task.UpdatedAt = now

	// Determine state and destination key.
	state := string(taskqueue.StatePending)
	if task.ProcessAt.After(now) {
		state = string(taskqueue.StateScheduled)
		task.State = taskqueue.StateScheduled
	} else {
		task.State = taskqueue.StatePending
	}

	data, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("taskqueue redis: marshal task: %w", err)
	}

	var destKey string
	var processAtUnix int64
	if state == string(taskqueue.StateScheduled) {
		destKey = b.keyScheduled(task.Queue)
		processAtUnix = task.ProcessAt.Unix()
	} else {
		destKey = b.keyPending(task.Queue)
		processAtUnix = 0
	}

	uniqueKey := ""
	if task.UniqueKey != "" {
		uniqueKey = b.keyUnique(task.UniqueKey)
	}
	uniqueTTL := int64(0)
	if task.UniqueFor > 0 {
		uniqueTTL = int64(task.UniqueFor.Seconds())
	}

	keys := []string{b.keyQueues(), b.keyTask(task.ID), destKey, uniqueKey}
	args := []any{
		string(data),
		task.ID,
		state,
		processAtUnix,
		task.Queue,
		uniqueTTL,
	}

	err = b.scriptEnqueue.Run(ctx, b.rdb, keys, args...).Err()
	if err != nil {
		if err.Error() == "DUPLICATE" {
			return taskqueue.ErrDuplicateTask
		}
		return fmt.Errorf("taskqueue redis: enqueue: %w", err)
	}
	return nil
}

// Dequeue atomically pops the highest-priority pending task from one of the
// given queues and marks it active.
func (b *Broker) Dequeue(ctx context.Context, queues []string, deadline time.Time) (*taskqueue.Task, error) {
	for _, q := range queues {
		keys := []string{b.keyPending(q), b.keyActive(q)}
		args := []any{b.prefix, deadline.Unix()}

		res, err := b.scriptDequeue.Run(ctx, b.rdb, keys, args...).StringSlice()
		if err != nil {
			if err == goredis.Nil {
				continue
			}
			return nil, fmt.Errorf("taskqueue redis: dequeue %q: %w", q, err)
		}
		if len(res) < 2 {
			continue
		}

		var task taskqueue.Task
		if err := json.Unmarshal([]byte(res[1]), &task); err != nil {
			return nil, fmt.Errorf("taskqueue redis: unmarshal task %q: %w", res[0], err)
		}
		if err := task.Validate(); err != nil {
			return nil, fmt.Errorf("taskqueue redis: invalid task %q: %w", res[0], err)
		}
		task.State = taskqueue.StateActive
		return &task, nil
	}
	return nil, taskqueue.ErrNoTask
}

// Ack marks the task as done.
func (b *Broker) Ack(ctx context.Context, task *taskqueue.Task) error {
	task.State = taskqueue.StateDone
	task.UpdatedAt = time.Now()

	uniqueKeyArg := ""
	uniqueKey := ""
	if task.UniqueKey != "" {
		uniqueKey = b.keyUnique(task.UniqueKey)
		uniqueKeyArg = task.UniqueKey
	}

	keys := []string{b.keyActive(task.Queue), b.keyTask(task.ID), uniqueKey}
	args := []any{task.ID, uniqueKeyArg}
	if err := b.scriptAck.Run(ctx, b.rdb, keys, args...).Err(); err != nil {
		return fmt.Errorf("taskqueue redis: ack %q: %w", task.ID, err)
	}
	return nil
}

// Nack records failure. If retryAt is zero the task is dead-lettered;
// otherwise it is placed in the retry set.
func (b *Broker) Nack(ctx context.Context, task *taskqueue.Task, lastErr string, retryAt time.Time) error {
	task.LastError = lastErr
	task.UpdatedAt = time.Now()

	var destKey string
	var destState string
	var scoreUnix int64

	if retryAt.IsZero() {
		task.State = taskqueue.StateDead
		destKey = b.keyDead(task.Queue)
		destState = "dead"
		scoreUnix = time.Now().Unix()
	} else {
		task.State = taskqueue.StateRetry
		destKey = b.keyRetry(task.Queue)
		destState = "retry"
		scoreUnix = retryAt.Unix()
	}

	data, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("taskqueue redis: marshal task for nack: %w", err)
	}

	keys := []string{b.keyActive(task.Queue), b.keyTask(task.ID), destKey}
	args := []any{task.ID, string(data), destState, scoreUnix}
	if err := b.scriptNack.Run(ctx, b.rdb, keys, args...).Err(); err != nil {
		return fmt.Errorf("taskqueue redis: nack %q: %w", task.ID, err)
	}
	return nil
}

// Schedule promotes scheduled and retry tasks whose ProcessAt has elapsed
// back to pending.
func (b *Broker) Schedule(ctx context.Context) error {
	keys := []string{b.keyQueues()}
	args := []any{b.prefix, time.Now().Unix()}
	if err := b.scriptSchedule.Run(ctx, b.rdb, keys, args...).Err(); err != nil {
		return fmt.Errorf("taskqueue redis: schedule: %w", err)
	}
	return nil
}

// ReapStale recovers active tasks whose lease deadline has passed.
func (b *Broker) ReapStale(ctx context.Context) error {
	keys := []string{b.keyQueues()}
	args := []any{b.prefix, time.Now().Unix()}
	if err := b.scriptReap.Run(ctx, b.rdb, keys, args...).Err(); err != nil {
		return fmt.Errorf("taskqueue redis: reap: %w", err)
	}
	return nil
}

// Close closes the underlying Redis client.
func (b *Broker) Close() error {
	return b.rdb.Close()
}
