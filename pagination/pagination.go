// Package pagination provides offset- and cursor-based pagination helpers for
// Astra applications.
//
// # Offset pagination
//
//	req := pagination.FromRequest(c)  // ?page=2&size=20
//	var users []User
//	db.Scopes(orm.GORMScope(req)).Find(&users)
//	total := int64(0)
//	db.Model(&User{}).Count(&total)
//	return c.JSON(200, pagination.NewPage(users, total, req))
//
// # Cursor pagination
//
//	req := pagination.FromRequest(c)   // ?cursor=<opaque>&size=20
//	var posts []Post
//	db.Where("id > ?", req.DecodeCursor()).Limit(req.Size + 1).Find(&posts)
//	return c.JSON(200, pagination.NewCursorPage(posts, req))
package pagination

import (
	"encoding/base64"
	"strconv"

	"github.com/astra-go/astra"
)

const (
	defaultPage = 1
	defaultSize = 20
	maxSize     = 100
)

// options configures FromRequest behaviour.
type options struct {
	defaultSize int
	maxSize     int
}

// Option configures FromRequest.
type Option func(*options)

// WithDefaultSize overrides the default page size (default: 20).
func WithDefaultSize(n int) Option {
	return func(o *options) { o.defaultSize = n }
}

// WithMaxSize overrides the maximum allowed page size (default: 100).
func WithMaxSize(n int) Option {
	return func(o *options) { o.maxSize = n }
}

// Request holds pagination parameters parsed from the HTTP request.
type Request struct {
	// Page is the 1-based page number (offset mode). Default: 1.
	Page int

	// Size is the page size (both modes). Clamped to [1, MaxSize].
	Size int

	// Cursor is the opaque continuation token (cursor mode).
	// Empty string means "start from the beginning".
	Cursor string

	maxSize int
}

// FromRequest parses page / size / cursor from the query string of c.
//
//	?page=2&size=20          → offset mode
//	?cursor=<token>&size=20  → cursor mode
func FromRequest(c *astra.Ctx, opts ...Option) Request {
	o := &options{defaultSize: defaultSize, maxSize: maxSize}
	for _, opt := range opts {
		opt(o)
	}

	page := parseIntParam(c.Query("page"), defaultPage)
	if page < 1 {
		page = 1
	}

	size := parseIntParam(c.Query("size"), o.defaultSize)
	if size < 1 {
		size = 1
	}
	if size > o.maxSize {
		size = o.maxSize
	}

	return Request{
		Page:    page,
		Size:    size,
		Cursor:  c.Query("cursor"),
		maxSize: o.maxSize,
	}
}

// Offset returns the SQL OFFSET for this request (offset mode only).
func (r Request) Offset() int {
	if r.Page < 1 {
		return 0
	}
	return (r.Page - 1) * r.Size
}

// DecodeCursor base64url-decodes the cursor and returns the raw string.
// Returns empty string when cursor is absent or malformed.
func (r Request) DecodeCursor() string {
	if r.Cursor == "" {
		return ""
	}
	b, err := base64.RawURLEncoding.DecodeString(r.Cursor)
	if err != nil {
		return ""
	}
	return string(b)
}

// ─── Response types ───────────────────────────────────────────────────────────

// Page is a typed offset-paginated response.
type Page[T any] struct {
	Items []T   `json:"items"`
	Total int64 `json:"total"`
	Page  int   `json:"page"`
	Size  int   `json:"size"`
	Pages int   `json:"pages"`
}

// NewPage constructs a Page response.
func NewPage[T any](items []T, total int64, req Request) Page[T] {
	pages := 0
	if req.Size > 0 {
		pages = int((total + int64(req.Size) - 1) / int64(req.Size))
	}
	if items == nil {
		items = []T{}
	}
	return Page[T]{
		Items: items,
		Total: total,
		Page:  req.Page,
		Size:  req.Size,
		Pages: pages,
	}
}

// CursorPage is a typed cursor-paginated response.
// Set NextCursor in the response so the client can fetch the next page.
type CursorPage[T any] struct {
	Items      []T    `json:"items"`
	HasMore    bool   `json:"has_more"`
	NextCursor string `json:"next_cursor,omitempty"`
}

// NewCursorPage constructs a CursorPage by checking whether a "next page"
// exists. Pass items fetched with Limit(req.Size + 1); this function trims
// the extra element and sets HasMore accordingly.
//
// encodeCursor converts the last included item into a continuation token
// (base64url-encoded string). Pass nil to skip cursor encoding.
func NewCursorPage[T any](items []T, req Request, encodeCursor func(T) string) CursorPage[T] {
	hasMore := len(items) > req.Size
	if hasMore {
		items = items[:req.Size]
	}
	if items == nil {
		items = []T{}
	}
	p := CursorPage[T]{Items: items, HasMore: hasMore}
	if hasMore && encodeCursor != nil && len(items) > 0 {
		raw := encodeCursor(items[len(items)-1])
		p.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(raw))
	}
	return p
}

// EncodeCursor base64url-encodes a raw cursor string for use in a response.
func EncodeCursor(raw string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func parseIntParam(s string, defaultVal int) int {
	if s == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return n
}
