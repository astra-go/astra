package orm

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"
)

// ─── N+1 Query Detector ───────────────────────────────────────────────────────
//
// N+1 queries occur when a query to fetch a list of N records is followed by
// N additional queries to fetch related data (e.g. lazy-loaded associations).
//
// This plugin detects N+1 patterns by tracking:
//   - Queries that fetch a collection of records (SELECT ... FROM table LIMIT > 1)
//   - Subsequent queries on the same table within the same request context
//
// Usage:
//
//	db, _ := gorm.Open(postgres.Open(dsn), &gorm.Config{
//	    Plugins: map[string]interface{}{
//	        "n_plus_one_detector": orm.NewNPlusOneDetector(
//	            orm.WithNPlusOneLogLevel(slog.LevelWarn),
//	        ),
//	    },
//	})

// NPlusOneDetector is a GORM plugin that detects N+1 query patterns.
// It is safe for concurrent use.
type NPlusOneDetector struct {
	opts NPlusOneOptions

	// Per-request tracking.
	mu      sync.RWMutex // map access
	queries map[string]*queryRecord
}

// queryRecord tracks a query for N+1 detection.
type queryRecord struct {
	table       string
	query       string
	count       int
	lastQueryAt time.Time
	firstQuery  time.Time
}

// NPlusOneOptions configures the detector.
type NPlusOneOptions struct {
	// LogLevel sets the slog level for N+1 warnings.
	// Default: Warn.
	LogLevel slog.Level

	// Threshold is the number of repeated queries that trigger a warning.
	// Default: 3.
	Threshold int

	// MaxCache is the maximum number of query patterns to cache.
	// Default: 1000.
	MaxCache int

	// Enabled enables/disables detection.
	// Default: true.
	Enabled bool
}

// DefaultNPlusOneOptions returns the default options.
func DefaultNPlusOneOptions() NPlusOneOptions {
	return NPlusOneOptions{
		LogLevel:  slog.LevelWarn,
		Threshold: 3,
		MaxCache:  1000,
		Enabled:   true,
	}
}

// NPlusOneOption configures NPlusOneDetector.
type NPlusOneOption func(*NPlusOneOptions)

// WithNPlusOneLogLevel sets the log level.
func WithNPlusOneLogLevel(level slog.Level) NPlusOneOption {
	return func(o *NPlusOneOptions) { o.LogLevel = level }
}

// WithNPlusOneThreshold sets the warning threshold.
func WithNPlusOneThreshold(n int) NPlusOneOption {
	return func(o *NPlusOneOptions) { o.Threshold = n }
}

// WithNPlusOneMaxCache sets the maximum cache size.
func WithNPlusOneMaxCache(n int) NPlusOneOption {
	return func(o *NPlusOneOptions) { o.MaxCache = n }
}

// NewNPlusOneDetector creates a new N+1 query detector.
func NewNPlusOneDetector(opts ...NPlusOneOption) *NPlusOneDetector {
	o := DefaultNPlusOneOptions()
	for _, opt := range opts {
		opt(&o)
	}
	return &NPlusOneDetector{
		opts:    o,
		queries: make(map[string]*queryRecord),
	}
}

// Name returns the plugin name.
func (d *NPlusOneDetector) Name() string {
	return "n_plus_one_detector"
}

// Initialize sets up the plugin.
func (d *NPlusOneDetector) Initialize(db *gorm.DB) error {
	return db.Callback().Query().Before("gorm:query").Register("n_plus_one:before_query", d.beforeQuery)
}

// beforeQuery is called before each query.
func (d *NPlusOneDetector) beforeQuery(db *gorm.DB) {
	if !d.opts.Enabled {
		return
	}

	ctx := db.Statement.Context
	if ctx == nil {
		return
	}

	// Extract query info.
	query := db.Statement.SQL.String()
	table := d.extractTable(query)
	if table == "" {
		return
	}

	// Check for N+1 pattern.
	key := d.buildKey(ctx, table)
	d.mu.Lock()
	record, exists := d.queries[key]
	if exists {
		record.count++
		record.lastQueryAt = time.Now()
		d.mu.Unlock()

		if record.count >= d.opts.Threshold {
			slog.Log(ctx, d.opts.LogLevel, "N+1 query detected",
				"table", table,
				"count", record.count,
				"query", truncateQuery(query),
				"elapsed", time.Since(record.firstQuery),
			)
		}
	} else {
		d.queries[key] = &queryRecord{
			table:       table,
			query:       query,
			count:       1,
			lastQueryAt: time.Now(),
			firstQuery:  time.Now(),
		}
		d.mu.Unlock()
	}
}

// buildKey creates a unique key for the query pattern.
func (d *NPlusOneDetector) buildKey(ctx context.Context, table string) string {
	return fmt.Sprintf("%s:%s", table, d.getRequestID(ctx))
}

// tablePatterns holds pre-compiled regex for extractTable.
// Compiled once at init to avoid per-call MustCompile overhead.
var tablePatterns []*regexp.Regexp

func init() {
	tablePatterns = []*regexp.Regexp{
		regexp.MustCompile(`\bFROM\s+(\w+)`),
		regexp.MustCompile(`\bJOIN\s+(\w+)`),
		regexp.MustCompile(`\bUPDATE\s+(\w+)`),
		regexp.MustCompile(`\bINSERT\s+INTO\s+(\w+)`),
		regexp.MustCompile(`\bDELETE\s+FROM\s+(\w+)`),
	}
}

// getRequestID extracts or generates a request ID from context.
func (d *NPlusOneDetector) getRequestID(ctx context.Context) string {
	// Try to extract from common tracing headers.
	if id, ok := ctx.Value("request-id").(string); ok {
		return id
	}
	if id, ok := ctx.Value("x-request-id").(string); ok {
		return id
	}

	// Fall back to generating a unique ID per request.
	// This is a simplified approach; in production, use a proper trace ID.
	return fmt.Sprintf("%p", ctx)
}

// extractTable extracts the table name from a SQL query.
func (d *NPlusOneDetector) extractTable(query string) string {
	query = strings.ToUpper(query)
	query = strings.TrimSpace(query)
	for _, p := range tablePatterns {
		if matches := p.FindStringSubmatch(query); len(matches) > 1 {
			return matches[1]
		}
	}
	return ""
}

// truncateQuery truncates a query for logging.
func truncateQuery(q string) string {
	if len(q) > 200 {
		return q[:200] + "..."
	}
	return q
}

// Stats returns detection statistics.
func (d *NPlusOneDetector) Stats() NPlusOneStats {
	d.mu.Lock()
	defer d.mu.Unlock()

	var total, highCount int
	for _, r := range d.queries {
		total++
		if r.count >= d.opts.Threshold {
			highCount++
		}
	}

	return NPlusOneStats{
		CachedPatterns: len(d.queries),
		NPlusOneDetected: highCount,
	}
}

// NPlusOneStats contains detection statistics.
type NPlusOneStats struct {
	CachedPatterns   int
	NPlusOneDetected int
}

// Reset clears all tracked queries.
func (d *NPlusOneDetector) Reset() {
	d.mu.Lock()
	d.queries = make(map[string]*queryRecord)
	d.mu.Unlock()
}

// ─── Query Counter ────────────────────────────────────────────────────────────
//
// QueryCounter is a simpler alternative that just counts queries per request.

type QueryCounter struct {
	mu       sync.RWMutex
	counters map[string]*atomic.Int64
}

// NewQueryCounter creates a new query counter.
func NewQueryCounter() *QueryCounter {
	return &QueryCounter{
		counters: make(map[string]*atomic.Int64),
	}
}

// Count returns the number of queries for a request.
func (c *QueryCounter) Count(ctx context.Context) int64 {
	key := fmt.Sprintf("%p", ctx)
	c.mu.RLock()
	defer c.mu.RUnlock()
	if counter, ok := c.counters[key]; ok {
		return counter.Load()
	}
	return 0
}

func (c *QueryCounter) beforeQuery(db *gorm.DB) {
	key := fmt.Sprintf("%p", db.Statement.Context)
	c.mu.Lock()
	if _, ok := c.counters[key]; !ok {
		c.counters[key] = &atomic.Int64{}
	}
	c.counters[key].Add(1)
	c.mu.Unlock()
}