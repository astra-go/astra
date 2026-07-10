package otel

import (
	"context"

	gotel "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"gorm.io/gorm"
)

// GormOption configures the GORM tracing plugin.
type GormOption func(*gormConfig)

type gormConfig struct {
	tp         trace.TracerProvider
	tracerName string
	dbSystem   string
	recordSQL  bool
}

// WithGormApp binds the plugin to a specific *App.
func WithGormApp(a *App) GormOption {
	return func(c *gormConfig) {
		if a != nil {
			c.tp = a.tp
		}
	}
}

// WithGormTracerProvider overrides the TracerProvider (defaults to global).
func WithGormTracerProvider(tp trace.TracerProvider) GormOption {
	return func(c *gormConfig) { c.tp = tp }
}

// WithGormTracerName sets the instrumentation library name. Default: "astra/otel/gorm".
func WithGormTracerName(name string) GormOption {
	return func(c *gormConfig) { c.tracerName = name }
}

// WithGormDBSystem sets the db.system span attribute (e.g. "postgresql",
// "mysql", "sqlite"). Default: "unknown".
func WithGormDBSystem(system string) GormOption {
	return func(c *gormConfig) { c.dbSystem = system }
}

// WithGormRecordSQL enables capturing the SQL text in db.query.text. Disabled
// by default to avoid leaking parameterised queries (which may contain PII or
// credentials) into your trace backend.
func WithGormRecordSQL(enable bool) GormOption {
	return func(c *gormConfig) { c.recordSQL = enable }
}

// NewGormPlugin returns a gorm.Plugin that creates a client span for every
// database operation (Create, Query, Update, Delete, Row, Raw). The span is a
// child of the span already present on db.Statement.Context — which is set
// automatically when the statement runs inside a traced HTTP request or gRPC
// handler.
//
//	db, _ := gorm.Open(mysql.Open(dsn), &gorm.Config{})
//	db.Use(otel.NewGormPlugin(otel.WithGormDBSystem("mysql")))
func NewGormPlugin(opts ...GormOption) gorm.Plugin {
	c := &gormConfig{
		tp:         gotel.GetTracerProvider(),
		tracerName: "astra/otel/gorm",
		dbSystem:   "unknown",
	}
	for _, o := range opts {
		o(c)
	}
	var tracer trace.Tracer
	if c.tp != nil {
		tracer = c.tp.Tracer(c.tracerName)
	} else {
		tracer = gotel.Tracer(c.tracerName)
	}
	return &gormPlugin{cfg: c, tracer: tracer}
}

type gormPlugin struct {
	cfg    *gormConfig
	tracer trace.Tracer
}

// Name is the GORM plugin identifier.
func (p *gormPlugin) Name() string { return "astra-otel" }

// Initialize registers before/after hooks on every GORM callback chain.
// gorm v1.31 uses unexported *processor (returned by db.Callback()) and
// *callback (returned by processor.Before/After). We call methods directly
// on the unexported types via 'any' captures and type-assert to regFunc.
func (p *gormPlugin) Initialize(db *gorm.DB) error {
	type regFunc = func(string, func(*gorm.DB)) error

	register := func(scope, op string) error {
		beforeHook := "gorm:before_" + lower(scope)
		afterHook := "gorm:after_" + lower(scope)

		// Capture unexported *processor as any, then type-assert to call
		// its Before/After methods (which return unexported *gorm.callback).
		var proc any
		switch scope {
		case "Query":
			proc = db.Callback().Query()
		case "Update":
			proc = db.Callback().Update()
		case "Delete":
			proc = db.Callback().Delete()
		case "Row":
			proc = db.Callback().Row()
		case "Raw":
			proc = db.Callback().Raw()
		default:
			proc = db.Callback().Create()
		}

		// Type assert to call Before(name) → returns *gorm.callback as any.
		beforeCB := proc.(interface{ Before(string) any }).Before(beforeHook)
		if err := beforeCB.(regFunc)("otel:before_"+scope, p.beforeCallback(op)); err != nil {
			return err
		}
		afterCB := proc.(interface{ After(string) any }).After(afterHook)
		return afterCB.(regFunc)("otel:after_"+scope, p.afterCallback)
	}

	for _, ch := range []struct {
		scope string
		op    string
	}{
		{"Create", "INSERT"},
		{"Query", "SELECT"},
		{"Update", "UPDATE"},
		{"Delete", "DELETE"},
		{"Row", "ROW"},
		{"Raw", "RAW"},
	} {
		if err := register(ch.scope, ch.op); err != nil {
			return err
		}
	}
	return nil
}

func (p *gormPlugin) beforeCallback(op string) func(*gorm.DB) {
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

		// Zero-alloc: reuse a pooled slice; the SDK copies the values at Start.
		attrs := getAttrs()
		attrs.v = append(attrs.v,
			semconv.DBSystemKey.String(p.cfg.dbSystem),
			semconv.DBOperationNameKey.String(op),
		)
		if table != "" {
			attrs.v = append(attrs.v, attribute.String("db.sql.table", table))
		}

		ctx, _ = p.tracer.Start(ctx, spanName,
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(attrs.v...),
		)
		putAttrs(attrs)

		db.Statement.Context = ctx
	}
}

func (p *gormPlugin) afterCallback(db *gorm.DB) {
	if db.Statement == nil {
		return
	}
	span := trace.SpanFromContext(db.Statement.Context)
	if !span.IsRecording() {
		return
	}
	defer span.End()

	if p.cfg.recordSQL && db.Statement.SQL.Len() > 0 {
		span.SetAttributes(semconv.DBQueryTextKey.String(db.Statement.SQL.String()))
	}
	if db.Error != nil && db.Error != gorm.ErrRecordNotFound {
		span.SetStatus(codes.Error, db.Error.Error())
		span.RecordError(db.Error)
	}
}

func lower(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}
