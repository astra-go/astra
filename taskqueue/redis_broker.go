package taskqueue

// This file provides the Redis broker, enabled with build tag "redis".

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// RedisConfig configures the Redis broker.
type RedisConfig struct {
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

func (c *RedisConfig) setRedisDefaults() {
	if c.KeyPrefix == "" {
		c.KeyPrefix = "tq"
	}
	if c.PoolSize == 0 {
		c.PoolSize = 10
	}
}

// RedisBroker is a Redis-backed Broker.
type RedisBroker struct {
	rdb    goredis.UniversalClient
	prefix string

	scriptEnqueue  *goredis.Script
	scriptDequeue  *goredis.Script
	scriptAck      *goredis.Script
	scriptNack     *goredis.Script
	scriptSchedule *goredis.Script
	scriptReap     *goredis.Script
}

// NewRedisBroker creates a Broker using a new Redis client built from cfg.
func NewRedisBroker(cfg RedisConfig) (*RedisBroker, error) {
	cfg.setRedisDefaults()
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
	return NewRedisBrokerFromClient(rdb, cfg.KeyPrefix), nil
}

// NewRedisBrokerFromClient creates a Broker from an existing Redis client.
// prefix is the key namespace (default "tq" if empty).
func NewRedisBrokerFromClient(rdb goredis.UniversalClient, prefix string) *RedisBroker {
	if prefix == "" {
		prefix = "tq"
	}
	b := &RedisBroker{rdb: rdb, prefix: prefix}
	b.initRedisScripts()
	return b
}

// ─── key helpers ──────────────────────────────────────────────────────────────

func (b *RedisBroker) redisKeyQueues() string               { return b.prefix + ":queues" }
func (b *RedisBroker) redisKeyTask(id string) string        { return b.prefix + ":task:" + id }
func (b *RedisBroker) redisKeyUnique(key string) string     { return b.prefix + ":unique:" + key }
func (b *RedisBroker) redisKeyPending(q string) string      { return b.prefix + ":" + q + ":pending" }
func (b *RedisBroker) redisKeyActive(q string) string       { return b.prefix + ":" + q + ":active" }
func (b *RedisBroker) redisKeyScheduled(q string) string    { return b.prefix + ":" + q + ":scheduled" }
func (b *RedisBroker) redisKeyRetry(q string) string        { return b.prefix + ":" + q + ":retry" }
func (b *RedisBroker) redisKeyDead(q string) string         { return b.prefix + ":" + q + ":dead" }

// ─── Lua scripts ──────────────────────────────────────────────────────────────

const redisLuaEnqueue = `
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

if unique_key ~= "" and unique_ttl > 0 then
    if redis.call("EXISTS", unique_key) == 1 then
        return redis.error_reply("DUPLICATE")
    end
    redis.call("SET", unique_key, task_id, "EX", unique_ttl)
end

redis.call("SET", task_key, task_json)
redis.call("SADD", queues_key, queue_name)

if state == "pending" then
    redis.call("LPUSH", dest_key, task_id)
else
    redis.call("ZADD", dest_key, process_at, task_id)
end

return "OK"
`

const redisLuaDequeue = `
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
    return nil
end

redis.call("ZADD", active_key, deadline, task_id)
return {task_id, task_json}
`

const redisLuaAck = `
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

const redisLuaNack = `
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

const redisLuaSchedule = `
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

const redisLuaReap = `
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

func (b *RedisBroker) initRedisScripts() {
	b.scriptEnqueue = goredis.NewScript(redisLuaEnqueue)
	b.scriptDequeue = goredis.NewScript(redisLuaDequeue)
	b.scriptAck = goredis.NewScript(redisLuaAck)
	b.scriptNack = goredis.NewScript(redisLuaNack)
	b.scriptSchedule = goredis.NewScript(redisLuaSchedule)
	b.scriptReap = goredis.NewScript(redisLuaReap)
}

// ─── Broker interface implementation ─────────────────────────────────────────

// Enqueue atomically stores and enqueues the task.
func (b *RedisBroker) Enqueue(ctx context.Context, task *Task) error {
	now := time.Now()
	task.UpdatedAt = now

	state := string(StatePending)
	if task.ProcessAt.After(now) {
		state = string(StateScheduled)
		task.State = StateScheduled
	} else {
		task.State = StatePending
	}

	data, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("taskqueue redis: marshal task: %w", err)
	}

	var destKey string
	var processAtUnix int64
	if state == string(StateScheduled) {
		destKey = b.redisKeyScheduled(task.Queue)
		processAtUnix = task.ProcessAt.Unix()
	} else {
		destKey = b.redisKeyPending(task.Queue)
		processAtUnix = 0
	}

	uniqueKey := ""
	if task.UniqueKey != "" {
		uniqueKey = b.redisKeyUnique(task.UniqueKey)
	}
	uniqueTTL := int64(0)
	if task.UniqueFor > 0 {
		uniqueTTL = int64(task.UniqueFor.Seconds())
	}

	keys := []string{b.redisKeyQueues(), b.redisKeyTask(task.ID), destKey, uniqueKey}
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
			return ErrDuplicateTask
		}
		return fmt.Errorf("taskqueue redis: enqueue: %w", err)
	}
	return nil
}

// Dequeue atomically pops the highest-priority pending task from one of the queues.
func (b *RedisBroker) Dequeue(ctx context.Context, queues []string, deadline time.Time) (*Task, error) {
	for _, q := range queues {
		keys := []string{b.redisKeyPending(q), b.redisKeyActive(q)}
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

		var task Task
		if err := json.Unmarshal([]byte(res[1]), &task); err != nil {
			return nil, fmt.Errorf("taskqueue redis: unmarshal task %q: %w", res[0], err)
		}
		if err := task.Validate(); err != nil {
			return nil, fmt.Errorf("taskqueue redis: invalid task %q: %w", res[0], err)
		}
		task.State = StateActive
		return &task, nil
	}
	return nil, ErrNoTask
}

// Ack marks the task as done.
func (b *RedisBroker) Ack(ctx context.Context, task *Task) error {
	task.State = StateDone
	task.UpdatedAt = time.Now()

	uniqueKeyArg := ""
	uniqueKey := ""
	if task.UniqueKey != "" {
		uniqueKey = b.redisKeyUnique(task.UniqueKey)
		uniqueKeyArg = task.UniqueKey
	}

	keys := []string{b.redisKeyActive(task.Queue), b.redisKeyTask(task.ID), uniqueKey}
	args := []any{task.ID, uniqueKeyArg}
	if err := b.scriptAck.Run(ctx, b.rdb, keys, args...).Err(); err != nil {
		return fmt.Errorf("taskqueue redis: ack %q: %w", task.ID, err)
	}
	return nil
}

// Nack records failure. If retryAt is zero the task is dead-lettered.
func (b *RedisBroker) Nack(ctx context.Context, task *Task, lastErr string, retryAt time.Time) error {
	task.LastError = lastErr
	task.UpdatedAt = time.Now()

	var destKey string
	var destState string
	var scoreUnix int64

	if retryAt.IsZero() {
		task.State = StateDead
		destKey = b.redisKeyDead(task.Queue)
		destState = "dead"
		scoreUnix = time.Now().Unix()
	} else {
		task.State = StateRetry
		destKey = b.redisKeyRetry(task.Queue)
		destState = "retry"
		scoreUnix = retryAt.Unix()
	}

	data, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("taskqueue redis: marshal task for nack: %w", err)
	}

	keys := []string{b.redisKeyActive(task.Queue), b.redisKeyTask(task.ID), destKey}
	args := []any{task.ID, string(data), destState, scoreUnix}
	if err := b.scriptNack.Run(ctx, b.rdb, keys, args...).Err(); err != nil {
		return fmt.Errorf("taskqueue redis: nack %q: %w", task.ID, err)
	}
	return nil
}

// Schedule promotes scheduled and retry tasks whose ProcessAt has elapsed.
func (b *RedisBroker) Schedule(ctx context.Context) error {
	keys := []string{b.redisKeyQueues()}
	args := []any{b.prefix, time.Now().Unix()}
	if err := b.scriptSchedule.Run(ctx, b.rdb, keys, args...).Err(); err != nil {
		return fmt.Errorf("taskqueue redis: schedule: %w", err)
	}
	return nil
}

// ReapStale recovers active tasks whose lease deadline has passed.
func (b *RedisBroker) ReapStale(ctx context.Context) error {
	keys := []string{b.redisKeyQueues()}
	args := []any{b.prefix, time.Now().Unix()}
	if err := b.scriptReap.Run(ctx, b.rdb, keys, args...).Err(); err != nil {
		return fmt.Errorf("taskqueue redis: reap: %w", err)
	}
	return nil
}

// Close closes the underlying Redis client.
func (b *RedisBroker) Close() error {
	return b.rdb.Close()
}

// Verify RedisBroker implements Broker at compile time.
var _ Broker = (*RedisBroker)(nil)
