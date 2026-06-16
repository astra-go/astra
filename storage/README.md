# Storage — Object Storage

Unified object storage interface supporting AWS S3, Alibaba Cloud OSS, Tencent Cloud COS, and local filesystem.

## Features

- **Unified Interface**: `Storage` interface defines all storage operations; switching backends requires no business code changes
- **Presigned URLs**: `SignedURL` generates download URL, `SignedPutURL` generates upload URL
- **Streaming Read/Write**: Based on `io.Reader`/`io.ReadCloser`; constant memory footprint
- **Metadata Management**: Supports setting Content-Type, ContentLength, etc. on upload
- **Multi-Backend**: S3 (MinIO compatible), OSS, COS, Local

## Supported Backends

| Backend | Import Path | Description |
|---------|-------------|-------------|
| AWS S3 | `storage/s3` | MinIO compatible |
| Alibaba Cloud OSS | `storage/oss` | Alibaba Cloud |
| Tencent Cloud COS | `storage/cos` | Tencent Cloud |
| Local filesystem | `storage/local` | Dev/testing |

## Quick Start

### S3

```go
import stores3 "github.com/astra-go/astra/storage/s3"

store, _ := stores3.New(stores3.Config{
    Bucket:    "my-bucket",
    Region:    "us-east-1",
    AccessKey: "AKID...",
    SecretKey: "...",
})
```

### Upload File

```go
import (
    "github.com/astra-go/astra/storage"
    "os"
)

f, _ := os.Open("avatar.png")
defer f.Close()

err := store.Put(ctx, "avatars/42.png", f, storage.PutOptions{
    ContentType: "image/png",
})
```

### Presigned URLs

```go
// Generate download URL (15 min validity)
url, _ := store.SignedURL(ctx, "avatars/42.png", 15*time.Minute)

// Generate upload URL (client uploads directly to storage)
putURL, _ := store.SignedPutURL(ctx, "avatars/42.png", 15*time.Minute, storage.PutOptions{
    ContentType: "image/png",
})
```

## API

### Storage Interface

```go
type Storage interface {
    Put(ctx context.Context, key string, r io.Reader, opts PutOptions) error
    Get(ctx context.Context, key string) (io.ReadCloser, error)
    Delete(ctx context.Context, key string) error
    Exists(ctx context.Context, key string) (bool, error)
    Stat(ctx context.Context, key string) (ObjectInfo, error)
    SignedURL(ctx context.Context, key string, ttl time.Duration) (string, error)
    SignedPutURL(ctx context.Context, key string, ttl time.Duration, opts PutOptions) (string, error)
}
```

### PutOptions

```go
type PutOptions struct {
    ContentType   string // MIME type, e.g. "image/png"
    ContentLength int64  // File size (required by some backends)
    Metadata      map[string]string // Custom metadata
}
```

### ObjectInfo

```go
type ObjectInfo struct {
    Key          string
    Size         int64
    ContentType  string
    LastModified time.Time
    ETag         string
}
```

## Config

### S3

| Option | Type | Description |
|--------|------|-------------|
| `Bucket` | `string` | Bucket name |
| `Region` | `string` | AWS region |
| `Endpoint` | `string` | Custom endpoint (required for MinIO) |
| `AccessKey` | `string` | Access key |
| `SecretKey` | `string` | Secret key |
| `PathStyle` | `bool` | Path-style (required true for MinIO) |

### Local

```go
import storelocal "github.com/astra-go/astra/storage/local"

store, _ := storelocal.New(storelocal.Config{
    RootDir: "/data/storage",
    BaseURL: "https://cdn.example.com", // Public access base URL
})
```

## Complete Example

```go
package main

import (
    "context"
    "fmt"
    "io"
    "time"

    stores3 "github.com/astra-go/astra/storage/s3"
    "github.com/astra-go/astra/storage"
)

func main() {
    store, _ := stores3.New(stores3.Config{
        Bucket:    "my-bucket",
        Region:    "us-east-1",
        AccessKey: "AKIA...",
        SecretKey: "...",
    })
    ctx := context.Background()

    // Upload
    content := []byte("Hello, World!")
    store.Put(ctx, "hello.txt", nil, io.NopCloser(io.Reader(nil)), storage.PutOptions{
        ContentType:   "text/plain",
        ContentLength: int64(len(content)),
    })

    // Generate presigned download URL
    url, _ := store.SignedURL(ctx, "hello.txt", 1*time.Hour)
    fmt.Println("Download:", url)
}
```

## Module Dependencies

Each sub-package depends on the corresponding cloud provider's SDK.

## Notes

- Presigned URL validity should not be too long (generally 15 min~1 hour) to avoid security risks
- `PathStyle: true` is required for MinIO; S3 uses virtual-hosted style by default
- `SignedPutURL` is suitable for browser direct upload, reducing app server bandwidth pressure
