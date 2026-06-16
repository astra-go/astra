# Search — Search Engine Integration

Elasticsearch search engine integration.

## Features

- **Unified Client**: Wraps common Elasticsearch operations, supports multiple query types
- **Index Management**: Create, update, delete indices
- **Full-Text Search**: Supports match, multi_match, term, range queries
- **Bulk Operations**: Bulk API for efficient batch write/update/delete

## Quick Start

```go
import "github.com/astra-go/astra/search/elastic"

client, _ := elastic.New(elastic.Config{
    Addresses: []string{"http://localhost:9200"},
})

// Index document
client.Index(ctx, "posts", "1", map[string]any{
    "title": "Go Tutorial",
    "content": "Learn Go language...",
    "author": "alice",
})

// Search
results, _ := client.Search(ctx, "posts", elastic.Query{
    MultiMatch: &elastic.MultiMatchQuery{
        Query: "Go language",
        Fields: []string{"title", "content"},
    },
})
```

## API

### elastic.New

```go
func New(cfg Config) (*Client, error)

type Config struct {
    Addresses []string  // ES node address list
    Username  string    // Optional: auth username
    Password  string    // Optional: auth password
    IndexPrefix string  // Index prefix (e.g., "prod_")
}
```

### Index Operations

```go
// Create index (with mapping)
client.CreateIndex(ctx, "posts", mapping)

// Check if index exists
client.Exists(ctx, "posts")

// Delete index
client.DeleteIndex(ctx, "posts")
```

### Document Operations

```go
// Single write
client.Index(ctx, index, id, doc)

// Bulk write
client.BulkIndex(ctx, index, docs) // docs is []map[string]any

// Delete
client.Delete(ctx, index, id)
```

### Queries

```go
// Full-text search
client.Search(ctx, index, Query{
    MultiMatch: &MultiMatchQuery{
        Query:  "keywords",
        Fields: []string{"title", "content"},
    },
})

// Exact match
client.Search(ctx, index, Query{
    Term: map[string]any{"status": "published"},
})

// Range query
client.Search(ctx, index, Query{
    Range: map[string]any{
        "created_at": map[string]any{"gte": "2024-01-01"},
    },
})
```

## Config

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `Addresses` | `[]string` | — | ES node addresses (required) |
| `Username` | `string` | `""` | HTTP Basic auth user |
| `Password` | `string` | `""` | HTTP Basic auth password |
| `IndexPrefix` | `string` | `""` | Index name prefix |

## Module Dependencies

- `github.com/elastic/go-elasticsearch/v8` — Elasticsearch Go client

## Notes

- Production recommends configuring auth to prevent data leakage
- `IndexPrefix` for multi-environment isolation to avoid index name conflicts between environments
