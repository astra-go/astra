// Package cos provides a storage.Storage implementation backed by Tencent Cloud COS.
//
// # Usage
//
//	import storecos "github.com/astra-go/astra/storage/cos"
//
//	store, err := storecos.New(storecos.Config{
//	    BucketURL: "https://my-bucket-1234567890.cos.ap-guangzhou.myqcloud.com",
//	    SecretID:  "AKIDxxx",
//	    SecretKey: "yyy",
//	})
//
//	err = store.Put(ctx, "avatars/42.png", reader, storage.PutOptions{ContentType: "image/png"})
//	url, err := store.SignedURL(ctx, "avatars/42.png", 15*time.Minute)
package cos

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	cos "github.com/tencentyun/cos-go-sdk-v5"

	"github.com/astra-go/astra/storage"
)

// Config holds parameters required to connect to Tencent Cloud COS.
type Config struct {
	// BucketURL is the full bucket endpoint URL, e.g.
	// "https://my-bucket-1234567890.cos.ap-guangzhou.myqcloud.com"
	BucketURL string

	// SecretID is the Tencent Cloud API key ID.
	SecretID string

	// SecretKey is the Tencent Cloud API key secret.
	SecretKey string
}

// Store is a COS-backed Storage implementation.
type Store struct {
	client    *cos.Client
	secretID  string
	secretKey string
}

// New creates a COS-backed Store.
// All Config fields are required.
func New(cfg Config) (*Store, error) {
	if cfg.BucketURL == "" {
		return nil, fmt.Errorf("storage/cos: BucketURL is required")
	}
	if cfg.SecretID == "" || cfg.SecretKey == "" {
		return nil, fmt.Errorf("storage/cos: SecretID and SecretKey are required")
	}

	u, err := url.Parse(cfg.BucketURL)
	if err != nil {
		return nil, fmt.Errorf("storage/cos: invalid BucketURL %q: %w", cfg.BucketURL, err)
	}

	client := cos.NewClient(&cos.BaseURL{BucketURL: u}, &http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID:  cfg.SecretID,
			SecretKey: cfg.SecretKey,
		},
	})
	return &Store{
		client:    client,
		secretID:  cfg.SecretID,
		secretKey: cfg.SecretKey,
	}, nil
}

// Put uploads the content from r to key.
func (s *Store) Put(ctx context.Context, key string, r io.Reader, opts storage.PutOptions) error {
	putOpts := &cos.ObjectPutOptions{}
	if opts.ContentType != "" || opts.ContentLength > 0 || opts.ACL != "" {
		putOpts.ObjectPutHeaderOptions = &cos.ObjectPutHeaderOptions{}
		if opts.ContentType != "" {
			putOpts.ObjectPutHeaderOptions.ContentType = opts.ContentType
		}
		if opts.ContentLength > 0 {
			putOpts.ObjectPutHeaderOptions.ContentLength = opts.ContentLength
		}
		if opts.ACL != "" {
			putOpts.ACLHeaderOptions = &cos.ACLHeaderOptions{XCosACL: opts.ACL}
		}
	}
	_, err := s.client.Object.Put(ctx, key, r, putOpts)
	return wrapErr("Put", key, err)
}

// Get opens the object at key for reading.
// The caller must close the returned ReadCloser.
func (s *Store) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	resp, err := s.client.Object.Get(ctx, key, nil)
	if err != nil {
		return nil, wrapErr("Get", key, err)
	}
	return resp.Body, nil
}

// Delete removes the object at key.
func (s *Store) Delete(ctx context.Context, key string) error {
	_, err := s.client.Object.Delete(ctx, key)
	return wrapErr("Delete", key, err)
}

// Exists reports whether the object at key exists.
func (s *Store) Exists(ctx context.Context, key string) (bool, error) {
	ok, err := s.client.Object.IsExist(ctx, key)
	if err != nil {
		return false, wrapErr("Exists", key, err)
	}
	return ok, nil
}

// Stat returns metadata for the object at key.
func (s *Store) Stat(ctx context.Context, key string) (storage.ObjectInfo, error) {
	resp, err := s.client.Object.Head(ctx, key, nil)
	if err != nil {
		return storage.ObjectInfo{}, wrapErr("Stat", key, err)
	}
	info := storage.ObjectInfo{
		Key:         key,
		ContentType: resp.Header.Get("Content-Type"),
		ETag:        resp.Header.Get("ETag"),
	}
	if lm := resp.Header.Get("Last-Modified"); lm != "" {
		if t, err := http.ParseTime(lm); err == nil {
			info.LastModified = t
		}
	}
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		_, _ = fmt.Sscanf(cl, "%d", &info.Size)
	}
	return info, nil
}

// SignedURL generates a pre-signed GET URL valid for ttl.
func (s *Store) SignedURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	u, err := s.client.Object.GetPresignedURL(
		ctx, http.MethodGet, key,
		s.secretID, s.secretKey, ttl, nil,
	)
	if err != nil {
		return "", wrapErr("SignedURL", key, err)
	}
	return u.String(), nil
}

// SignedPutURL generates a pre-signed PUT URL valid for ttl.
func (s *Store) SignedPutURL(ctx context.Context, key string, ttl time.Duration, _ storage.PutOptions) (string, error) {
	u, err := s.client.Object.GetPresignedURL(
		ctx, http.MethodPut, key,
		s.secretID, s.secretKey, ttl, nil,
	)
	if err != nil {
		return "", wrapErr("SignedPutURL", key, err)
	}
	return u.String(), nil
}

func wrapErr(op, key string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("storage/cos: %s %q: %w", op, key, err)
}

// Verify Store implements storage.Storage at compile time.
var _ storage.Storage = (*Store)(nil)
