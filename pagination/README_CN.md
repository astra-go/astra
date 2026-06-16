# Pagination — 分页工具

数据库查询分页辅助工具，自动从请求参数解析分页信息并转换为 GORM Scope。

## 特性

- **请求参数解析**：自动解析 URL 查询参数中的分页信息（`page`、`page_size`）
- **GORM Scope 集成**：直接在链式调用中传入 `Scopes(pagination.Scope(page))`
- **合理默认值**：未指定时使用合理的默认页码和每页条数
- **总数辅助**：自动计算 Offset；支持总页数计算

## 快速开始

```go
import "github.com/astra-go/astra/pagination"

// 从请求解析分页
page := pagination.NewFromRequest(c, pagination.DefaultPageSize(20))

// 传入 GORM 查询
var users []User
db.Scopes(pagination.Scope(page)).Find(&users)

// 获取总页数
totalPages := (page.Total + page.PageSize - 1) / page.PageSize
```

## API

### NewFromRequest

```go
func NewFromRequest(c contract.Context, defaultPageSize int) *Page
```

从请求的 `page` 和 `page_size` 参数解析分页信息。

### Page 结构体

```go
type Page struct {
    PageNum  int // 当前页（从 1 开始）
    PageSize int // 每页条数
    Offset  int // SQL OFFSET 值（= (PageNum-1) * PageSize）
    Total   int64 // 总数（调用方查询后填充）
}
```

### pagination.Scope

```go
func Scope(page *Page) func(db *gorm.DB) *gorm.DB
```

GORM Scope 函数，传入 `db.Scopes(Scope(page))`。

### 配置默认值

| 函数 | 说明 |
|----------|-------------|
| `DefaultPageSize(n)` | 设置默认每页条数（默认 10） |
| `MaxPageSize(n)` | 设置最大每页条数（默认 100） |

## 配置

```go
// 默认每页 20 条，最大 50 条
page := pagination.NewFromRequest(c,
    pagination.DefaultPageSize(20),
    pagination.MaxPageSize(50),
)
```

## 完整示例

```go
package main

import (
    "github.com/astra-go/astra"
    "github.com/astra-xin/astra/pagination"
)

type User struct { ID int; Name string }

func listUsers(c *astra.Ctx) error {
    // 解析分页，默认每页 20 条，最大 100 条
    page := pagination.NewFromRequest(c,
        pagination.DefaultPageSize(20),
        pagination.MaxPageSize(100),
    )

    // 查询总数
    var total int64
    db.Model(&User{}).Count(&total)

    // 分页查询
    var users []User
    db.Scopes(pagination.Scope(page)).Find(&users)

    page.Total = total

    return c.JSON(200, astra.Map{
        "data":  users,
        "page":  page.PageNum,
        "size":  page.PageSize,
        "total": page.Total,
    })
}
```

## 模块依赖

- `gorm.io/gorm` — ORM 集成

## 注意事项

- `PageNum` 从 1 开始，而 SQL OFFSET 从 0 开始；`Offset` 字段完成转换
- 总数需由调用方填充；`pagination` 包不会自动查询
- `page_size=0` 使用默认每页条数；`page=0` 使用第一页