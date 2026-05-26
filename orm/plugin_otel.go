package orm

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

// OTelPlugin returns a gorm.Plugin that creates an OTel span for every database
// operation (Create, Query, Update, Delete, Row, Raw).
//
// Install once after opening the DB:
//
//	db.Use(orm.OTelPlugin(orm.WithOTelDBSystem("postgresql")))
//
// The plugin reads the active span from db.Statement.Context, which is set
// automatically when handlers call orm.DB(c) or orm.FromCtx(ctx, db).
// No per-query changes are required in application code.
func OTelPlugin(opts ...OTelOption) gorm.Plugin {
	cfg := &otelConfig{
		tracerName: "astra/orm",
		dbSystem:   "unknown",
	}
	for _, o := range opts {
		o(cfg)
	}
	if cfg.tracerProvider != nil {
		cfg.tracer = cfg.tracerProvider.Tracer(cfg.tracerName)
	} else {
		cfg.tracer = otel.Tracer(cfg.tracerName)
	}
	return &otelPlugin{cfg: cfg}
}

// OTelOption configures the OTel GORM plugin.
type OTelOption func(*otelConfig)

type otelConfig struct {
	tracerProvider trace.TracerProvider
	tracer         trace.Tracer
	tracerName     string
	dbSystem       string
	recordSQL      bool
}

// WithOTelTracerProvider overrides the global TracerProvider for this plugin.
func WithOTelTracerProvider(tp trace.TracerProvider) OTelOption {
	return func(c *otelConfig) { c.tracerProvider = tp }
}

// WithOTelTracerName sets the OTel instrumentation library name. Default: "astra/orm".
func WithOTelTracerName(name string) OTelOption {
	return func(c *otelConfig) { c.tracerName = name }
}

// WithOTelDBSystem sets the db.system span attribute (e.g. "mysql", "postgresql", "sqlite").
func WithOTelDBSystem(system string) OTelOption {
	return func(c *otelConfig) { c.dbSystem = system }
}

// WithOTelRecordSQL enables capturing SQL statement text in the db.statement span
// attribute. Disabled by default to prevent accidental PII / credential leakage
// in parameterised queries.
func WithOTelRecordSQL(enable bool) OTelOption {
	return func(c *otelConfig) { c.recordSQL = enable }
}

// ─── Plugin implementation ────────────────────────────────────────────────────

type otelPlugin struct{ cfg *otelConfig }

func (p *otelPlugin) Name() string { return "astra:otel" }

// Initialize registers before/after hooks on every GORM callback chain.
// Each chain (Create/Query/Update/Delete/Row/Raw) is independent.
func (p *otelPlugin) Initialize(db *gorm.DB) error {
	cb := db.Callback()

	// Create chain
	if err := cb.Create().Before("gorm:before_create").Register("otel:before_create", p.beforeCallback("INSERT")); err != nil {
		return err
	}
	if err := cb.Create().After("gorm:after_create").Register("otel:after_create", p.afterCallback); err != nil {
		return err
	}

	// Query chain
	if err := cb.Query().Before("gorm:query").Register("otel:before_query", p.beforeCallback("SELECT")); err != nil {
		return err
	}
	if err := cb.Query().After("gorm:after_query").Register("otel:after_query", p.afterCallback); err != nil {
		return err
	}

	// Update chain
	if err := cb.Update().Before("gorm:before_update").Register("otel:before_update", p.beforeCallback("UPDATE")); err != nil {
		return err
	}
	if err := cb.Update().After("gorm:after_update").Register("otel:after_update", p.afterCallback); err != nil {
		return err
	}

	// Delete chain
	if err := cb.Delete().Before("gorm:before_delete").Register("otel:before_delete", p.beforeCallback("DELETE")); err != nil {
		return err
	}
	if err := cb.Delete().After("gorm:after_delete").Register("otel:after_delete", p.afterCallback); err != nil {
		return err
	}

	// Row chain
	if err := cb.Row().Before("gorm:row").Register("otel:before_row", p.beforeCallback("ROW")); err != nil {
		return err
	}
	if err := cb.Row().After("gorm:after_row").Register("otel:after_row", p.afterCallback); err != nil {
		return err
	}

	// Raw chain
	if err := cb.Raw().Before("gorm:raw").Register("otel:before_raw", p.beforeCallback("RAW")); err != nil {
		return err
	}
	if err := cb.Raw().After("gorm:after_raw").Register("otel:after_raw", p.afterCallback); err != nil {
		return err
	}

	return nil
}

func (p *otelPlugin) beforeCallback(op string) func(*gorm.DB) {
	return func(db *gorm.DB) {
		if db.Statement == nil {
			return
		}
		ctx := db.Statement.Context
		if ctx == nil {
			ctx = context.Background()
		}

		table := db.Statement.Table
		spanName := op
		if table != "" {
			spanName = op + " " + table
		}

		attrs := []attribute.KeyValue{
			semconv.DBSystemKey.String(p.cfg.dbSystem),
			attribute.String("db.operation", op),
		}
		if table != "" {
			attrs = append(attrs, attribute.String("db.sql.table", table))
		}

		// Start a child span. tracer.Start returns the new context containing
		// the span as the active span — afterCallback retrieves it via SpanFromContext.
		ctx, _ = p.cfg.tracer.Start(ctx, spanName,
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(attrs...),
		)
		db.Statement.Context = ctx
	}
}

func (p *otelPlugin) afterCallback(db *gorm.DB) {
	if db.Statement == nil {
		return
	}
	span := trace.SpanFromContext(db.Statement.Context)
	if !span.IsRecording() {
		return
	}
	defer span.End()

	if p.cfg.recordSQL && db.Statement.SQL.Len() > 0 {
		span.SetAttributes(attribute.String("db.statement", db.Statement.SQL.String()))
	}
	if db.Error != nil && db.Error != gorm.ErrRecordNotFound {
		span.SetStatus(codes.Error, db.Error.Error())
		span.RecordError(db.Error)
	}
}
