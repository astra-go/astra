# Storage — 对象存储

统一对象存储接口，支持 AWS S3、阿里云 OSS、腾讯云 COS 和本地文件系统。

## 特性

- **统一接口**：`Storage` 接口定义所有存储操作；切换后端无需修改业务代码
- **预签名 URL**：`SignedURL` 生成下载 URL，`SignedPutURL` 生成上传 URL
- **流式读写**：基于 `io.Reader`/`io.ReadCloser`；内存占用恒定
- **元数据管理**：上传时支持设置 Content-Type、ContentLength 等
- **多后端**：S3（兼容 MinIO）、OSS、COS、Local

## 支持的后端

| 后端 | 导入路径 | 说明 |
|---------|-------------|-------------|
| AWS S3 | `storage/s3` | 兼容 MinIO |
| 阿里云 OSS | `storage/oss` | 阿里云 |
| 腾讯云 COS | `storage/cos` | 腾讯云 |
| 本地文件系统 | `storage/local` | 开发/测试 |

## 快速开始

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

### 上传文件

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

### 预签名 URL

```go
// 生成下载 URL（15 分钟有效期）
url, _ := store.SignedURL(ctx, "avatars/42.png", 15*time.Minute)

// 生成上传 URL（客户端直传存储）
putURL, _ := store.SignedPutURL(ctx, "avatars/42.png", 15*time.Minute, storage.PutOptions{
    ContentType: "image/png",
})
```

## API

### Storage 接口

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
    ContentType   string // MIME 类型，如 "image/png"
    ContentLength int64  // 文件大小（部分后端必需）
    Metadata      map[string]string // 自定义元数据
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

## 配置

### S3

| 选项 | 类型 | 说明 |
|--------|------|-------------|
| `Bucket` | `string` | Bucket 名称 |
| `Region` | `string` | AWS 区域 |
| `Endpoint` | `string` | 自定义端点（MinIO 必需） |
| `AccessKey` | `string` | Access Key |
| `SecretKey` | `string` | Secret Key |
| `PathStyle` | `bool` | Path-style（MinIO 必须为 true） |

### Local

```go
import storelocal "github.com/astra-go/astra/storage/local"

store, _ := storelocal.New(storelocal.Config{
    RootDir: "/data/storage",
    BaseURL: "https://cdn.example.com", // 公共访问基础 URL
})
```

## 完整示例

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

    // 上传
    content := []byte("Hello, World!")
    store.Put(ctx, "hello.txt", nil, io.NopCloser(io.Reader(nil)), storage.PutOptions{
        ContentType:   "text/plain",
        ContentLength: int64(len(content)),
    })

    // 生成预签名下载 URL
    url, _ := store.SignedURL(ctx, "hello.txt", 1*time.Hour)
    fmt.Println("Download:", url)
}
```

## 模块依赖

各子包依赖对应云厂商的 SDK。

## 注意事项

- 预签名 URL 有效期不宜过长（一般 15 分钟～1 小时），避免安全风险
- MinIO 必须设置 `PathStyle: true`；S3 默认使用 virtual-hosted style
- `SignedPutURL` 适合浏览器直传存储，减少 App 服务器带宽压力