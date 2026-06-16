# Pagination — Pagination Utility

Database query pagination helper, auto-parses pagination info from request parameters and converts to GORM Scope.

## Features

- **Request Parameter Parsing**: Auto-parses pagination from URL query parameters (`page`, `page_size`)
- **GORM Scope Integration**: Directly pass `Scopes(pagination.Scope(page))` in chain calls
- **Reasonable Defaults**: Uses reasonable default page number and page size when not specified
- **Total Count Helper**: Auto-calculates Offset; supports total page count calculation

## Quick Start

```go
import "github.com/astra-go/astra/pagination"

// Parse pagination from request
page := pagination.NewFromRequest(c, pagination.DefaultPageSize(20))

// Pass to GORM query
var users []User
db.Scopes(pagination.Scope(page)).Find(&users)

// Get total pages
totalPages := (page.Total + page.PageSize - 1) / page.PageSize
```

## API

### NewFromRequest

```go
func NewFromRequest(c contract.Context, defaultPageSize int) *Page
```

Parses pagination info from request `page` and `page_size` parameters.

### Page Structure

```go
type Page struct {
    PageNum  int // Current page (starts from 1)
    PageSize int // Items per page
    Offset  int // SQL OFFSET value (= (PageNum-1) * PageSize)
    Total   int64 // Total count (caller fills after query)
}
```

### pagination.Scope

```go
func Scope(page *Page) func(db *gorm.DB) *gorm.DB
```

GORM Scope function, pass `db.Scopes(Scope(page))`.

### Config Defaults

| Function | Description |
|----------|-------------|
| `DefaultPageSize(n)` | Set default items per page (default 10) |
| `MaxPageSize(n)` | Set max items per page (default 100) |

## Config

```go
// Default 20 per page, max 50
page := pagination.NewFromRequest(c,
    pagination.DefaultPageSize(20),
    pagination.MaxPageSize(50),
)
```

## Complete Example

```go
package main

import (
    "github.com/astra-go/astra"
    "github.com/astra-xin/astra/pagination"
)

type User struct { ID int; Name string }

func listUsers(c *astra.Ctx) error {
    // Parse pagination, default 20 per page, max 100
    page := pagination.NewFromRequest(c,
        pagination.DefaultPageSize(20),
        pagination.MaxPageSize(100),
    )

    // Count total
    var total int64
    db.Model(&User{}).Count(&total)

    // Paginated query
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

## Module Dependencies

- `gorm.io/gorm` — ORM integration

## Notes

- `PageNum` starts from 1, while SQL OFFSET starts from 0; `Offset` field does the conversion
- Total count must be filled by caller; `pagination` package doesn't auto-query
- `page_size=0` uses default items per page; `page=0` uses first page
