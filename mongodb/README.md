# MongoDB — MongoDB Database Integration

Lightweight wrapper around official MongoDB Go Driver v2, providing generic collection interface and safe connection pool configuration.

## Features

- **Generic Collections**: `TypedCollection[T]` predefines CRUD methods, no manual type assertions
- **Connection Pool**: Safe production defaults (MaxPoolSize=100), supports min connections keep-alive
- **BSON v2**: Uses `go.mongodb.org/mongo-driver/v2`
- **Simplified API**: Shortcut methods like `Ping`, `Disconnect`, `DB`

## Quick Start

```go
import "github.com/astra-go/astra/mongodb"

ctx := context.Background()

// Connect
client, err := mongodb.Connect(ctx, "mongodb://localhost:27017")
defer client.Disconnect(ctx)

// Get generic collection
users := mongodb.Collection[User](client, "mydb", "users")

// Insert
_ = users.InsertOne(ctx, User{Name: "alice", Age: 18})

// Query
var u User
err = users.FindByID(ctx, id, &u)
```

## API

### mongodb.Connect

```go
func Connect(ctx context.Context, uri string, opts ...ConnectOption) (*Client, error)
```

### ConnectConfig

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `MaxPoolSize` | `uint64` | `100` | Max connection pool size |
| `MinPoolSize` | `uint64` | `0` | Min keep-alive connections |
| `ConnectTimeout` | `time.Duration` | `10s` | Connection timeout |
| `ServerSelectionTimeout` | `time.Duration` | `30s` | Server selection timeout |

### TypedCollection[T] Methods

```go
type User struct { Name string; Age int }

users := mongodb.Collection[User](client, "db", "users")

// Insert
users.InsertOne(ctx, User{Name: "alice", Age: 18})
users.InsertMany(ctx, []User{{Name: "bob", Age: 20}, {Name: "carol", Age: 25}})

// Query
var u User
users.FindByID(ctx, id, &u)                       // Query by _id
users.FindOne(ctx, filter, &u)                  // Query by filter
cursor, _ := users.Find(ctx, filter)             // Multiple query
users.CountDocuments(ctx, filter)                // Count

// Update
users.UpdateByID(ctx, id, bson.M{"$set": bson.M{"age": 20}})
users.UpdateOne(ctx, filter, update)

// Delete
users.DeleteByID(ctx, id)
users.DeleteOne(ctx, filter)

// Aggregate
cursor, _ := users.Aggregate(ctx, pipeline)
```

## Complete Example

```go
package main

import (
    "context"
    "fmt"

    "go.mongodb.org/mongo-driver/v2/bson"
    "github.com/astra-go/astra/mongodb"
)

type User struct {
    Name string `bson:"name"`
    Age  int    `bson:"age"`
}

func main() {
    ctx := context.Background()
    client, _ := mongodb.Connect(ctx, "mongodb://localhost:27017")
    defer client.Disconnect(ctx)

    users := mongodb.Collection[User](client, "myapp", "users")

    // Insert
    users.InsertOne(ctx, User{Name: "Alice", Age: 18})

    // Query
    filter := bson.M{"name": "Alice"}
    var u User
    users.FindOne(ctx, filter, &u)
    fmt.Printf("Found: %+v\n", u)

    // Update
    users.UpdateOne(ctx, filter, bson.M{"$set": bson.M{"age": 19}})

    // Delete
    users.DeleteOne(ctx, filter)
}
```

## Module Dependencies

- `go.mongodb.org/mongo-driver/v2` — MongoDB official Go driver

## Notes

- Connection URI format: `mongodb://[user:pass@]host:port/[database]?options`
- T in `TypedCollection[T]` must include BSON tags (e.g., `bson:"field_name"`)
- `Find` returns a cursor; must close after use
