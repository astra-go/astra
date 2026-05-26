// Package s3 provides a storage.Storage implementation backed by AWS S3.
//
// The same backend works for any S3-compatible service (MinIO, Cloudflare R2,
// Backblaze B2, …) by setting Config.Endpoint and Config.PathStyle = true.
//
// # AWS S3
//
//	store, err := s3.New(s3.Config{
//	    Bucket:    "my-bucket",
//	    Region:    "us-east-1",
//	    AccessKey: "AKID...",
//	    SecretKey: "SECRET...",
//	})
//
// # MinIO (local / on-prem)
//
//	store, err := s3.New(s3.Config{
//	    Bucket:    "my-bucket",
//	    Region:    "us-east-1",          // arbitrary for MinIO
//	    Endpoint:  "http://localhost:9000",
//	    PathStyle: true,                 // required for MinIO
//	    AccessKey: "minioadmin",
//	    SecretKey: "minioadmin",
//	})
//
// # Cloudflare R2
//
//	store, err := s3.New(s3.Config{
//	    Bucket:    "my-bucket",
//	    Region:    "auto",
//	    Endpoint:  "https://<account-id>.r2.cloudflarestorage.com",
//	    PathStyle: false,
//	    AccessKey: "...",
//	    SecretKey: "...",
//	})
package s3

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/astra-go/astra/storage"
)

// Config holds all parameters required to connect to an S3-compatible service.
type Config struct {
	// Bucket is the target S3 bucket name.
	Bucket string

	// Region is the AWS region, e.g. "us-east-1".
	// For MinIO or other non-AWS services any non-empty value is accepted.
	Region string

	// Endpoint overrides the default AWS endpoint.
	// Required for MinIO, Cloudflare R2, Backblaze B2, etc.
	// Example: "http://localhost:9000"
	Endpoint string

	// PathStyle forces path-style addressing (bucket in path, not subdomain).
	// Must be true for MinIO.
	PathStyle bool

	// AccessKey is the AWS Access Key ID (or equivalent credential).
	AccessKey string

	// SecretKey is the AWS Secret Access Key (or equivalent credential).
	SecretKey string
}

// Store is an S3-backed Storage implementation.
type Store struct {
	client *s3.Client
	bucket string
}

// New creates an S3-backed Store.
// All Config fields except Endpoint and PathStyle are required.
func New(cfg Config) (*Store, error) {
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("storage/s3: Bucket is required")
	}
	if cfg.Region == "" {
		return nil, fmt.Errorf("storage/s3: Region is required")
	}
	if cfg.AccessKey == "" || cfg.SecretKey == "" {
		return nil, fmt.Errorf("storage/s3: AccessKey and SecretKey are required")
	}

	s3opts := s3.Options{
		Region: cfg.Region,
		Credentials: credentials.NewStaticCredentialsProvider(
			cfg.AccessKey, cfg.SecretKey, "",
		),
		UsePathStyle: cfg.PathStyle,
	}
	if cfg.Endpoint != "" {
		s3opts.BaseEndpoint = aws.String(cfg.Endpoint)
	}
	client := s3.New(s3opts)

	return &Store{client: client, bucket: cfg.Bucket}, nil
}

// Put uploads the content from r to key.
func (s *Store) Put(ctx context.Context, key string, r io.Reader, opts storage.PutOptions) error {
	in := &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   r,
	}
	if opts.ContentType != "" {
		in.ContentType = aws.String(opts.ContentType)
	}
	if opts.ContentLength > 0 {
		in.ContentLength = aws.Int64(opts.ContentLength)
	}
	if opts.ACL != "" {
		in.ACL = types.ObjectCannedACL(opts.ACL)
	}
	if len(opts.Metadata) > 0 {
		in.Metadata = opts.Metadata
	}
	_, err := s.client.PutObject(ctx, in)
	return wrapErr("Put", key, err)
}

// Get opens the object at key for reading.
// The caller is responsible for closing the returned ReadCloser.
func (s *Store) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, wrapErr("Get", key, err)
	}
	return out.Body, nil
}

// Delete removes the object at key.
func (s *Store) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	return wrapErr("Delete", key, err)
}

// Exists reports whether the object at key exists.
func (s *Store) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNotFound(err) {
			return false, nil
		}
		return false, wrapErr("Exists", key, err)
	}
	return true, nil
}

// Stat returns metadata for the object at key without downloading its content.
func (s *Store) Stat(ctx context.Context, key string) (storage.ObjectInfo, error) {
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return storage.ObjectInfo{}, wrapErr("Stat", key, err)
	}
	info := storage.ObjectInfo{Key: key}
	if out.ContentLength != nil {
		info.Size = *out.ContentLength
	}
	if out.ContentType != nil {
		info.ContentType = *out.ContentType
	}
	if out.ETag != nil {
		info.ETag = *out.ETag
	}
	if out.LastModified != nil {
		info.LastModified = *out.LastModified
	}
	return info, nil
}

// SignedURL generates a pre-signed GET URL valid for ttl.
func (s *Store) SignedURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	pc := s3.NewPresignClient(s.client)
	req, err := pc.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", wrapErr("SignedURL", key, err)
	}
	return req.URL, nil
}

// SignedPutURL generates a pre-signed PUT URL valid for ttl.
func (s *Store) SignedPutURL(ctx context.Context, key string, ttl time.Duration, opts storage.PutOptions) (string, error) {
	in := &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}
	if opts.ContentType != "" {
		in.ContentType = aws.String(opts.ContentType)
	}
	if opts.ACL != "" {
		in.ACL = types.ObjectCannedACL(opts.ACL)
	}
	pc := s3.NewPresignClient(s.client)
	req, err := pc.PresignPutObject(ctx, in, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", wrapErr("SignedPutURL", key, err)
	}
	return req.URL, nil
}

// isNotFound reports whether err is an S3 404 / NoSuchKey error.
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	// The aws-sdk-go-v2 surfaces not-found as *types.NoSuchKey or HTTP 404
	// wrapped in smithy errors; string matching is the safe fallback.
	type notFounder interface{ ErrorCode() string }
	if nf, ok := err.(notFounder); ok {
		code := nf.ErrorCode()
		return code == "NoSuchKey" || code == "NotFound" || code == "404"
	}
	return false
}

func wrapErr(op, key string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("storage/s3: %s %q: %w", op, key, err)
}

// Verify Store implements storage.Storage at compile time.
var _ storage.Storage = (*Store)(nil)
