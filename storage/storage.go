// Package storage provides a unified object storage abstraction.
//
// The Storage interface covers the core operations (Put, Get, Delete, Exists,
// Stat, SignedURL, SignedPutURL) and is implemented by three backends:
//
//   - storage/s3  — AWS S3; also works with MinIO (set Endpoint + PathStyle)
//   - storage/oss — Alibaba Cloud OSS
//   - storage/cos — Tencent Cloud COS
//
// Switching backends requires only a one-line change to the initialisation call;
// all business code that accepts a storage.Storage interface is unchanged.
//
// # Usage
//
//	import (
//	    "github.com/astra-go/astra/storage"
//	    stores3 "github.com/astra-go/astra/storage/s3"
//	)
//
//	store, err := stores3.New(stores3.Config{
//	    Bucket:    "my-bucket",
//	    Region:    "us-east-1",
//	    AccessKey: "AKID...",
//	    SecretKey: "...",
//	})
//
//	// Upload
//	err = store.Put(ctx, "avatars/42.png", reader, storage.PutOptions{
//	    ContentType: "image/png",
//	})
//
//	// Pre-signed download URL (15 min)
//	url, err := store.SignedURL(ctx, "avatars/42.png", 15*time.Minute)
package storage

import (
	"context"
	"io"
	"time"
)

// Storage is the unified object storage interface.
// All backends (S3, OSS, COS) satisfy this interface.
type Storage interface {
	// Put uploads r to key. Metadata and content-type are set via opts.
	Put(ctx context.Context, key string, r io.Reader, opts PutOptions) error

	// Get opens the object at key for reading.
	// The caller must close the returned ReadCloser.
	Get(ctx context.Context, key string) (io.ReadCloser, error)

	// Delete removes the object at key.
	// Returns nil if the object does not exist.
	Delete(ctx context.Context, key string) error

	// Exists reports whether the object exists.
	Exists(ctx context.Context, key string) (bool, error)

	// Stat returns metadata about the object without downloading its content.
	Stat(ctx context.Context, key string) (ObjectInfo, error)

	// SignedURL generates a pre-signed GET URL valid for ttl.
	// Allows unauthenticated clients to download the object directly.
	SignedURL(ctx context.Context, key string, ttl time.Duration) (string, error)

	// SignedPutURL generates a pre-signed PUT URL valid for ttl.
	// Allows unauthenticated clients to upload directly to the backend,
	// bypassing the application server (browser / mobile direct upload).
	SignedPutURL(ctx context.Context, key string, ttl time.Duration, opts PutOptions) (string, error)
}

// PutOptions configures an object upload.
type PutOptions struct {
	// ContentType is the MIME type, e.g. "image/png". If empty the backend
	// may auto-detect or default to "application/octet-stream".
	ContentType string

	// ContentLength hints the object size in bytes.
	// Some backends require this for streaming uploads; -1 means unknown.
	ContentLength int64

	// ACL controls the canned access policy.
	// Common values: "private" (default), "public-read".
	// Leave empty to use the bucket default.
	ACL string

	// Metadata stores arbitrary key/value pairs alongside the object.
	Metadata map[string]string
}

// ObjectInfo carries metadata returned by Stat.
type ObjectInfo struct {
	Key          string
	Size         int64
	ContentType  string
	ETag         string
	LastModified time.Time
}
