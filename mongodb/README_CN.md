# MongoDB — MongoDB 数据库集成

轻量级封装官方 MongoDB Go Driver v2，提供泛型 Collection 接口和安全连接池配置。

## 特性

- **泛型 Collections**：`TypedCollection[T]` 预定义 CRUD 方法，无需手动类型断言
- **连接池**：安全的生产默认值（MaxPoolSize=100），支持最小连接保活
- **BSON v2**：使用 `go.mongodb.org/mongo-driver/v2`
- **简化 API**：`Ping`、`Disconnect`、`DB` 等快捷方法

## 快速开始

```go
import "github.com/astra-go/astra/mongodb"

ctx := context.Background()

// 连接
client, err := mongodb.Connect(ctx, "mongodb://localhost:27017")
defer client.Disconnect(ctx)

// 获取泛型 Collection
users := mongodb.Collection[User](client, "mydb", "users")

// 插入
_ = users.InsertOne(ctx, User{Name: "alice", Age: 18})

// 查询
var u User
err = users.FindByID(ctx, id, &u)
```

## API

### mongodb.Connect

```go
func Connect(ctx context.Context, uri string, opts ...ConnectOption) (*Client, error)
```

### ConnectConfig

| 选项 | 类型 | 默认值 | 说明 |
|--------|------|---------|-------------|
| `MaxPoolSize` | `uint64` | `100` | 最大连接池大小 |
| `MinPoolSize` | `uint64` | `0` | 最小保活连接数 |
| `ConnectTimeout` | `time.Duration` | `10s` | 连接超时 |
| `ServerSelectionTimeout` | `time.Duration` | `30s` | 服务器选择超时 |

### TypedCollection[T] 方法

```go
type User struct { Name string; Age int }

users := mongodb.Collection[User](client, "db", "users")

// 插入
users.InsertOne(ctx, User{Name: "alice", Age: 18})
users.InsertMany(ctx, []User{{Name: "bob", Age: 20}, {Name: "carol", Age: 25}})

// 查询
var u User
users.FindByID(ctx, id, &u)                       // 按 _id 查询
users.FindOne(ctx, filter, &u)                  // 按 filter 查询
cursor, _ := users.Find(ctx, filter)             // 多条查询
users.CountDocuments(ctx, filter)                // 计数

// 更新
users.UpdateByID(ctx, id, bson.M{"$set": bson.M{"age": 20}})
users.UpdateOne(ctx, filter, update)

// 删除
users.DeleteByID(ctx, id)
users.DeleteOne(ctx, filter)

// 聚合
cursor, _ := users.Aggregate(ctx, pipeline)
```

## 完整示例

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

    // 插入
    users.InsertOne(ctx, User{Name: "Alice", Age: 18})

    // 查询
    filter := bson.M{"name": "Alice"}
    var u User
    users.FindOne(ctx, filter, &u)
    fmt.Printf("Found: %+v\n", u)

    // 更新
    users.UpdateOne(ctx, filter, bson.M{"$set": bson.M{"age": 19}})

    // 删除
    users.DeleteOne(ctx, filter)
}
```

## 模块依赖

- `go.mongodb.org/mongo-driver/v2` — MongoDB 官方 Go 驱动

## 注意事项

- 连接 URI 格式：`mongodb://[user:pass@]host:port/[database]?options`
- `TypedCollection[T]` 中的 T 必须包含 BSON tag（如 `bson:"field_name"`）
- `Find` 返回游标；使用后必须关闭