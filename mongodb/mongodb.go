// Package mongodb provides a thin, ergonomic wrapper around the official MongoDB
// Go driver v2. It adds:
//
//   - A [Client] that embeds *mongo.Client with helpers (Ping, Disconnect, DB).
//   - A generic [TypedCollection][T] that pre-types Find / FindOne / InsertOne
//     / InsertMany / UpdateByID / DeleteByID / CountDocuments so callers never
//     need to decode raw bson.Raw themselves.
//   - Opinionated connection pool defaults that are safe for production use.
//
// # Quick start
//
//	c, err := mongodb.Connect(ctx, "mongodb://localhost:27017")
//	defer c.Disconnect(ctx)
//
//	users := mongodb.Collection[User](c, "mydb", "users")
//	_ = users.InsertOne(ctx, User{Name: "alice"})
//
//	var u User
//	_ = users.FindByID(ctx, id, &u)
//
// # Connection options
//
//	c, err := mongodb.Connect(ctx, uri, mongodb.ConnectConfig{
//	    MaxPoolSize: 50,
//	    MinPoolSize: 5,
//	})
package mongodb

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

// ConnectConfig tunes the MongoDB connection pool and timeouts.
type ConnectConfig struct {
	// MaxPoolSize is the maximum number of connections. Default: 100.
	MaxPoolSize uint64
	// MinPoolSize is the minimum number of connections kept alive. Default: 0.
	MinPoolSize uint64
	// ConnectTimeout is the timeout for establishing a new connection. Default: 10s.
	ConnectTimeout time.Duration
	// ServerSelectionTimeout is the timeout for server selection. Default: 30s.
	ServerSelectionTimeout time.Duration
}

func (c *ConnectConfig) setDefaults() {
	if c.MaxPoolSize == 0 {
		c.MaxPoolSize = 100
	}
	if c.ConnectTimeout == 0 {
		c.ConnectTimeout = 10 * time.Second
	}
	if c.ServerSelectionTimeout == 0 {
		c.ServerSelectionTimeout = 30 * time.Second
	}
}

// Client wraps *mongo.Client with convenience helpers.
type Client struct {
	*mongo.Client
	defaultDB string
}

// Connect creates a new MongoDB client and verifies the connection via Ping.
//
// uri example: "mongodb://localhost:27017" or
// "mongodb+srv://user:pass@cluster.mongodb.net/mydb"
func Connect(ctx context.Context, uri string, cfgs ...ConnectConfig) (*Client, error) {
	cfg := ConnectConfig{}
	if len(cfgs) > 0 {
		cfg = cfgs[0]
	}
	cfg.setDefaults()

	opts := options.Client().ApplyURI(uri).
		SetMaxPoolSize(cfg.MaxPoolSize).
		SetMinPoolSize(cfg.MinPoolSize).
		SetConnectTimeout(cfg.ConnectTimeout).
		SetServerSelectionTimeout(cfg.ServerSelectionTimeout)

	client, err := mongo.Connect(opts)
	if err != nil {
		return nil, fmt.Errorf("mongodb: connect: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
	defer cancel()
	if err := client.Ping(pingCtx, readpref.Primary()); err != nil {
		_ = client.Disconnect(ctx)
		return nil, fmt.Errorf("mongodb: ping: %w", err)
	}

	return &Client{Client: client}, nil
}

// WithDefaultDB returns a shallow copy of the client with a default database
// name. When set, DB() and the Collection helper use it when no database is
// specified.
func (c *Client) WithDefaultDB(name string) *Client {
	cp := *c
	cp.defaultDB = name
	return &cp
}

// DB returns a *mongo.Database. If name is empty, defaultDB is used.
func (c *Client) DB(name string) *mongo.Database {
	if name == "" {
		name = c.defaultDB
	}
	return c.Database(name)
}

// Ping verifies the connection is alive using a primary read-preference.
func (c *Client) Ping(ctx context.Context) error {
	return c.Client.Ping(ctx, readpref.Primary())
}

// ─── Typed Collection ─────────────────────────────────────────────────────────

// TypedCollection is a generic wrapper around *mongo.Collection that decodes
// documents into T automatically, avoiding repetitive bson.Raw boilerplate.
//
//	users := mongodb.Collection[User](client, "mydb", "users")
//	user, err := users.FindByID(ctx, id)
type TypedCollection[T any] struct {
	coll *mongo.Collection
}

// Collection returns a TypedCollection[T] pointing to db.coll on client.
// If db is empty, the client's defaultDB is used.
func Collection[T any](c *Client, db, coll string) *TypedCollection[T] {
	return &TypedCollection[T]{coll: c.DB(db).Collection(coll)}
}

// Raw returns the underlying *mongo.Collection for operations not covered by
// the typed helpers (e.g. aggregation pipelines, bulk writes).
func (tc *TypedCollection[T]) Raw() *mongo.Collection { return tc.coll }

// InsertOne inserts a single document and returns its inserted ID.
func (tc *TypedCollection[T]) InsertOne(ctx context.Context, doc T, opts ...options.Lister[options.InsertOneOptions]) (any, error) {
	res, err := tc.coll.InsertOne(ctx, doc, opts...)
	if err != nil {
		return nil, fmt.Errorf("mongodb: InsertOne: %w", err)
	}
	return res.InsertedID, nil
}

// InsertMany inserts multiple documents and returns their inserted IDs.
func (tc *TypedCollection[T]) InsertMany(ctx context.Context, docs []T, opts ...options.Lister[options.InsertManyOptions]) ([]any, error) {
	raw := make([]any, len(docs))
	for i, d := range docs {
		raw[i] = d
	}
	res, err := tc.coll.InsertMany(ctx, raw, opts...)
	if err != nil {
		return nil, fmt.Errorf("mongodb: InsertMany: %w", err)
	}
	return res.InsertedIDs, nil
}

// FindByID retrieves a single document by its _id field.
// Returns mongo.ErrNoDocuments when not found.
func (tc *TypedCollection[T]) FindByID(ctx context.Context, id any, opts ...options.Lister[options.FindOneOptions]) (*T, error) {
	var result T
	err := tc.coll.FindOne(ctx, bson.M{"_id": id}, opts...).Decode(&result)
	if err != nil {
		return nil, fmt.Errorf("mongodb: FindByID: %w", err)
	}
	return &result, nil
}

// FindOne returns the first document matching filter.
// Returns mongo.ErrNoDocuments when not found.
func (tc *TypedCollection[T]) FindOne(ctx context.Context, filter any, opts ...options.Lister[options.FindOneOptions]) (*T, error) {
	var result T
	err := tc.coll.FindOne(ctx, filter, opts...).Decode(&result)
	if err != nil {
		return nil, fmt.Errorf("mongodb: FindOne: %w", err)
	}
	return &result, nil
}

// Find returns all documents matching filter.
func (tc *TypedCollection[T]) Find(ctx context.Context, filter any, opts ...options.Lister[options.FindOptions]) ([]T, error) {
	cur, err := tc.coll.Find(ctx, filter, opts...)
	if err != nil {
		return nil, fmt.Errorf("mongodb: Find: %w", err)
	}
	defer cur.Close(ctx)

	var results []T
	if err := cur.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("mongodb: Find decode: %w", err)
	}
	return results, nil
}

// UpdateByID applies update to the document with the given _id.
// Returns the number of documents modified.
func (tc *TypedCollection[T]) UpdateByID(ctx context.Context, id any, update any, opts ...options.Lister[options.UpdateOneOptions]) (int64, error) {
	res, err := tc.coll.UpdateOne(ctx, bson.M{"_id": id}, update, opts...)
	if err != nil {
		return 0, fmt.Errorf("mongodb: UpdateByID: %w", err)
	}
	return res.ModifiedCount, nil
}

// UpdateOne applies update to the first document matching filter.
func (tc *TypedCollection[T]) UpdateOne(ctx context.Context, filter, update any, opts ...options.Lister[options.UpdateOneOptions]) (int64, error) {
	res, err := tc.coll.UpdateOne(ctx, filter, update, opts...)
	if err != nil {
		return 0, fmt.Errorf("mongodb: UpdateOne: %w", err)
	}
	return res.ModifiedCount, nil
}

// UpdateMany applies update to all documents matching filter.
func (tc *TypedCollection[T]) UpdateMany(ctx context.Context, filter, update any, opts ...options.Lister[options.UpdateManyOptions]) (int64, error) {
	res, err := tc.coll.UpdateMany(ctx, filter, update, opts...)
	if err != nil {
		return 0, fmt.Errorf("mongodb: UpdateMany: %w", err)
	}
	return res.ModifiedCount, nil
}

// DeleteByID removes the document with the given _id.
func (tc *TypedCollection[T]) DeleteByID(ctx context.Context, id any, opts ...options.Lister[options.DeleteOneOptions]) error {
	_, err := tc.coll.DeleteOne(ctx, bson.M{"_id": id}, opts...)
	if err != nil {
		return fmt.Errorf("mongodb: DeleteByID: %w", err)
	}
	return nil
}

// DeleteOne removes the first document matching filter.
func (tc *TypedCollection[T]) DeleteOne(ctx context.Context, filter any, opts ...options.Lister[options.DeleteOneOptions]) (int64, error) {
	res, err := tc.coll.DeleteOne(ctx, filter, opts...)
	if err != nil {
		return 0, fmt.Errorf("mongodb: DeleteOne: %w", err)
	}
	return res.DeletedCount, nil
}

// DeleteMany removes all documents matching filter.
func (tc *TypedCollection[T]) DeleteMany(ctx context.Context, filter any, opts ...options.Lister[options.DeleteManyOptions]) (int64, error) {
	res, err := tc.coll.DeleteMany(ctx, filter, opts...)
	if err != nil {
		return 0, fmt.Errorf("mongodb: DeleteMany: %w", err)
	}
	return res.DeletedCount, nil
}

// CountDocuments returns the number of documents matching filter.
func (tc *TypedCollection[T]) CountDocuments(ctx context.Context, filter any, opts ...options.Lister[options.CountOptions]) (int64, error) {
	n, err := tc.coll.CountDocuments(ctx, filter, opts...)
	if err != nil {
		return 0, fmt.Errorf("mongodb: CountDocuments: %w", err)
	}
	return n, nil
}

// FindOneAndUpdate atomically finds and updates a document, returning the
// updated document (by default, the document after modification).
func (tc *TypedCollection[T]) FindOneAndUpdate(ctx context.Context, filter, update any, opts ...options.Lister[options.FindOneAndUpdateOptions]) (*T, error) {
	var result T
	err := tc.coll.FindOneAndUpdate(ctx, filter, update, opts...).Decode(&result)
	if err != nil {
		return nil, fmt.Errorf("mongodb: FindOneAndUpdate: %w", err)
	}
	return &result, nil
}
