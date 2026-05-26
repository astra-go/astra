// Package oss provides a storage.Storage implementation backed by Alibaba Cloud OSS.
//
// # Usage
//
//	import storeoss "github.com/astra-go/astra/storage/oss"
//
//	store, err := storeoss.New(storeoss.Config{
//	    Endpoint:  "https://oss-cn-hangzhou.aliyuncs.com",
//	    Bucket:    "my-bucket",
//	    AccessKey: "LTAI...",
//	    SecretKey: "...",
//	})
//
//	err = store.Put(ctx, "avatars/42.png", reader, storage.PutOptions{ContentType: "image/png"})
//	url, err := store.SignedURL(ctx, "avatars/42.png", 15*time.Minute)
package oss

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	ossdk "github.com/aliyun/aliyun-oss-go-sdk/oss"

	"github.com/astra-go/astra/storage"
)

// Config holds parameters required to connect to Alibaba Cloud OSS.
type Config struct {
	// Endpoint is the OSS region endpoint, e.g.
	// "https://oss-cn-hangzhou.aliyuncs.com"
	Endpoint string

	// Bucket is the target OSS bucket name.
	Bucket string

	// AccessKey is the Alibaba Cloud Access Key ID.
	AccessKey string

	// SecretKey is the Alibaba Cloud Access Key Secret.
	SecretKey string
}

// Store is an OSS-backed Storage implementation.
type Store struct {
	bucket *ossdk.Bucket
}

// New creates an OSS-backed Store.
// All Config fields are required.
func New(cfg Config) (*Store, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("storage/oss: Endpoint is required")
	}
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("storage/oss: Bucket is required")
	}
	if cfg.AccessKey == "" || cfg.SecretKey == "" {
		return nil, fmt.Errorf("storage/oss: AccessKey and SecretKey are required")
	}

	client, err := ossdk.New(cfg.Endpoint, cfg.AccessKey, cfg.SecretKey)
	if err != nil {
		return nil, fmt.Errorf("storage/oss: create client: %w", err)
	}
	bucket, err := client.Bucket(cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("storage/oss: open bucket %q: %w", cfg.Bucket, err)
	}
	return &Store{bucket: bucket}, nil
}

// Put uploads the content from r to key.
func (s *Store) Put(_ context.Context, key string, r io.Reader, opts storage.PutOptions) error {
	var options []ossdk.Option
	if opts.ContentType != "" {
		options = append(options, ossdk.ContentType(opts.ContentType))
	}
	if opts.ContentLength > 0 {
		options = append(options, ossdk.ContentLength(opts.ContentLength))
	}
	if opts.ACL != "" {
		options = append(options, ossdk.ObjectACL(ossdk.ACLType(opts.ACL)))
	}
	for k, v := range opts.Metadata {
		options = append(options, ossdk.Meta(k, v))
	}
	if err := s.bucket.PutObject(key, r, options...); err != nil {
		return wrapErr("Put", key, err)
	}
	return nil
}

// Get opens the object at key for reading.
// The caller must close the returned ReadCloser.
func (s *Store) Get(_ context.Context, key string) (io.ReadCloser, error) {
	rc, err := s.bucket.GetObject(key)
	if err != nil {
		return nil, wrapErr("Get", key, err)
	}
	return rc, nil
}

// Delete removes the object at key.
func (s *Store) Delete(_ context.Context, key string) error {
	if err := s.bucket.DeleteObject(key); err != nil {
		return wrapErr("Delete", key, err)
	}
	return nil
}

// Exists reports whether the object at key exists.
func (s *Store) Exists(_ context.Context, key string) (bool, error) {
	ok, err := s.bucket.IsObjectExist(key)
	if err != nil {
		return false, wrapErr("Exists", key, err)
	}
	return ok, nil
}

// Stat returns metadata for the object at key.
func (s *Store) Stat(_ context.Context, key string) (storage.ObjectInfo, error) {
	header, err := s.bucket.GetObjectDetailedMeta(key)
	if err != nil {
		return storage.ObjectInfo{}, wrapErr("Stat", key, err)
	}
	info := storage.ObjectInfo{
		Key:         key,
		ContentType: header.Get("Content-Type"),
		ETag:        header.Get("ETag"),
	}
	if lm := header.Get("Last-Modified"); lm != "" {
		if t, err := http.ParseTime(lm); err == nil {
			info.LastModified = t
		}
	}
	if cl := header.Get("Content-Length"); cl != "" {
		_, _ = fmt.Sscanf(cl, "%d", &info.Size)
	}
	return info, nil
}

// SignedURL generates a pre-signed GET URL valid for ttl.
func (s *Store) SignedURL(_ context.Context, key string, ttl time.Duration) (string, error) {
	url, err := s.bucket.SignURL(key, ossdk.HTTPGet, int64(ttl.Seconds()))
	if err != nil {
		return "", wrapErr("SignedURL", key, err)
	}
	return url, nil
}

// SignedPutURL generates a pre-signed PUT URL valid for ttl.
func (s *Store) SignedPutURL(_ context.Context, key string, ttl time.Duration, opts storage.PutOptions) (string, error) {
	var options []ossdk.Option
	if opts.ContentType != "" {
		options = append(options, ossdk.ContentType(opts.ContentType))
	}
	url, err := s.bucket.SignURL(key, ossdk.HTTPPut, int64(ttl.Seconds()), options...)
	if err != nil {
		return "", wrapErr("SignedPutURL", key, err)
	}
	return url, nil
}

func wrapErr(op, key string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("storage/oss: %s %q: %w", op, key, err)
}

// Verify Store implements storage.Storage at compile time.
var _ storage.Storage = (*Store)(nil)
