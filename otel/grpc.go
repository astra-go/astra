package otel

import (
	"context"
	"strings"
	"sync"

	gotel "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"google.golang.org/grpc"
	gcodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	gstatus "google.golang.org/grpc/status"
)

// GRPCOption configures the gRPC tracing interceptors.
type GRPCOption func(*grpcConfig)

type grpcConfig struct {
	tp         trace.TracerProvider
	prop       propagation.TextMapPropagator
	tracerName string
}

// WithGRPCApp binds the interceptors to a specific *App.
func WithGRPCApp(a *App) GRPCOption {
	return func(c *grpcConfig) {
		if a != nil {
			c.tp = a.tp
			c.prop = a.prop
		}
	}
}

// WithGRPCTracerProvider overrides the TracerProvider (defaults to global).
func WithGRPCTracerProvider(tp trace.TracerProvider) GRPCOption {
	return func(c *grpcConfig) { c.tp = tp }
}

// WithGRPCPropagator overrides the propagator (defaults to global).
func WithGRPCPropagator(p propagation.TextMapPropagator) GRPCOption {
	return func(c *grpcConfig) { c.prop = p }
}

func newGRPCConfig(opts []GRPCOption) *grpcConfig {
	c := &grpcConfig{
		tp:         gotel.GetTracerProvider(),
		prop:       gotel.GetTextMapPropagator(),
		tracerName: "astra/otel/grpc",
	}
	for _, o := range opts {
		o(c)
	}
	if c.prop == nil {
		c.prop = propagation.TraceContext{}
	}
	return c
}

// ─── propagator carrier over gRPC metadata ───────────────────────────────────

// mdCarrier adapts gRPC metadata.MD to the OTel TextMapCarrier interface so the
// same type works for both extracting inbound context and injecting outbound.
type mdCarrier struct{ md metadata.MD }

func (c mdCarrier) Get(key string) string {
	vals := c.md.Get(key)
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}

func (c mdCarrier) Set(key, val string) { c.md.Append(key, val) }

func (c mdCarrier) Keys() []string {
	keys := make([]string, 0, len(c.md))
	for k := range c.md {
		keys = append(keys, k)
	}
	return keys
}

// splitFullMethod splits "/package.Service/Method" into ("package.Service", "Method").
func splitFullMethod(full string) (service, method string) {
	full = strings.TrimPrefix(full, "/")
	if i := strings.LastIndex(full, "/"); i >= 0 {
		return full[:i], full[i+1:]
	}
	return "", full
}

// setGRPCStatus records the gRPC status code on the span and marks it errored
// for non-OK codes.
func setGRPCStatus(span trace.Span, err error) {
	if err == nil {
		span.SetAttributes(semconv.RPCGRPCStatusCodeKey.Int(int(gcodes.OK)))
		return
	}
	st, _ := gstatus.FromError(err)
	code := st.Code()
	span.SetAttributes(semconv.RPCGRPCStatusCodeKey.Int(int(code)))
	if code != gcodes.OK {
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
	}
}

func rpcSpanAttributes(service, method string, dst *attrSlice) {
	dst.v = append(dst.v,
		semconv.RPCSystemGRPC,
		semconv.RPCServiceKey.String(service),
		semconv.RPCMethodKey.String(method),
	)
}

// ─── server interceptors ──────────────────────────────────────────────────────

// GRPCServerUnaryInterceptor returns a unary server interceptor that extracts
// the inbound trace context, starts a server span, and records the gRPC status.
func (a *App) GRPCServerUnaryInterceptor() grpc.UnaryServerInterceptor {
	return GRPCServerUnaryInterceptor(WithGRPCApp(a))
}

// GRPCServerUnaryInterceptor is the package-level, App-agnostic counterpart.
func GRPCServerUnaryInterceptor(opts ...GRPCOption) grpc.UnaryServerInterceptor {
	c := newGRPCConfig(opts)
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		md, _ := metadata.FromIncomingContext(ctx)
		ctx = c.prop.Extract(ctx, mdCarrier{md})

		tracer := c.tp.Tracer(c.tracerName)
		service, method := splitFullMethod(info.FullMethod)

		a := getAttrs()
		rpcSpanAttributes(service, method, a)
		ctx, span := tracer.Start(ctx, info.FullMethod,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(a.v...),
		)
		putAttrs(a)
		defer span.End()

		resp, err := handler(ctx, req)
		setGRPCStatus(span, err)
		return resp, err
	}
}

// GRPCServerStreamInterceptor returns a stream server interceptor analogous to
// GRPCServerUnaryInterceptor.
func (a *App) GRPCServerStreamInterceptor() grpc.StreamServerInterceptor {
	return GRPCServerStreamInterceptor(WithGRPCApp(a))
}

// GRPCServerStreamInterceptor is the package-level, App-agnostic counterpart.
func GRPCServerStreamInterceptor(opts ...GRPCOption) grpc.StreamServerInterceptor {
	c := newGRPCConfig(opts)
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		md, _ := metadata.FromIncomingContext(ss.Context())
		ctx := c.prop.Extract(ss.Context(), mdCarrier{md})

		tracer := c.tp.Tracer(c.tracerName)
		service, method := splitFullMethod(info.FullMethod)

		a := getAttrs()
		rpcSpanAttributes(service, method, a)
		ctx, span := tracer.Start(ctx, info.FullMethod,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(a.v...),
		)
		putAttrs(a)
		defer span.End()

		err := handler(srv, &serverStreamWrapper{ServerStream: ss, ctx: ctx})
		setGRPCStatus(span, err)
		return err
	}
}

// ─── client interceptors ──────────────────────────────────────────────────────

// GRPCClientUnaryInterceptor returns a unary client interceptor that injects the
// active trace context into outgoing metadata and records a client span.
func (a *App) GRPCClientUnaryInterceptor() grpc.UnaryClientInterceptor {
	return GRPCClientUnaryInterceptor(WithGRPCApp(a))
}

// GRPCClientUnaryInterceptor is the package-level, App-agnostic counterpart.
func GRPCClientUnaryInterceptor(opts ...GRPCOption) grpc.UnaryClientInterceptor {
	c := newGRPCConfig(opts)
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, callOpts ...grpc.CallOption) error {
		tracer := c.tp.Tracer(c.tracerName)
		service, m := splitFullMethod(method)

		a := getAttrs()
		rpcSpanAttributes(service, m, a)
		ctx, span := tracer.Start(ctx, method,
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(a.v...),
		)
		putAttrs(a)
		defer span.End()

		md := metadata.MD{}
		c.prop.Inject(ctx, mdCarrier{md})
		ctx = metadata.NewOutgoingContext(ctx, md)

		err := invoker(ctx, method, req, reply, cc, callOpts...)
		setGRPCStatus(span, err)
		return err
	}
}

// GRPCClientStreamInterceptor returns a stream client interceptor that injects
// the active trace context and closes the client span when the stream ends.
func (a *App) GRPCClientStreamInterceptor() grpc.StreamClientInterceptor {
	return GRPCClientStreamInterceptor(WithGRPCApp(a))
}

// GRPCClientStreamInterceptor is the package-level, App-agnostic counterpart.
func GRPCClientStreamInterceptor(opts ...GRPCOption) grpc.StreamClientInterceptor {
	c := newGRPCConfig(opts)
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, callOpts ...grpc.CallOption) (grpc.ClientStream, error) {
		tracer := c.tp.Tracer(c.tracerName)
		service, m := splitFullMethod(method)

		a := getAttrs()
		rpcSpanAttributes(service, m, a)
		ctx, span := tracer.Start(ctx, method,
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(a.v...),
		)
		putAttrs(a)

		md := metadata.MD{}
		c.prop.Inject(ctx, mdCarrier{md})
		ctx = metadata.NewOutgoingContext(ctx, md)

		cs, err := streamer(ctx, desc, cc, method, callOpts...)
		if err != nil {
			span.End()
			return nil, err
		}
		return &clientStreamWrapper{ClientStream: cs, span: span}, nil
	}
}

// ─── stream wrappers ──────────────────────────────────────────────────────────

// serverStreamWrapper forwards the span context to the handler.
type serverStreamWrapper struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *serverStreamWrapper) Context() context.Context { return w.ctx }

// clientStreamWrapper ends the client span when the stream is closed or the
// final RecvMsg returns (EOF or error).
type clientStreamWrapper struct {
	grpc.ClientStream
	span trace.Span
	once sync.Once
}

func (w *clientStreamWrapper) finish(err error) {
	w.once.Do(func() {
		setGRPCStatus(w.span, err)
		w.span.End()
	})
}

func (w *clientStreamWrapper) CloseSend() error {
	err := w.ClientStream.CloseSend()
	w.finish(err)
	return err
}

func (w *clientStreamWrapper) RecvMsg(m interface{}) error {
	err := w.ClientStream.RecvMsg(m)
	if err != nil {
		w.finish(err)
	}
	return err
}
