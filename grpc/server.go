// Package grpc provides gRPC dual-stack support for Astra.
//
// Run an HTTP server and a gRPC server side-by-side from a single application:
//
//	s := grpcserver.New(app,
//	    grpcserver.WithHTTPAddr(":8080"),
//	    grpcserver.WithGRPCAddr(":9090"),
//	    grpcserver.WithTimeout(5*time.Second),      // Kratos-style per-call timeout
//	    grpcserver.WithUnaryInterceptors(
//	        grpcserver.UnaryInterceptorRecovery(),
//	        grpcserver.UnaryInterceptorTracing(),
//	        grpcserver.UnaryInterceptorLogger(),
//	    ),
//	)
//	pb.RegisterGreeterServer(s.GRPC, &greeterImpl{})
//	s.Run()
package grpcserver

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/astra-go/astra"
	gotel "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

// ServerOptions configures the dual-stack server.
type ServerOptions struct {
	httpAddr        string
	grpcAddr        string
	grpcServerOpts  []grpc.ServerOption
	shutdownTimeout time.Duration
	// Kratos-style additions
	timeout   time.Duration // per-call deadline; 0 = no limit
	tlsConfig *tls.Config   // gRPC TLS config; nil = plaintext
}

// Option is a functional option for the dual-stack server.
type Option func(*ServerOptions)

// WithHTTPAddr sets the HTTP listening address. Default: ":8080".
func WithHTTPAddr(addr string) Option {
	return func(o *ServerOptions) { o.httpAddr = addr }
}

// WithGRPCAddr sets the gRPC listening address. Default: ":9090".
func WithGRPCAddr(addr string) Option {
	return func(o *ServerOptions) { o.grpcAddr = addr }
}

// WithGRPCServerOptions passes additional grpc.ServerOptions (e.g. TLS, interceptors).
func WithGRPCServerOptions(opts ...grpc.ServerOption) Option {
	return func(o *ServerOptions) { o.grpcServerOpts = append(o.grpcServerOpts, opts...) }
}

// WithShutdownTimeout sets the graceful shutdown timeout. Default: 10s.
func WithShutdownTimeout(d time.Duration) Option {
	return func(o *ServerOptions) { o.shutdownTimeout = d }
}

// WithTimeout sets the per-call server-side deadline (Kratos-style).
// When a request arrives without a deadline and this value is > 0, the server
// automatically applies a context.WithTimeout so handlers cannot run forever.
// Default: 0 (no timeout enforced by the server).
func WithTimeout(d time.Duration) Option {
	return func(o *ServerOptions) { o.timeout = d }
}

// WithTLSConfig enables TLS for the gRPC listener using the provided config.
// Typical usage:
//
//	cert, _ := tls.LoadX509KeyPair("server.crt", "server.key")
//	grpcserver.New(app, grpcserver.WithTLSConfig(&tls.Config{Certificates: []tls.Certificate{cert}}))
func WithTLSConfig(c *tls.Config) Option {
	return func(o *ServerOptions) { o.tlsConfig = c }
}

// Server manages both an Astra HTTP server and a gRPC server.
type Server struct {
	// HTTP is the underlying Astra application (add routes here).
	HTTP *astra.App
	// GRPC is the underlying gRPC server (register services here).
	GRPC *grpc.Server

	opts       ServerOptions
	healthSrv  *health.Server
	grpcLis    net.Listener
	httpServer *http.Server // kept for graceful shutdown
}

// New creates a dual-stack server wrapping the given Astra app.
func New(app *astra.App, opts ...Option) *Server {
	options := ServerOptions{
		httpAddr:        ":8080",
		grpcAddr:        ":9090",
		shutdownTimeout: 10 * time.Second,
	}
	for _, opt := range opts {
		opt(&options)
	}

	// ── Default gRPC server options ──────────────────────────────────────────
	defaultGRPCOpts := []grpc.ServerOption{
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle: 5 * time.Minute,
			Time:              2 * time.Minute,
			Timeout:           20 * time.Second,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             30 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.MaxRecvMsgSize(4 << 20), // 4MB
		grpc.MaxSendMsgSize(4 << 20),
	}

	// ── TLS (Kratos-style WithTLSConfig) ─────────────────────────────────────
	if options.tlsConfig != nil {
		defaultGRPCOpts = append(defaultGRPCOpts,
			grpc.Creds(credentials.NewTLS(options.tlsConfig)))
	}

	// ── Built-in interceptors (Kratos-style) ─────────────────────────────────
	// These are always prepended before user-supplied interceptors so that
	// structured errors are encoded correctly regardless of user middleware order.
	var builtinUnary []grpc.UnaryServerInterceptor
	if options.timeout > 0 {
		builtinUnary = append(builtinUnary, UnaryInterceptorTimeout(options.timeout))
	}
	builtinUnary = append(builtinUnary, unaryInterceptorErrors())
	defaultGRPCOpts = append(defaultGRPCOpts, grpc.ChainUnaryInterceptor(builtinUnary...))

	grpcSrv := grpc.NewServer(append(defaultGRPCOpts, options.grpcServerOpts...)...)

	// Register health check service
	healthSrv := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcSrv, healthSrv)

	// Enable server reflection for grpcurl / grpc_cli / Evans
	reflection.Register(grpcSrv)

	return &Server{
		HTTP:      app,
		GRPC:      grpcSrv,
		opts:      options,
		healthSrv: healthSrv,
	}
}

// SetServiceStatus sets the health status for a specific gRPC service.
// Use grpc_health_v1.HealthCheckResponse_SERVING or NOT_SERVING.
func (s *Server) SetServiceStatus(service string, status grpc_health_v1.HealthCheckResponse_ServingStatus) {
	s.healthSrv.SetServingStatus(service, status)
}

// Run starts both servers and blocks until shutdown.
// It listens for SIGINT / SIGTERM to trigger graceful shutdown.
func (s *Server) Run() error {
	// Pre-bind gRPC listener (fail fast before starting HTTP)
	lis, err := net.Listen("tcp", s.opts.grpcAddr)
	if err != nil {
		return fmt.Errorf("grpc: listen %s: %w", s.opts.grpcAddr, err)
	}
	s.grpcLis = lis

	// Create the HTTP server explicitly so we can shut it down in shutdown().
	httpSrv := &http.Server{
		Addr:         s.opts.httpAddr,
		Handler:      s.HTTP,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	s.httpServer = httpSrv

	errCh := make(chan error, 2)
	var wg sync.WaitGroup

	// ── gRPC server ──────────────────────────────────────────────────────────
	wg.Add(1)
	go func() {
		defer wg.Done()
		slog.Info("gRPC server started", slog.String("addr", s.opts.grpcAddr))
		if err := s.GRPC.Serve(lis); err != nil {
			errCh <- fmt.Errorf("grpc: %w", err)
		}
	}()

	// ── HTTP server ──────────────────────────────────────────────────────────
	wg.Add(1)
	go func() {
		defer wg.Done()
		slog.Info("HTTP server started", slog.String("addr", s.opts.httpAddr))
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("http: %w", err)
		}
	}()

	// ── Signal handler ───────────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		s.shutdown()
		return err
	case sig := <-quit:
		slog.Info("shutdown signal received", slog.String("signal", sig.String()))
		s.shutdown()
	}

	// Wait for both goroutines to exit
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()

	select {
	case <-done:
	case <-time.After(s.opts.shutdownTimeout + 2*time.Second):
		slog.Warn("servers did not stop in time, forcing exit")
	}

	return nil
}

// shutdown gracefully stops both servers.
func (s *Server) shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), s.opts.shutdownTimeout)
	defer cancel()

	// Mark all services as not serving
	s.healthSrv.Shutdown()

	var wg sync.WaitGroup

	// Gracefully stop the HTTP server.
	if s.httpServer != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.httpServer.Shutdown(ctx); err != nil {
				slog.Warn("HTTP server shutdown error", slog.String("err", err.Error()))
			} else {
				slog.Info("HTTP server stopped gracefully")
			}
		}()
	}

	// Gracefully stop the gRPC server.
	wg.Add(1)
	go func() {
		defer wg.Done()
		stopped := make(chan struct{})
		go func() { s.GRPC.GracefulStop(); close(stopped) }()
		select {
		case <-stopped:
			slog.Info("gRPC server stopped gracefully")
		case <-ctx.Done():
			s.GRPC.Stop()
			slog.Warn("gRPC server force-stopped")
		}
	}()

	wg.Wait()
}

// UnaryInterceptorLogger returns a gRPC unary interceptor that logs requests.
func UnaryInterceptorLogger() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		slog.Info("grpc request",
			slog.String("method", info.FullMethod),
			slog.Duration("latency", time.Since(start)),
			slog.Bool("error", err != nil),
		)
		return resp, err
	}
}

// UnaryInterceptorRecovery returns a gRPC unary interceptor that recovers from panics.
func UnaryInterceptorRecovery() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp any, err error) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("grpc panic recovered",
					slog.String("method", info.FullMethod),
					slog.Any("panic", r),
				)
				err = fmt.Errorf("internal server error")
			}
		}()
		return handler(ctx, req)
	}
}

// StreamInterceptorLogger returns a gRPC stream interceptor that logs streaming RPCs.
// It records the method name, whether the RPC is client/server/bidirectional streaming,
// and the total duration once the stream is closed.
func StreamInterceptorLogger() grpc.StreamServerInterceptor {
	return func(
		srv any,
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		start := time.Now()
		err := handler(srv, ss)
		slog.Info("grpc stream",
			slog.String("method", info.FullMethod),
			slog.Bool("client_stream", info.IsClientStream),
			slog.Bool("server_stream", info.IsServerStream),
			slog.Duration("latency", time.Since(start)),
			slog.Bool("error", err != nil),
		)
		return err
	}
}

// StreamInterceptorRecovery returns a gRPC stream interceptor that recovers
// from panics inside streaming handlers. The panic is logged and converted to
// an internal gRPC error so the client receives a structured error rather than
// a broken stream.
func StreamInterceptorRecovery() grpc.StreamServerInterceptor {
	return func(
		srv any,
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) (err error) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("grpc stream panic recovered",
					slog.String("method", info.FullMethod),
					slog.Any("panic", r),
				)
				err = fmt.Errorf("internal server error")
			}
		}()
		return handler(srv, ss)
	}
}

// ─── OTel tracing interceptors ────────────────────────────────────────────────

// metadataCarrier adapts gRPC metadata to satisfy propagation.TextMapCarrier,
// enabling W3C TraceContext / Baggage extraction and injection over gRPC.
type metadataCarrier metadata.MD

func (mc metadataCarrier) Get(key string) string {
	vals := metadata.MD(mc).Get(key)
	if len(vals) > 0 {
		return vals[0]
	}
	return ""
}

func (mc metadataCarrier) Set(key, value string) {
	metadata.MD(mc).Set(key, value)
}

func (mc metadataCarrier) Keys() []string {
	md := metadata.MD(mc)
	keys := make([]string, 0, len(md))
	for k := range md {
		keys = append(keys, k)
	}
	return keys
}

// UnaryInterceptorTracing returns a gRPC unary server interceptor that:
//   - Extracts the W3C TraceContext from incoming metadata (propagation.TraceContext).
//   - Creates a child span named after the gRPC method.
//   - Injects span attributes (rpc.system, rpc.method, grpc status code).
//   - Marks the span as failed when the handler returns a non-nil error.
//
// Prerequisites: call otel.Setup (or set global TracerProvider + TextMapPropagator)
// before registering this interceptor.
func UnaryInterceptorTracing() grpc.UnaryServerInterceptor {
	tracer := gotel.GetTracerProvider().Tracer("grpc")
	prop := gotel.GetTextMapPropagator()

	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		// Extract parent span from incoming metadata.
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			md = metadata.MD{}
		}
		ctx = prop.Extract(ctx, metadataCarrier(md))

		ctx, span := tracer.Start(ctx, info.FullMethod,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("rpc.system", "grpc"),
				attribute.String("rpc.method", info.FullMethod),
			),
		)
		defer span.End()

		resp, err := handler(ctx, req)
		if err != nil {
			s, _ := status.FromError(err)
			span.SetStatus(otelcodes.Error, s.Message())
			span.SetAttributes(attribute.Int("rpc.grpc.status_code", int(s.Code())))
		} else {
			span.SetAttributes(attribute.Int("rpc.grpc.status_code", int(codes.OK)))
		}
		return resp, err
	}
}

// StreamInterceptorTracing returns a gRPC stream server interceptor that traces
// the full lifecycle of a streaming RPC.
func StreamInterceptorTracing() grpc.StreamServerInterceptor {
	tracer := gotel.GetTracerProvider().Tracer("grpc")
	prop := gotel.GetTextMapPropagator()

	return func(
		srv any,
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		ctx := ss.Context()
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			md = metadata.MD{}
		}
		ctx = prop.Extract(ctx, metadataCarrier(md))

		ctx, span := tracer.Start(ctx, info.FullMethod,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("rpc.system", "grpc"),
				attribute.String("rpc.method", info.FullMethod),
				attribute.Bool("rpc.stream.client", info.IsClientStream),
				attribute.Bool("rpc.stream.server", info.IsServerStream),
			),
		)
		defer span.End()

		err := handler(srv, &wrappedServerStream{ServerStream: ss, ctx: ctx})
		if err != nil {
			s, _ := status.FromError(err)
			span.SetStatus(otelcodes.Error, s.Message())
			span.SetAttributes(attribute.Int("rpc.grpc.status_code", int(s.Code())))
		}
		return err
	}
}

// wrappedServerStream wraps grpc.ServerStream to propagate a modified context.
// Used by stream interceptors (tracing, middleware) that need to inject a
// new context (with span, auth tokens, etc.) into the streaming handler.
type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *wrappedServerStream) Context() context.Context { return s.ctx }

// ─── Interceptor chain helpers ────────────────────────────────────────────────

// WithUnaryInterceptors is a convenience Option that prepends the given
// unary server interceptors (in order) before any user-supplied ones.
//
// Usage:
//
//	s := grpcserver.New(app,
//	    grpcserver.WithUnaryInterceptors(
//	        grpcserver.UnaryInterceptorRecovery(),
//	        grpcserver.UnaryInterceptorTracing(),
//	        grpcserver.UnaryInterceptorLogger(),
//	    ),
//	)
func WithUnaryInterceptors(interceptors ...grpc.UnaryServerInterceptor) Option {
	return func(o *ServerOptions) {
		o.grpcServerOpts = append(o.grpcServerOpts,
			grpc.ChainUnaryInterceptor(interceptors...),
		)
	}
}

// WithStreamInterceptors is a convenience Option that chains the given
// stream server interceptors.
func WithStreamInterceptors(interceptors ...grpc.StreamServerInterceptor) Option {
	return func(o *ServerOptions) {
		o.grpcServerOpts = append(o.grpcServerOpts,
			grpc.ChainStreamInterceptor(interceptors...),
		)
	}
}

// ─── gRPC ↔ HTTP status mapping ──────────────────────────────────────────────

// GRPCStatusToHTTPError converts a gRPC status error to an astra.HTTPError
// using the standard gRPC ↔ HTTP status mapping table.
// If err is nil or not a gRPC status error, it is returned as-is.
//
// Useful in gRPC-gateway / transcoding layers, or when a service calls an
// upstream gRPC backend and wants to surface HTTP-friendly errors.
func GRPCStatusToHTTPError(err error) error {
	if err == nil {
		return nil
	}
	s, ok := status.FromError(err)
	if !ok {
		return err
	}
	return astra.NewHTTPError(grpcCodeToHTTPStatus(s.Code()), s.Message())
}

// grpcCodeToHTTPStatus maps gRPC canonical codes to HTTP status codes.
// Based on the gRPC → HTTP/2 status mapping from the gRPC spec.
func grpcCodeToHTTPStatus(code codes.Code) int {
	switch code {
	case codes.OK:
		return http.StatusOK
	case codes.Canceled:
		return 499 // Client Closed Request (nginx convention)
	case codes.Unknown:
		return http.StatusInternalServerError
	case codes.InvalidArgument:
		return http.StatusBadRequest
	case codes.DeadlineExceeded:
		return http.StatusGatewayTimeout
	case codes.NotFound:
		return http.StatusNotFound
	case codes.AlreadyExists:
		return http.StatusConflict
	case codes.PermissionDenied:
		return http.StatusForbidden
	case codes.ResourceExhausted:
		return http.StatusTooManyRequests
	case codes.FailedPrecondition:
		return http.StatusBadRequest
	case codes.Aborted:
		return http.StatusConflict
	case codes.OutOfRange:
		return http.StatusBadRequest
	case codes.Unimplemented:
		return http.StatusNotImplemented
	case codes.Internal:
		return http.StatusInternalServerError
	case codes.Unavailable:
		return http.StatusServiceUnavailable
	case codes.DataLoss:
		return http.StatusInternalServerError
	case codes.Unauthenticated:
		return http.StatusUnauthorized
	default:
		return http.StatusInternalServerError
	}
}

// ─── Kratos-style built-in interceptors ───────────────────────────────────────

// UnaryInterceptorTimeout returns a gRPC unary interceptor that enforces a
// per-call timeout (Kratos-style WithTimeout).
//
// If the incoming context already has a deadline that is tighter than d, the
// existing deadline is respected and no new one is added.
// If d == 0, the interceptor is a no-op.
func UnaryInterceptorTimeout(d time.Duration) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		_ *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		if d <= 0 {
			return handler(ctx, req)
		}
		if _, ok := ctx.Deadline(); ok {
			// Caller already set a deadline — honour it.
			return handler(ctx, req)
		}
		ctx, cancel := context.WithTimeout(ctx, d)
		defer cancel()
		return handler(ctx, req)
	}
}

// unaryInterceptorErrors is the internal error-encoding interceptor.
// It converts any error returned from a handler into a properly encoded gRPC
// status so that Kratos-compatible clients can decode it via FromError:
//
//   - *Error values are encoded via GRPCStatus() (carrying errdetails.ErrorInfo).
//   - Plain errors are wrapped as 500 Internal Server Error.
//   - Already-encoded gRPC status errors are passed through unchanged.
func unaryInterceptorErrors() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		_ *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		resp, err := handler(ctx, req)
		if err == nil {
			return resp, nil
		}
		return resp, encodeGRPCError(err)
	}
}

// encodeGRPCError ensures err is an encoded gRPC status error.
func encodeGRPCError(err error) error {
	// *Error implements GRPCStatus() — gRPC encodes it automatically.
	// We call it explicitly to make the status wire-format explicit.
	var e *Error
	if errors.As(err, &e) {
		return e.GRPCStatus().Err()
	}
	// Already a gRPC status error — pass through.
	if _, ok := status.FromError(err); ok {
		return err
	}
	// Plain Go error — wrap as 500 Internal Server Error.
	return InternalServer("INTERNAL", err.Error()).GRPCStatus().Err()
}
