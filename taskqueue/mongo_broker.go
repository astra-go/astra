//go:build mongo
// +build mongo

package taskqueue

// This file provides the MongoDB broker, enabled with build tag "mongo".

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// MongoConfig configures the MongoDB broker.
type MongoConfig struct {
	// URI is the MongoDB connection string. e.g. "mongodb://localhost:27017".
	URI string

	// Database is the target database name.
	Database string

	// MessagesCollection is the name of the tasks collection.
	// Default: "taskqueue_messages".
	MessagesCollection string

	// DedupCollection is the name of the deduplication collection.
	// Default: "taskqueue_dedup".
	DedupCollection string
}

func (c *MongoConfig) setMongoDefaults() {
	if c.MessagesCollection == "" {
		c.MessagesCollection = "taskqueue_messages"
	}
	if c.DedupCollection == "" {
		c.DedupCollection = "taskqueue_dedup"
	}
}

// ─── document shapes ──────────────────────────────────────────────────────────

// mongoTaskDoc is the MongoDB representation of a Task.
type mongoTaskDoc struct {
	TaskID     string    `bson:"task_id"`
	Type       string    `bson:"type"`
	Payload    []byte    `bson:"payload"`
	Queue      string    `bson:"queue"`
	State      string    `bson:"state"`
	MaxRetries int       `bson:"max_retries"`
	Retried    int       `bson:"retried"`
	TimeoutSec int64     `bson:"timeout_sec"`
	ProcessAt  time.Time `bson:"process_at"`
	UniqueKey  string    `bson:"unique_key,omitempty"`
	UniqueFor  int64     `bson:"unique_for_sec,omitempty"`
	LastError  string    `bson:"last_error,omitempty"`
	ActiveBy   time.Time `bson:"active_by,omitempty"`
	CreatedAt  time.Time `bson:"created_at"`
	UpdatedAt  time.Time `bson:"updated_at"`
}

// mongoDedupDoc is the deduplication lock document.
type mongoDedupDoc struct {
	ID        string    `bson:"_id"`
	TaskID    string    `bson:"task_id"`
	ExpiresAt time.Time `bson:"expires_at"`
}

func mongoTaskToDoc(t *Task) *mongoTaskDoc {
	return &mongoTaskDoc{
		TaskID:     t.ID,
		Type:       t.Type,
		Payload:    t.Payload,
		Queue:      t.Queue,
		State:      string(t.State),
		MaxRetries: t.MaxRetries,
		Retried:    t.Retried,
		TimeoutSec: int64(t.Timeout.Seconds()),
		ProcessAt:  t.ProcessAt,
		UniqueKey:  t.UniqueKey,
		UniqueFor:  int64(t.UniqueFor.Seconds()),
		LastError:  t.LastError,
		CreatedAt:  t.CreatedAt,
		UpdatedAt:  t.UpdatedAt,
	}
}

func mongoDocToTask(d *mongoTaskDoc) *Task {
	return &Task{
		ID:         d.TaskID,
		Type:       d.Type,
		Payload:    d.Payload,
		Queue:      d.Queue,
		State:      State(d.State),
		MaxRetries: d.MaxRetries,
		Retried:    d.Retried,
		Timeout:    time.Duration(d.TimeoutSec) * time.Second,
		ProcessAt:  d.ProcessAt,
		UniqueKey:  d.UniqueKey,
		UniqueFor:  time.Duration(d.UniqueFor) * time.Second,
		LastError:  d.LastError,
		CreatedAt:  d.CreatedAt,
		UpdatedAt:  d.UpdatedAt,
	}
}

// ─── Broker ───────────────────────────────────────────────────────────────────

// MongoBroker is a MongoDB-backed Broker.
type MongoBroker struct {
	client   *mongo.Client
	messages *mongo.Collection
	dedup    *mongo.Collection
}

// NewMongoBroker connects to MongoDB, creates collections, and ensures indexes.
func NewMongoBroker(ctx context.Context, cfg MongoConfig) (*MongoBroker, error) {
	cfg.setMongoDefaults()

	client, err := mongo.Connect(options.Client().ApplyURI(cfg.URI))
	if err != nil {
		return nil, fmt.Errorf("taskqueue mongo: connect: %w", err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(ctx)
		return nil, fmt.Errorf("taskqueue mongo: ping: %w", err)
	}
	return newMongoBrokerFromClient(ctx, client, cfg)
}

// NewMongoBrokerFromClient creates a Broker from an existing *mongo.Client.
func NewMongoBrokerFromClient(ctx context.Context, client *mongo.Client, cfg MongoConfig) (*MongoBroker, error) {
	cfg.setMongoDefaults()
	return newMongoBrokerFromClient(ctx, client, cfg)
}

func newMongoBrokerFromClient(ctx context.Context, client *mongo.Client, cfg MongoConfig) (*MongoBroker, error) {
	db := client.Database(cfg.Database)
	b := &MongoBroker{
		client:   client,
		messages: db.Collection(cfg.MessagesCollection),
		dedup:    db.Collection(cfg.DedupCollection),
	}
	if err := b.mongoEnsureIndexes(ctx); err != nil {
		return nil, err
	}
	return b, nil
}

func (b *MongoBroker) mongoEnsureIndexes(ctx context.Context) error {
	// ── messages indexes ──────────────────────────────────────────────────────
	msgIndexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "task_id", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{
				{Key: "queue", Value: 1},
				{Key: "state", Value: 1},
				{Key: "process_at", Value: 1},
			},
		},
		{
			Keys: bson.D{
				{Key: "state", Value: 1},
				{Key: "active_by", Value: 1},
			},
		},
	}
	if _, err := b.messages.Indexes().CreateMany(ctx, msgIndexes); err != nil {
		return fmt.Errorf("taskqueue mongo: create message indexes: %w", err)
	}

	// ── dedup TTL index ───────────────────────────────────────────────────────
	dedupIdx := mongo.IndexModel{
		Keys:    bson.D{{Key: "expires_at", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(0),
	}
	if _, err := b.dedup.Indexes().CreateOne(ctx, dedupIdx); err != nil {
		return fmt.Errorf("taskqueue mongo: create dedup TTL index: %w", err)
	}
	return nil
}

// ─── Broker interface ─────────────────────────────────────────────────────────

// Enqueue stores the task. Returns ErrDuplicateTask on unique key collision.
func (b *MongoBroker) Enqueue(ctx context.Context, task *Task) error {
	now := time.Now()
	task.UpdatedAt = now

	if task.ProcessAt.After(now) {
		task.State = StateScheduled
	} else {
		task.State = StatePending
	}

	// Deduplication: try to insert a dedup lock first.
	if task.UniqueKey != "" && task.UniqueFor > 0 {
		lock := &mongoDedupDoc{
			ID:        task.UniqueKey,
			TaskID:    task.ID,
			ExpiresAt: now.Add(task.UniqueFor),
		}
		_, err := b.dedup.InsertOne(ctx, lock)
		if err != nil {
			if isMongoDuplicateKey(err) {
				return ErrDuplicateTask
			}
			return fmt.Errorf("taskqueue mongo: insert dedup lock: %w", err)
		}
	}

	doc := mongoTaskToDoc(task)
	if _, err := b.messages.InsertOne(ctx, doc); err != nil {
		return fmt.Errorf("taskqueue mongo: insert task: %w", err)
	}
	return nil
}

// Dequeue atomically moves the next pending task to active state.
func (b *MongoBroker) Dequeue(ctx context.Context, queues []string, deadline time.Time) (*Task, error) {
	now := time.Now()
	for _, q := range queues {
		filter := bson.D{
			{Key: "queue", Value: q},
			{Key: "state", Value: string(StatePending)},
			{Key: "process_at", Value: bson.D{{Key: "$lte", Value: now}}},
		}
		update := bson.D{
			{Key: "$set", Value: bson.D{
				{Key: "state", Value: string(StateActive)},
				{Key: "active_by", Value: deadline},
				{Key: "updated_at", Value: now},
			}},
		}
		opts := options.FindOneAndUpdate().
			SetSort(bson.D{{Key: "process_at", Value: 1}}).
			SetReturnDocument(options.After)

		var doc mongoTaskDoc
		err := b.messages.FindOneAndUpdate(ctx, filter, update, opts).Decode(&doc)
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				continue
			}
			return nil, fmt.Errorf("taskqueue mongo: dequeue %q: %w", q, err)
		}
		t := mongoDocToTask(&doc)
		t.State = StateActive
		return t, nil
	}
	return nil, ErrNoTask
}

// Ack marks the task as successfully done.
func (b *MongoBroker) Ack(ctx context.Context, task *Task) error {
	now := time.Now()
	filter := bson.D{{Key: "task_id", Value: task.ID}}
	update := bson.D{
		{Key: "$set", Value: bson.D{
			{Key: "state", Value: string(StateDone)},
			{Key: "updated_at", Value: now},
		}},
	}
	res, err := b.messages.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("taskqueue mongo: ack %q: %w", task.ID, err)
	}
	if res.MatchedCount == 0 {
		return ErrTaskNotFound
	}

	// Remove dedup lock if present.
	if task.UniqueKey != "" {
		_, _ = b.dedup.DeleteOne(ctx, bson.D{{Key: "_id", Value: task.UniqueKey}})
	}
	return nil
}

// Nack records failure. If retryAt is zero the task is dead-lettered.
func (b *MongoBroker) Nack(ctx context.Context, task *Task, lastErr string, retryAt time.Time) error {
	now := time.Now()

	newState := StateDead
	var processAt time.Time
	if !retryAt.IsZero() {
		newState = StateRetry
		processAt = retryAt
	}

	filter := bson.D{{Key: "task_id", Value: task.ID}}
	setFields := bson.D{
		{Key: "state", Value: string(newState)},
		{Key: "last_error", Value: lastErr},
		{Key: "retried", Value: task.Retried},
		{Key: "updated_at", Value: now},
	}
	if !processAt.IsZero() {
		setFields = append(setFields, bson.E{Key: "process_at", Value: processAt})
	}
	update := bson.D{{Key: "$set", Value: setFields}}

	res, err := b.messages.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("taskqueue mongo: nack %q: %w", task.ID, err)
	}
	if res.MatchedCount == 0 {
		return ErrTaskNotFound
	}
	return nil
}

// Schedule promotes scheduled and retry tasks whose process_at has elapsed.
func (b *MongoBroker) Schedule(ctx context.Context) error {
	now := time.Now()
	filter := bson.D{
		{Key: "state", Value: bson.D{{Key: "$in", Value: bson.A{
			string(StateScheduled),
			string(StateRetry),
		}}}},
		{Key: "process_at", Value: bson.D{{Key: "$lte", Value: now}}},
	}
	update := bson.D{
		{Key: "$set", Value: bson.D{
			{Key: "state", Value: string(StatePending)},
			{Key: "updated_at", Value: now},
		}},
	}
	if _, err := b.messages.UpdateMany(ctx, filter, update); err != nil {
		return fmt.Errorf("taskqueue mongo: schedule: %w", err)
	}
	return nil
}

// ReapStale recovers active tasks whose active_by lease has passed.
func (b *MongoBroker) ReapStale(ctx context.Context) error {
	now := time.Now()
	filter := bson.D{
		{Key: "state", Value: string(StateActive)},
		{Key: "active_by", Value: bson.D{{Key: "$lt", Value: now}}},
	}
	update := bson.D{
		{Key: "$set", Value: bson.D{
			{Key: "state", Value: string(StatePending)},
			{Key: "updated_at", Value: now},
		}},
		{Key: "$unset", Value: bson.D{{Key: "active_by", Value: ""}}},
	}
	if _, err := b.messages.UpdateMany(ctx, filter, update); err != nil {
		return fmt.Errorf("taskqueue mongo: reap stale: %w", err)
	}
	return nil
}

// Close disconnects the MongoDB client.
func (b *MongoBroker) Close() error {
	return b.client.Disconnect(context.Background())
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func isMongoDuplicateKey(err error) bool {
	var we mongo.WriteException
	if errors.As(err, &we) {
		for _, e := range we.WriteErrors {
			if e.Code == 11000 {
				return true
			}
		}
	}
	return false
}

// mongoMarshalPayload is a convenience shim used in tests.
func mongoMarshalPayload(v any) ([]byte, error) {
	return json.Marshal(v)
}

// Verify MongoBroker implements Broker at compile time.
var _ Broker = (*MongoBroker)(nil)
