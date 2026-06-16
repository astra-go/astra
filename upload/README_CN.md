# Upload — 文件上传

流式文件上传中间件，支持请求体大小限制、单文件大小限制和自定义存储。

## 特性

- **流式处理**：文件直接流式写入磁盘，内存占用恒定，避免 OOM
- **请求体限制**：`StreamLimit` 防止大请求体耗尽服务器资源
- **单文件大小限制**：`SaveFile` 支持单文件大小限制
- **自定义存储目录**：上传文件保存到指定目录
- **文件覆盖防护**：可选唯一文件名生成，避免冲突

## 快速开始

### 全局限制

```go
import "github.com/astra-go/astra/upload"

app := astra.New()

// 全局限制：每个请求最多 50 MB
app.Use(upload.StreamLimit(50 << 20))

app.POST("/upload", func(c *astra.Ctx) error {
    path, info, err := c.SaveFile("avatar", upload.SaveFileConfig{
        MaxSize:    5 << 20,  // 单文件最大 5 MB
        DestDir:    "/tmp/uploads",
    })
    if err != nil {
        return err
    }
    return c.JSON(200, astra.Map{
        "path":      path,
        "size":      info.Size,
        "filename":  info.Filename,
    })
})
```

## API

### upload.StreamLimit

```go
func StreamLimit(maxBytes int64) astra.MiddlewareFunc
```

限制整个请求体大小；超出返回 HTTP 413。

### c.SaveFile

```go
func (c *Ctx) SaveFile(formKey string, cfg SaveFileConfig) (path string, info *FileInfo, err error)

type SaveFileConfig struct {
    MaxSize    int64   // 单文件最大字节数
    DestDir    string  // 目标目录（自动创建）
    Rename     func(orig string) string // 文件名重命名函数，默认为 UUID
}

type FileInfo struct {
    Filename string    // 原始文件名
    Size     int64     // 文件大小
    Header   *multipart.FileHeader // 原始文件头
}
```

### upload.UploadLimiter（细粒度控制）

```go
app.Use(upload.UploadLimiter(upload.LimiterConfig{
    MaxBytes:     50 << 20,  // 请求体最大 50 MB
    MaxFiles:     5,          // 最多 5 个文件
    MaxFileSize:  10 << 20,   // 单文件最大 10 MB
    MaxFieldSize: 1 << 20,    // 单个表单字段最大 1 MB
}))
```

## 配置

### 错误处理

| 错误 | HTTP 状态 | 说明 |
|-------|------------|-------------|
| `ErrFileTooLarge` | 413 | 单文件超过 MaxSize |
| `ErrTooManyFiles` | 400 | 文件数量超限 |
| `ErrFieldTooLarge` | 400 | 表单字段超限 |
| `ErrBodyTooLarge` | 413 | 请求体超限 |

## 完整示例

```go
package main

import (
    "github.com/astra-go/astra"
    "github.com/astra-go/astra/upload"
)

func main() {
    app := astra.New()

    // 全局限制
    app.Use(upload.StreamLimit(50 << 20))

    app.POST("/upload", func(c *astra.Ctx) error {
        // 保存单个文件
        path, info, err := c.SaveFile("avatar", upload.SaveFileConfig{
            MaxSize: 5 << 20, // 5 MB
            DestDir: "/tmp/uploads/avatars",
        })
        if err != nil {
            return c.JSON(400, astra.Map{"error": err.Error()})
        }
        return c.JSON(200, astra.Map{
            "path":     path,
            "filename": info.Filename,
            "size":     info.Size,
        })
    })

    app.POST("/upload-batch", func(c *astra.Ctx) error {
        // 保存多个文件
        form, _ := c.Request().FormFile("files")
        for _, fh := range form {
            path, _, _ := c.SaveFile(fh.Fieldname, upload.SaveFileConfig{
                MaxSize: 10 << 20,
                DestDir: "/tmp/uploads",
            })
            println(path)
        }
        return c.JSON(200, astra.Map{"ok": true})
    })

    app.Run(":8080")
}
```

## 模块依赖

无外部依赖（使用 Go 标准库 `mime/multipart`）。

## 注意事项

- `DestDir` 不存在时自动创建；无需手动创建
- 默认文件名使用 UUID 生成，避免同一目录下冲突
- 生产环境建议配合对象存储（`storage` 模块）将文件上传到 OSS/S3