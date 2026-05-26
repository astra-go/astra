// Package local provides a filesystem-backed storage.Storage implementation.
//
// Intended for development and testing only — do not use in production.
// All objects are stored as regular files under Config.RootDir.
//
// # Usage
//
//	store, err := local.New(local.Config{
//	    RootDir: "/tmp/storage",
//	    BaseURL: "http://localhost:8080/files",
//	})
//
//	err = store.Put(ctx, "avatars/42.png", reader, storage.PutOptions{})
//	url, err := store.SignedURL(ctx, "avatars/42.png", 15*time.Minute)
//	// → "http://localhost:8080/files/avatars/42.png"
package local

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/astra-go/astra/storage"
)

// Config configures the local Store.
type Config struct {
	// RootDir is the base directory for all stored objects.
	// It is created automatically if it does not exist.
	RootDir string

	// BaseURL is used to construct SignedURL / SignedPutURL responses.
	// Example: "http://localhost:8080/files"
	// If empty, SignedURL returns a file:// URL.
	BaseURL string
}

// Store is a filesystem-backed Storage implementation.
type Store struct {
	root    string
	baseURL string
}

// New creates a local Store. RootDir is created if it does not exist.
func New(cfg Config) (*Store, error) {
	if cfg.RootDir == "" {
		return nil, fmt.Errorf("storage/local: RootDir is required")
	}
	if err := os.MkdirAll(cfg.RootDir, 0o755); err != nil {
		return nil, fmt.Errorf("storage/local: create root dir: %w", err)
	}
	slog.Warn("storage/local: using filesystem backend — not suitable for production",
		slog.String("root", cfg.RootDir))
	return &Store{root: cfg.RootDir, baseURL: strings.TrimRight(cfg.BaseURL, "/")}, nil
}

// absPath converts a storage key to an absolute filesystem path.
// It rejects keys that would escape RootDir via path traversal.
func (s *Store) absPath(key string) (string, error) {
	clean := filepath.Join(s.root, filepath.FromSlash(key))
	if !strings.HasPrefix(clean, filepath.Clean(s.root)+string(os.PathSeparator)) &&
		clean != filepath.Clean(s.root) {
		return "", fmt.Errorf("storage/local: key %q escapes root directory", key)
	}
	return clean, nil
}

// Put writes r to the file at key, creating parent directories as needed.
func (s *Store) Put(_ context.Context, key string, r io.Reader, _ storage.PutOptions) error {
	path, err := s.absPath(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("storage/local: Put %q: mkdir: %w", key, err)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("storage/local: Put %q: create: %w", key, err)
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("storage/local: Put %q: write: %w", key, err)
	}
	return nil
}

// Get opens the file at key for reading.
func (s *Store) Get(_ context.Context, key string) (io.ReadCloser, error) {
	path, err := s.absPath(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("storage/local: Get %q: not found", key)
		}
		return nil, fmt.Errorf("storage/local: Get %q: %w", key, err)
	}
	return f, nil
}

// Delete removes the file at key. Returns nil if the file does not exist.
func (s *Store) Delete(_ context.Context, key string) error {
	path, err := s.absPath(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("storage/local: Delete %q: %w", key, err)
	}
	return nil
}

// Exists reports whether the file at key exists.
func (s *Store) Exists(_ context.Context, key string) (bool, error) {
	path, err := s.absPath(key)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("storage/local: Exists %q: %w", key, err)
}

// Stat returns metadata for the file at key.
func (s *Store) Stat(_ context.Context, key string) (storage.ObjectInfo, error) {
	path, err := s.absPath(key)
	if err != nil {
		return storage.ObjectInfo{}, err
	}
	fi, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return storage.ObjectInfo{}, fmt.Errorf("storage/local: Stat %q: not found", key)
		}
		return storage.ObjectInfo{}, fmt.Errorf("storage/local: Stat %q: %w", key, err)
	}
	return storage.ObjectInfo{
		Key:          key,
		Size:         fi.Size(),
		LastModified: fi.ModTime(),
	}, nil
}

// SignedURL returns a URL for the object. If BaseURL is set it returns an
// HTTP URL; otherwise it returns a file:// URL. The ttl parameter is ignored.
func (s *Store) SignedURL(_ context.Context, key string, _ time.Duration) (string, error) {
	if _, err := s.absPath(key); err != nil {
		return "", err
	}
	if s.baseURL != "" {
		return s.baseURL + "/" + url.PathEscape(key), nil
	}
	path, _ := s.absPath(key)
	return "file://" + filepath.ToSlash(path), nil
}

// SignedPutURL returns the same URL as SignedURL. The ttl and opts are ignored.
func (s *Store) SignedPutURL(ctx context.Context, key string, ttl time.Duration, _ storage.PutOptions) (string, error) {
	return s.SignedURL(ctx, key, ttl)
}

// ─── ListableStorage ──────────────────────────────────────────────────────────

// List returns objects whose keys start with prefix.
// nextToken is the last key seen on the previous page (exclusive lower bound).
// maxKeys ≤ 0 defaults to 1000.
func (s *Store) List(_ context.Context, prefix, nextToken string, maxKeys int) (storage.ListResult, error) {
	if maxKeys <= 0 {
		maxKeys = 1000
	}

	var objects []storage.ObjectInfo
	truncated := false

	err := filepath.WalkDir(s.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Skip the hidden .multipart staging directory.
			if d.Name() == ".multipart" {
				return filepath.SkipDir
			}
			return nil
		}
		// Convert absolute path back to storage key.
		rel, err := filepath.Rel(s.root, path)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)

		if prefix != "" && !strings.HasPrefix(key, prefix) {
			return nil
		}
		if nextToken != "" && key <= nextToken {
			return nil
		}
		if len(objects) >= maxKeys {
			// We have a full page; signal truncation without adding this key.
			truncated = true
			return io.EOF
		}

		fi, err := d.Info()
		if err != nil {
			return err
		}
		objects = append(objects, storage.ObjectInfo{
			Key:          key,
			Size:         fi.Size(),
			LastModified: fi.ModTime(),
		})
		return nil
	})

	if err == io.EOF {
		err = nil
	}
	if err != nil {
		return storage.ListResult{}, fmt.Errorf("storage/local: List: %w", err)
	}

	result := storage.ListResult{
		Objects:     objects,
		IsTruncated: truncated,
	}
	// NextToken is the last key we returned; the next call passes it as nextToken
	// and we skip keys <= nextToken, so the page continues from the next key.
	if truncated && len(objects) > 0 {
		result.NextToken = objects[len(objects)-1].Key
	}
	return result, nil
}

// ─── CopyableStorage ─────────────────────────────────────────────────────────

// Copy duplicates the file at srcKey to dstKey.
func (s *Store) Copy(ctx context.Context, srcKey, dstKey string) error {
	rc, err := s.Get(ctx, srcKey)
	if err != nil {
		return fmt.Errorf("storage/local: Copy src %q: %w", srcKey, err)
	}
	defer rc.Close()
	if err := s.Put(ctx, dstKey, rc, storage.PutOptions{}); err != nil {
		return fmt.Errorf("storage/local: Copy dst %q: %w", dstKey, err)
	}
	return nil
}

// ─── MultipartStorage ────────────────────────────────────────────────────────
//
// The local backend simulates multipart upload by writing parts to temporary
// files under <root>/.multipart/<uploadID>/part-<N>.

// generateUploadID returns a cryptographically random 16-byte hex upload ID.
// Using crypto/rand instead of time.Now().UnixNano() prevents attackers from
// guessing upload IDs and interfering with other users' multipart uploads.
func generateUploadID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// CreateMultipartUpload creates a staging directory and returns an upload ID.
func (s *Store) CreateMultipartUpload(_ context.Context, key string, _ storage.PutOptions) (string, error) {
	if _, err := s.absPath(key); err != nil {
		return "", err
	}
	uploadID := generateUploadID()
	stageDir := filepath.Join(s.root, ".multipart", uploadID)
	if err := os.MkdirAll(stageDir, 0o755); err != nil {
		return "", fmt.Errorf("storage/local: CreateMultipartUpload %q: %w", key, err)
	}
	// Store the target key in a manifest file.
	if err := os.WriteFile(filepath.Join(stageDir, "_key"), []byte(key), 0o644); err != nil {
		return "", fmt.Errorf("storage/local: CreateMultipartUpload write key: %w", err)
	}
	return uploadID, nil
}

// UploadPart writes one part to the staging directory.
func (s *Store) UploadPart(_ context.Context, _ string, uploadID string, partNum int, r io.Reader, _ int64) (string, error) {
	stageDir := filepath.Join(s.root, ".multipart", uploadID)
	partPath := filepath.Join(stageDir, fmt.Sprintf("part-%05d", partNum))
	f, err := os.Create(partPath)
	if err != nil {
		return "", fmt.Errorf("storage/local: UploadPart %d: %w", partNum, err)
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		return "", fmt.Errorf("storage/local: UploadPart %d write: %w", partNum, err)
	}
	// ETag is the part file path (sufficient for local testing).
	return partPath, nil
}

// CompleteMultipartUpload concatenates parts in order and writes the final object.
func (s *Store) CompleteMultipartUpload(ctx context.Context, key, uploadID string, parts []storage.CompletedPart) error {
	stageDir := filepath.Join(s.root, ".multipart", uploadID)

	destPath, err := s.absPath(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("storage/local: CompleteMultipartUpload mkdir: %w", err)
	}

	dest, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("storage/local: CompleteMultipartUpload create: %w", err)
	}
	defer dest.Close()

	for _, p := range parts {
		partPath := filepath.Join(stageDir, fmt.Sprintf("part-%05d", p.PartNumber))
		f, err := os.Open(partPath)
		if err != nil {
			return fmt.Errorf("storage/local: CompleteMultipartUpload part %d: %w", p.PartNumber, err)
		}
		_, copyErr := io.Copy(dest, f)
		f.Close()
		if copyErr != nil {
			return fmt.Errorf("storage/local: CompleteMultipartUpload copy part %d: %w", p.PartNumber, copyErr)
		}
	}

	// Clean up staging directory.
	_ = os.RemoveAll(stageDir)
	return nil
}

// AbortMultipartUpload removes the staging directory.
func (s *Store) AbortMultipartUpload(_ context.Context, _ string, uploadID string) error {
	stageDir := filepath.Join(s.root, ".multipart", uploadID)
	if err := os.RemoveAll(stageDir); err != nil {
		return fmt.Errorf("storage/local: AbortMultipartUpload %q: %w", uploadID, err)
	}
	return nil
}

// Compile-time interface checks.
var (
	_ storage.Storage          = (*Store)(nil)
	_ storage.ListableStorage  = (*Store)(nil)
	_ storage.CopyableStorage  = (*Store)(nil)
	_ storage.MultipartStorage = (*Store)(nil)
)
