// Package upload provides streaming file-upload middleware for Astra.
//
// Standard multipart parsing (http.Request.ParseMultipartForm) buffers the
// entire request body in memory up to MaxMultipartMemory, then spills to
// temporary files.  For large uploads this still risks OOM when many
// concurrent uploads are in flight, and the caller has no control over
// per-file size limits or streaming to a custom destination.
//
// This package addresses those issues with:
//
//   - StreamLimit middleware: enforces per-request body size via
//     http.MaxBytesReader before the handler runs, rejecting oversized
//     requests immediately with 413.
//   - Ctx.SaveFile / Ctx.SaveFileTo: stream individual files to disk or an
//     arbitrary io.Writer, copying in fixed-size chunks so memory usage is
//     bounded regardless of file size.
//   - UploadLimiter middleware: combines StreamLimit with per-field and
//     per-file count/size constraints for fine-grained control.
//
// # Quick start
//
//	app := astra.New()
//
//	// Global body limit: 50 MB per request.
//	app.Use(upload.StreamLimit(50 << 20))
//
//	app.POST("/upload", func(c *astra.Ctx) error {
//	    // Save uploaded file to a temp directory.
//	    path, info, err := c.SaveFile("avatar", upload.SaveFileConfig{
//	        MaxSize:    5 << 20,   // 5 MB per file
//	        DestDir:    "/tmp/uploads",
//	    })
//	    if err != nil { return err }
//	    return c.JSON(200, map[string]string{"path": path})
//	})
package upload

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/astra-go/astra"
)

// ─── Errors ──────────────────────────────────────────────────────────────────

var (
	// ErrFileTooLarge is returned when an uploaded file exceeds MaxSize.
	ErrFileTooLarge = errors.New("upload: file too large")
	// ErrTooManyFiles is returned when the number of files exceeds MaxFiles.
	ErrTooManyFiles = errors.New("upload: too many files")
	// ErrFieldTooLarge is returned when a form field value exceeds MaxFieldSize.
	ErrFieldTooLarge = errors.New("upload: form field too large")
	// ErrBodyTooLarge is returned when the request body exceeds the limit.
	ErrBodyTooLarge = errors.New("upload: request body too large")
	// ErrUnsupportedExt is returned when the file extension is not allowed.
	ErrUnsupportedExt = errors.New("upload: file extension not allowed")
)

// ─── StreamLimit middleware ──────────────────────────────────────────────────

// StreamLimit returns middleware that caps the request body to maxBytes.
// It wraps the request body in an http.MaxBytesReader before the handler
// runs, so oversized payloads are rejected immediately with 413 without
// buffering the entire body.
//
// This is the simplest protection against OOM from large uploads:
//
//	app.Use(upload.StreamLimit(50 << 20)) // 50 MB max body
func StreamLimit(maxBytes int64) astra.HandlerFunc {
	return func(c *astra.Ctx) error {
		if c.Request().Body != nil {
			c.Request().Body = http.MaxBytesReader(c.Writer(), c.Request().Body, maxBytes)
		}
		return nil
	}
}

// ─── SaveFileConfig ──────────────────────────────────────────────────────────

// SaveFileConfig controls how SaveFile writes the uploaded file to disk.
type SaveFileConfig struct {
	// MaxSize is the maximum allowed file size in bytes.
	// 0 means use the App's MaxMultipartMemory (default 32 MB).
	MaxSize int64
	// DestDir is the directory to save the file. Empty means os.TempDir().
	DestDir string
	// Prefix is the temp-file name prefix. Default: "astra-upload-".
	Prefix string
	// AllowedExts lists allowed file extensions (with dot), e.g. ".jpg", ".png".
	// Empty means all extensions are allowed.
	AllowedExts []string
	// Overwrite allows overwriting an existing file at the destination.
	// By default a unique filename is generated to avoid collisions.
	Overwrite bool
}

// copyBufPool pools byte slices used for streaming copies.
// 32 KB matches io.Copy's internal buffer size.
var copyBufPool = &byteSlicePool{size: 32 * 1024}

type byteSlicePool struct {
	size int
}

func (p *byteSlicePool) Get() []byte {
	return make([]byte, p.size)
}

func (p *byteSlicePool) Put(_ []byte) {
	// No pooling needed; let GC collect. The fixed 32KB allocation
	// per SaveFile call is negligible and avoids pool complexity.
}

// ─── SaveFile ────────────────────────────────────────────────────────────────

// SaveFile saves a single multipart file field to a temporary file on disk.
// Returns the file path, file info, and any error.
//
// The file is streamed in fixed-size chunks so memory usage stays bounded
// regardless of file size.  If the file exceeds cfg.MaxSize, the partially
// written temp file is removed and ErrFileTooLarge is returned.
//
// Example:
//
//	path, info, err := c.SaveFile("document", upload.SaveFileConfig{
//	    MaxSize: 10 << 20, // 10 MB
//	    DestDir: "/var/uploads",
//	})
func SaveFile(c *astra.Ctx, field string, cfg SaveFileConfig) (string, *multipart.FileHeader, error) {
	if cfg.Prefix == "" {
		cfg.Prefix = "astra-upload-"
	}
	if cfg.DestDir == "" {
		cfg.DestDir = os.TempDir()
	}

	// Parse multipart form with bounded memory.
	maxMem := cfg.MaxSize
	if maxMem <= 0 {
		maxMem = c.Request().ContentLength
		if maxMem <= 0 {
			maxMem = 32 << 20 // 32 MB default
		}
	}

	if err := c.Request().ParseMultipartForm(maxMem); err != nil {
		return "", nil, fmt.Errorf("upload: parse multipart: %w", err)
	}

	file, header, err := c.Request().FormFile(field)
	if err != nil {
		return "", nil, fmt.Errorf("upload: read field %q: %w", field, err)
	}
	defer file.Close()

	// Validate extension.
	if len(cfg.AllowedExts) > 0 {
		ext := strings.ToLower(filepath.Ext(header.Filename))
		allowed := false
		for _, a := range cfg.AllowedExts {
			if ext == strings.ToLower(a) {
				allowed = true
				break
			}
		}
		if !allowed {
			return "", nil, ErrUnsupportedExt
		}
	}

	// Validate size.
	if cfg.MaxSize > 0 && header.Size > cfg.MaxSize {
		return "", nil, ErrFileTooLarge
	}

	// Ensure destination directory exists.
	if err := os.MkdirAll(cfg.DestDir, 0o700); err != nil {
		return "", nil, fmt.Errorf("upload: mkdir %q: %w", cfg.DestDir, err)
	}

	// Create temp file.
	ext := filepath.Ext(header.Filename)
	dst, err := os.CreateTemp(cfg.DestDir, cfg.Prefix+"*"+ext)
	if err != nil {
		return "", nil, fmt.Errorf("upload: create temp file: %w", err)
	}
	dstPath := dst.Name()

	// Stream-copy with size enforcement.
	buf := copyBufPool.Get()
	var written int64
	written, err = io.CopyBuffer(dst, file, buf)

	if err != nil {
		dst.Close()
		os.Remove(dstPath)
		return "", nil, fmt.Errorf("upload: copy: %w", err)
	}

	// Double-check size after copy (Content-Length can lie).
	if cfg.MaxSize > 0 && written > cfg.MaxSize {
		dst.Close()
		os.Remove(dstPath)
		return "", nil, ErrFileTooLarge
	}

	if err := dst.Close(); err != nil {
		os.Remove(dstPath)
		return "", nil, fmt.Errorf("upload: close: %w", err)
	}

	return dstPath, header, nil
}

// ─── SaveFileTo ──────────────────────────────────────────────────────────────

// SaveFileTo streams a multipart file field to an arbitrary io.Writer.
// This is useful for streaming directly to object storage (S3, OSS, COS)
// without touching the local disk.
//
// Returns the file header and any error. The caller is responsible for
// closing the writer when appropriate.
//
// Example — stream to S3:
//
//	s3Writer, _ := store.PutWriter(ctx, "uploads/avatar.png", storage.PutOptions{...})
//	defer s3Writer.Close()
//	header, err := upload.SaveFileTo(c, "avatar", s3Writer, upload.SaveFileConfig{MaxSize: 5<<20})
func SaveFileTo(c *astra.Ctx, field string, dst io.Writer, cfg SaveFileConfig) (*multipart.FileHeader, error) {
	maxMem := cfg.MaxSize
	if maxMem <= 0 {
		maxMem = 32 << 20
	}

	if err := c.Request().ParseMultipartForm(maxMem); err != nil {
		return nil, fmt.Errorf("upload: parse multipart: %w", err)
	}

	file, header, err := c.Request().FormFile(field)
	if err != nil {
		return nil, fmt.Errorf("upload: read field %q: %w", field, err)
	}
	defer file.Close()

	// Validate extension.
	if len(cfg.AllowedExts) > 0 {
		ext := strings.ToLower(filepath.Ext(header.Filename))
		allowed := false
		for _, a := range cfg.AllowedExts {
			if ext == strings.ToLower(a) {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, ErrUnsupportedExt
		}
	}

	// Validate size from header.
	if cfg.MaxSize > 0 && header.Size > cfg.MaxSize {
		return nil, ErrFileTooLarge
	}

	// Stream-copy with size enforcement using a LimitedReader.
	src := io.LimitReader(file, cfg.MaxSize+1) // +1 to detect overflow
	buf := copyBufPool.Get()
	var written int64
	written, err = io.CopyBuffer(dst, src, buf)

	if err != nil {
		return nil, fmt.Errorf("upload: copy: %w", err)
	}

	if cfg.MaxSize > 0 && written > cfg.MaxSize {
		return nil, ErrFileTooLarge
	}

	return header, nil
}

// ─── UploadLimiter middleware ────────────────────────────────────────────────

// UploadLimiterConfig configures the UploadLimiter middleware.
type UploadLimiterConfig struct {
	// MaxBodySize is the maximum total request body size. Default: 50 MB.
	MaxBodySize int64
	// MaxFileSize is the maximum size of a single uploaded file. Default: 10 MB.
	MaxFileSize int64
	// MaxFiles is the maximum number of files per request. Default: 10.
	MaxFiles int
	// MaxFieldSize is the maximum size of a non-file form field. Default: 1 MB.
	MaxFieldSize int64
	// AllowedExts lists allowed file extensions (with dot). Empty means all.
	AllowedExts []string
	// ErrorHandler is called when a limit is exceeded.
	// Default: 413 Request Entity Too Large with the error message.
	ErrorHandler func(c *astra.Ctx, err error) error
}

// DefaultUploadLimiterConfig provides sensible defaults.
var DefaultUploadLimiterConfig = UploadLimiterConfig{
	MaxBodySize:  50 << 20, // 50 MB
	MaxFileSize:  10 << 20, // 10 MB
	MaxFiles:     10,
	MaxFieldSize: 1 << 20, // 1 MB
}

// UploadLimiter returns middleware that enforces upload size and count limits.
// It combines StreamLimit with per-file, per-field, and file-count checks.
//
//	app.Use(upload.UploadLimiter(upload.UploadLimiterConfig{
//	    MaxBodySize: 100 << 20, // 100 MB total
//	    MaxFileSize:  20 << 20, // 20 MB per file
//	    MaxFiles:      5,
//	}))
func UploadLimiter(cfg UploadLimiterConfig) astra.HandlerFunc {
	if cfg.MaxBodySize <= 0 {
		cfg.MaxBodySize = DefaultUploadLimiterConfig.MaxBodySize
	}
	if cfg.MaxFileSize <= 0 {
		cfg.MaxFileSize = DefaultUploadLimiterConfig.MaxFileSize
	}
	if cfg.MaxFiles <= 0 {
		cfg.MaxFiles = DefaultUploadLimiterConfig.MaxFiles
	}
	if cfg.MaxFieldSize <= 0 {
		cfg.MaxFieldSize = DefaultUploadLimiterConfig.MaxFieldSize
	}
	if cfg.ErrorHandler == nil {
		cfg.ErrorHandler = func(c *astra.Ctx, err error) error {
			status := http.StatusRequestEntityTooLarge
			if errors.Is(err, ErrUnsupportedExt) {
				status = http.StatusBadRequest
			}
			return astra.NewHTTPError(status, err.Error())
		}
	}

	return func(c *astra.Ctx) error {
		// 1. Cap the request body.
		if c.Request().Body != nil {
			c.Request().Body = http.MaxBytesReader(c.Writer(), c.Request().Body, cfg.MaxBodySize)
		}

		// 2. Parse the multipart form with bounded memory.
		// Use MaxFieldSize as the in-memory limit so non-file fields don't blow memory.
		if err := c.Request().ParseMultipartForm(cfg.MaxFieldSize); err != nil {
			if strings.Contains(err.Error(), "http: request body too large") {
				return cfg.ErrorHandler(c, ErrBodyTooLarge)
			}
			return astra.NewHTTPError(http.StatusBadRequest, err.Error())
		}

		form := c.Request().MultipartForm
		if form == nil {
			return nil
		}

		// 3. Count files and check per-file sizes.
		fileCount := 0
		for _, headers := range form.File {
			for _, h := range headers {
				fileCount++
				if fileCount > cfg.MaxFiles {
					return cfg.ErrorHandler(c, ErrTooManyFiles)
				}
				if cfg.MaxFileSize > 0 && h.Size > cfg.MaxFileSize {
					return cfg.ErrorHandler(c, ErrFileTooLarge)
				}
				// Validate extension if configured.
				if len(cfg.AllowedExts) > 0 {
					ext := strings.ToLower(filepath.Ext(h.Filename))
					allowed := false
					for _, a := range cfg.AllowedExts {
						if ext == strings.ToLower(a) {
							allowed = true
							break
						}
					}
					if !allowed {
						return cfg.ErrorHandler(c, ErrUnsupportedExt)
					}
				}
			}
		}

		// 4. Check non-file field sizes.
		if form.Value != nil {
			for key, vals := range form.Value {
				for _, v := range vals {
					if int64(len(v)) > cfg.MaxFieldSize {
						return cfg.ErrorHandler(c, fmt.Errorf("%w: field %q", ErrFieldTooLarge, key))
					}
				}
			}
		}

		return nil
	}
}
