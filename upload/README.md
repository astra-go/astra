# Upload — File Upload

Streaming file upload middleware with request body size limit, per-file size limit, and custom storage.

## Features

- **Streaming Processing**: Files streamed directly to disk, constant memory footprint, avoids OOM
- **Request Body Limit**: `StreamLimit` prevents large request bodies from exhausting server resources
- **Per-File Size Limit**: `SaveFile` supports per-file size limit
- **Custom Storage Directory**: Upload files saved to specified directory
- **File Overwrite Prevention**: Optional unique filename generation to avoid conflicts

## Quick Start

### Global Limit

```go
import "github.com/astra-go/astra/upload"

app := astra.New()

// Global limit: max 50 MB per request
app.Use(upload.StreamLimit(50 << 20))

app.POST("/upload", func(c *astra.Ctx) error {
    path, info, err := c.SaveFile("avatar", upload.SaveFileConfig{
        MaxSize:    5 << 20,  // Max 5 MB per file
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

Limits entire request body size; returns HTTP 413 on exceeded.

### c.SaveFile

```go
func (c *Ctx) SaveFile(formKey string, cfg SaveFileConfig) (path string, info *FileInfo, err error)

type SaveFileConfig struct {
    MaxSize    int64   // Max bytes per file
    DestDir    string  // Destination directory (auto-created)
    Rename     func(orig string) string // Filename rename function, UUID by default
}

type FileInfo struct {
    Filename string    // Original filename
    Size     int64     // File size
    Header   *multipart.FileHeader // Original file header
}
```

### upload.UploadLimiter (Fine-Grained Control)

```go
app.Use(upload.UploadLimiter(upload.LimiterConfig{
    MaxBytes:     50 << 20,  // Max 50 MB request body
    MaxFiles:     5,          // Max 5 files
    MaxFileSize:  10 << 20,   // Max 10 MB per file
    MaxFieldSize: 1 << 20,    // Max 1 MB per form field
}))
```

## Config

### Error Handling

| Error | HTTP Status | Description |
|-------|------------|-------------|
| `ErrFileTooLarge` | 413 | Single file exceeds MaxSize |
| `ErrTooManyFiles` | 400 | Number of files exceeds limit |
| `ErrFieldTooLarge` | 400 | Form field exceeds limit |
| `ErrBodyTooLarge` | 413 | Request body exceeds limit |

## Complete Example

```go
package main

import (
    "github.com/astra-go/astra"
    "github.com/astra-go/astra/upload"
)

func main() {
    app := astra.New()

    // Global limit
    app.Use(upload.StreamLimit(50 << 20))

    app.POST("/upload", func(c *astra.Ctx) error {
        // Save single file
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
        // Save multiple files
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

## Module Dependencies

No external dependencies (uses Go standard library `mime/multipart`).

## Notes

- `DestDir` auto-created if doesn't exist; no manual creation needed
- Default filename uses UUID generation to avoid conflicts in same directory
- In production, recommend pairing with object storage (`storage` module) to upload files to OSS/S3
