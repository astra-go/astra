# Search — 搜索引擎集成

Elasticsearch 搜索引擎集成。

## 特性

- **统一客户端**：封装常用 Elasticsearch 操作，支持多种查询类型
- **索引管理**：创建、更新、删除索引
- **全文搜索**：支持 match、multi_match、term、range 查询
- **批量操作**：Bulk API 高效批量写入/更新/删除

## 快速开始

```go
import "github.com/astra-go/astra/search/elastic"

client, _ := elastic.New(elastic.Config{
    Addresses: []string{"http://localhost:9200"},
})

// 索引文档
client.Index(ctx, "posts", "1", map[string]any{
    "title": "Go Tutorial",
    "content": "Learn Go language...",
    "author": "alice",
})

// 搜索
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
    Addresses []string  // ES 节点地址列表
    Username  string    // 可选：认证用户名
    Password  string    // 可选：认证密码
    IndexPrefix string  // 索引前缀（如 "prod_"）
}
```

### 索引操作

```go
// 创建索引（带 mapping）
client.CreateIndex(ctx, "posts", mapping)

// 检查索引是否存在
client.Exists(ctx, "posts")

// 删除索引
client.DeleteIndex(ctx, "posts")
```

### 文档操作

```go
// 单条写入
client.Index(ctx, index, id, doc)

// 批量写入
client.BulkIndex(ctx, index, docs) // docs 为 []map[string]any

// 删除
client.Delete(ctx, index, id)
```

### 查询

```go
// 全文搜索
client.Search(ctx, index, Query{
    MultiMatch: &MultiMatchQuery{
        Query:  "keywords",
        Fields: []string{"title", "content"},
    },
})

// 精确匹配
client.Search(ctx, index, Query{
    Term: map[string]any{"status": "published"},
})

// 范围查询
client.Search(ctx, index, Query{
    Range: map[string]any{
        "created_at": map[string]any{"gte": "2024-01-01"},
    },
})
```

## 配置

| 选项 | 类型 | 默认值 | 说明 |
|--------|------|---------|-------------|
| `Addresses` | `[]string` | — | ES 节点地址（必填） |
| `Username` | `string` | `""` | HTTP Basic 认证用户 |
| `Password` | `string` | `""` | HTTP Basic 认证密码 |
| `IndexPrefix` | `string` | `""` | 索引名前缀 |

## 模块依赖

- `github.com/elastic/go-elasticsearch/v8` — Elasticsearch Go 客户端

## 注意事项

- 生产环境建议配置认证，防止数据泄露
- `IndexPrefix` 用于多环境隔离，避免不同环境间索引名冲突