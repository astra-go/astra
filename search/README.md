# search/elastic

`github.com/astra-go/astra/search/elastic` 提供针对 Elasticsearch 和 OpenSearch 的轻量级统一客户端，封装了最常用的索引、查询与管理操作。

- 兼容 Elasticsearch 7/8 和 AWS OpenSearch Service（均使用 Elasticsearch REST API）
- 基于 `go-elasticsearch/v8` 官方客户端，零额外依赖
- 提供 `Searcher` 接口，便于在测试中替换为 mock

---

## 安装

```bash
go get github.com/astra-go/astra/search/elastic
```

---

## 创建客户端

### 本地开发（无认证）

```go
import "github.com/astra-go/astra/search/elastic"

client, err := elastic.New(elastic.Config{
    Addresses: []string{"http://localhost:9200"},
})
```

### 用户名 + 密码认证

```go
client, err := elastic.New(elastic.Config{
    Addresses: []string{"https://my-es.example.com:9200"},
    Username:  "elastic",
    Password:  os.Getenv("ES_PASSWORD"),
})
```

### API Key 认证

```go
client, err := elastic.New(elastic.Config{
    Addresses: []string{"https://my-es.example.com:9200"},
    APIKey:    os.Getenv("ES_API_KEY"), // base64 编码的 "id:api_key"
})
```

### Elastic Cloud

```go
client, err := elastic.New(elastic.Config{
    CloudID: os.Getenv("ELASTIC_CLOUD_ID"),
    APIKey:  os.Getenv("ELASTIC_API_KEY"),
})
```

### 自定义 CA 证书 / 跳过 TLS 验证

```go
caCert, _ := os.ReadFile("/etc/ssl/certs/my-ca.pem")

client, err := elastic.New(elastic.Config{
    Addresses: []string{"https://secure-es:9200"},
    CACert:    caCert,
})

// ⚠️ 仅用于开发/测试
client, err = elastic.New(elastic.Config{
    Addresses:          []string{"https://dev-es:9200"},
    InsecureSkipVerify: true,
})
```

---

## 索引操作

### 写入单个文档

```go
type Product struct {
    Name     string  `json:"name"`
    Price    float64 `json:"price"`
    Category string  `json:"category"`
    Stock    int     `json:"stock"`
}

err := client.Index(ctx, elastic.IndexRequest{
    Index: "products",
    ID:    "prod-001",         // 留空则由 ES 自动生成
    Doc:   Product{
        Name:     "无线键盘",
        Price:    299.00,
        Category: "电子产品",
        Stock:    100,
    },
})
```

#### 使用 Ingest Pipeline

```go
err := client.Index(ctx, elastic.IndexRequest{
    Index:    "logs",
    Doc:      logEntry,
    Pipeline: "add-timestamp", // 写入前经过 ingest pipeline 处理
})
```

### 批量写入文档（Bulk API）

`BulkIndex` 将多个文档合并为一次 API 调用，适合批量导入数据。

```go
reqs := []elastic.IndexRequest{
    {Index: "products", ID: "prod-001", Doc: Product{Name: "键盘", Price: 299}},
    {Index: "products", ID: "prod-002", Doc: Product{Name: "鼠标", Price: 129}},
    {Index: "products", ID: "prod-003", Doc: Product{Name: "显示器", Price: 1999}},
}

err := client.BulkIndex(ctx, reqs)
```

> 批量写入不同索引时，每条 `IndexRequest.Index` 可以不同。

### 删除文档

```go
err := client.Delete(ctx, "products", "prod-001")
// 文档不存在时不报错（404 被静默忽略）
```

---

## 索引管理

### 创建索引（带 Mapping）

```go
mapping := map[string]any{
    "mappings": map[string]any{
        "properties": map[string]any{
            "name": map[string]any{
                "type":     "text",
                "analyzer": "ik_max_word", // 中文分词器
                "fields": map[string]any{
                    "keyword": map[string]any{"type": "keyword"}, // 精确匹配子字段
                },
            },
            "price": map[string]any{
                "type": "float",
            },
            "category": map[string]any{
                "type": "keyword",
            },
            "created_at": map[string]any{
                "type":   "date",
                "format": "strict_date_optional_time",
            },
        },
    },
    "settings": map[string]any{
        "number_of_shards":   3,
        "number_of_replicas": 1,
    },
}

err := client.CreateIndex(ctx, "products", mapping)
```

#### 创建默认 Mapping 索引

```go
// mapping 传 nil 使用 Elasticsearch 动态 mapping
err := client.CreateIndex(ctx, "logs", nil)
```

### 删除索引

```go
// 删除整个索引及其所有文档
err := client.DeleteIndex(ctx, "products")
```

---

## 搜索查询

`Search` 方法接受标准 Elasticsearch Query DSL，返回命中文档列表、总数和聚合结果。

```go
result, err := client.Search(ctx, elastic.SearchRequest{
    Index: []string{"products"},
    Query: /* Query DSL */,
    Size:  10,  // 返回条数，默认 10
    From:  0,   // 分页偏移
})
// result.Total — 匹配的总文档数
// result.Hits  — 本次返回的文档列表
// result.Aggs  — 聚合结果
```

`Hit` 结构：
```go
type Hit struct {
    Index  string         // 所属索引
    ID     string         // 文档 ID
    Score  float64        // 相关性得分（非评分查询为 0）
    Source map[string]any // 文档字段
}
```

---

### 全文搜索（match）

```go
result, err := client.Search(ctx, elastic.SearchRequest{
    Index: []string{"products"},
    Query: map[string]any{
        "match": map[string]any{
            "name": "无线键盘",
        },
    },
    Size: 10,
})

for _, hit := range result.Hits {
    fmt.Printf("ID: %s, 名称: %v\n", hit.ID, hit.Source["name"])
}
```

### 精确匹配（term）

```go
result, err := client.Search(ctx, elastic.SearchRequest{
    Index: []string{"products"},
    Query: map[string]any{
        "term": map[string]any{
            "category": "电子产品",
        },
    },
})
```

### 复合查询（bool）

```go
result, err := client.Search(ctx, elastic.SearchRequest{
    Index: []string{"products"},
    Query: map[string]any{
        "bool": map[string]any{
            "must": []any{
                map[string]any{"match": map[string]any{"name": "键盘"}},
            },
            "filter": []any{
                map[string]any{"term": map[string]any{"category": "电子产品"}},
                map[string]any{"range": map[string]any{
                    "price": map[string]any{"gte": 100, "lte": 500},
                }},
            },
            "must_not": []any{
                map[string]any{"term": map[string]any{"category": "配件"}},
            },
        },
    },
    Size: 20,
})
```

### 分页查询

```go
const pageSize = 10

// 第 1 页
result, _ := client.Search(ctx, elastic.SearchRequest{
    Index: []string{"products"},
    Query: map[string]any{"match_all": map[string]any{}},
    Size:  pageSize,
    From:  0,
})
total := result.Total // 总匹配数

// 第 N 页
page := 3
result, _ = client.Search(ctx, elastic.SearchRequest{
    Index: []string{"products"},
    Query: map[string]any{"match_all": map[string]any{}},
    Size:  pageSize,
    From:  (page - 1) * pageSize,
})
```

> `From + Size` 超过 10000 时需设置 `index.max_result_window`，或改用 Search After API（通过 `Query` 传递原始 DSL）。

### 排序

```go
result, err := client.Search(ctx, elastic.SearchRequest{
    Index: []string{"products"},
    Query: map[string]any{"match_all": map[string]any{}},
    Sort: []map[string]any{
        {"price": "asc"},           // 价格升序
        {"created_at": "desc"},     // 同价格时按时间降序
    },
    Size: 10,
})
```

### 字段过滤（_source）

```go
// 只返回 name 和 price 字段，减少网络传输
result, err := client.Search(ctx, elastic.SearchRequest{
    Index:  []string{"products"},
    Query:  map[string]any{"match_all": map[string]any{}},
    Source: []string{"name", "price"},
    Size:   50,
})
```

### 聚合（Aggregations）

```go
result, err := client.Search(ctx, elastic.SearchRequest{
    Index: []string{"products"},
    Query: map[string]any{"match_all": map[string]any{}},
    Size:  0, // 只需聚合结果，不返回文档
    Aggs: map[string]any{
        // 按分类统计商品数
        "by_category": map[string]any{
            "terms": map[string]any{
                "field": "category",
                "size":  10,
            },
        },
        // 价格统计
        "price_stats": map[string]any{
            "stats": map[string]any{
                "field": "price",
            },
        },
        // 价格区间直方图
        "price_histogram": map[string]any{
            "histogram": map[string]any{
                "field":    "price",
                "interval": 100,
            },
        },
    },
})

// 读取聚合结果（raw map，结构与 ES 响应一致）
if byCategory, ok := result.Aggs["by_category"].(map[string]any); ok {
    buckets := byCategory["buckets"].([]any)
    for _, b := range buckets {
        bucket := b.(map[string]any)
        fmt.Printf("分类: %v, 数量: %v\n", bucket["key"], bucket["doc_count"])
    }
}
```

### 跨索引搜索

```go
result, err := client.Search(ctx, elastic.SearchRequest{
    Index: []string{"products", "items", "goods"}, // 同时搜索多个索引
    Query: map[string]any{
        "multi_match": map[string]any{
            "query":  "蓝牙",
            "fields": []string{"name", "description"},
        },
    },
})
```

### 搜索全部索引

```go
result, err := client.Search(ctx, elastic.SearchRequest{
    Index: []string{"*"}, // 通配符匹配所有索引
    Query: map[string]any{"match_all": map[string]any{}},
    Size:  10,
})
```

---

## `Searcher` 接口

所有方法都通过 `Searcher` 接口暴露，便于测试时替换为 mock：

```go
type Searcher interface {
    Index(ctx context.Context, req IndexRequest) error
    BulkIndex(ctx context.Context, reqs []IndexRequest) error
    Search(ctx context.Context, req SearchRequest) (*SearchResult, error)
    Delete(ctx context.Context, index, id string) error
    DeleteIndex(ctx context.Context, index string) error
    CreateIndex(ctx context.Context, index string, mapping map[string]any) error
    Close() error
}
```

#### 在测试中使用 mock

```go
type mockSearcher struct{}

func (m *mockSearcher) Search(_ context.Context, req elastic.SearchRequest) (*elastic.SearchResult, error) {
    return &elastic.SearchResult{
        Total: 1,
        Hits: []elastic.Hit{
            {ID: "test-1", Source: map[string]any{"name": "测试商品"}},
        },
    }, nil
}

func (m *mockSearcher) Index(_ context.Context, _ elastic.IndexRequest) error         { return nil }
func (m *mockSearcher) BulkIndex(_ context.Context, _ []elastic.IndexRequest) error   { return nil }
func (m *mockSearcher) Delete(_ context.Context, _, _ string) error                   { return nil }
func (m *mockSearcher) DeleteIndex(_ context.Context, _ string) error                 { return nil }
func (m *mockSearcher) CreateIndex(_ context.Context, _ string, _ map[string]any) error { return nil }
func (m *mockSearcher) Close() error                                                   { return nil }

// 注入 mock
var searcher elastic.Searcher = &mockSearcher{}
```

---

## 在 Astra 应用中集成

```go
// main.go
saJSON, _ := os.ReadFile("firebase-service-account.json")
_ = saJSON // 示例：search 模块独立

esClient, err := elastic.New(elastic.Config{
    Addresses: []string{os.Getenv("ES_ADDRESS")},
    APIKey:    os.Getenv("ES_API_KEY"),
})
if err != nil {
    log.Fatal(err)
}
defer esClient.Close()

app := astra.New()

app.GET("/search", func(c astra.Context) error {
    q := c.Query("q")
    result, err := esClient.Search(c.Request().Context(), elastic.SearchRequest{
        Index: []string{"products"},
        Query: map[string]any{
            "multi_match": map[string]any{
                "query":  q,
                "fields": []string{"name^2", "description"},
            },
        },
        Size: 20,
    })
    if err != nil {
        return err
    }
    return c.JSON(http.StatusOK, result)
})
```

---

## 错误处理

- **连接/网络错误**：返回 `fmt.Errorf("elastic: ...: %w", err)`，可用 `errors.Is` / `errors.As` 解包。
- **ES 返回 HTTP 错误状态**：返回 `fmt.Errorf("elastic: operation index/id: HTTP status text")`，包含索引名、文档 ID 和 ES 状态。
- **`Delete` 的 404**：文档不存在不报错，静默成功（幂等删除）。
- **Context 取消**：客户端使用 `WithContext`，context 取消立即中止请求。

```go
_, err := esClient.Search(ctx, req)
if err != nil {
    // 错误格式: "elastic: search: [404 Not Found] ..."
    log.Printf("搜索失败: %v", err)
}
```
