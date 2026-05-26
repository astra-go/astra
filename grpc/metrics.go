// metrics.go — OTel metrics interceptors for Astra gRPC servers.
//
// Records three instruments per RPC:
//   - astra.grpc.requests        (Int64Counter)      — total calls by method + status
//   - astra.grpc.request.duration (Float64Histogram) — latency in seconds
//   - astra.grpc.requests.active  (Int64UpDownCounter) — in-flight calls
//
// Usage:
//
//	grpcserver.WithUnaryInterceptors(
//	    grpcserver.UnaryInterceptorMetrics(grpcserver.GRPCMetricsConfig{}),
//	)
//	grpcserver.WithStreamInterceptors(
//	    grpcserver.StreamInterceptorMetrics(grpcserver.GRPCMetricsConfig{}),
//	)
package grpcserver

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GRPCMetricsConfig configures the gRPC metrics interceptors.
type GRPCMetricsConfig struct {
	// ScopeName is the OTel instrumentation scope name. Default: "astra.grpc".
	ScopeName string
	// MeterProvider to use. Default: otel.GetMeterProvider() (global).
	MeterProvider metric.MeterProvider
	// DurationBuckets for the request duration histogram (seconds).
	// Default: {.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}
	DurationBuckets []float64
}

var defaultGRPCDurationBuckets = []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}

type grpcInstruments struct {
	requestsTotal   metric.Int64Counter
	requestDuration metric.Float64Histogram
	requestsActive  metric.Int64UpDownCounter
}

func newGRPCInstruments(cfg GRPCMetricsConfig) (*grpcInstruments, error) {
	if cfg.ScopeName == "" {
		cfg.ScopeName = "astra.grpc"
	}
	if cfg.MeterProvider == nil {
		cfg.MeterProvider = otel.GetMeterProvider()
	}
	if len(cfg.DurationBuckets) == 0 {
		cfg.DurationBuckets = defaultGRPCDurationBuckets
	}

	m := cfg.MeterProvider.Meter(cfg.ScopeName)

	total, err := m.Int64Counter("astra.grpc.requests",
		metric.WithDescription("Total number of gRPC requests processed, partitioned by method and status."),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, err
	}

	dur, err := m.Float64Histogram("astra.grpc.request.duration",
		metric.WithDescription("gRPC request latency distribution."),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(cfg.DurationBuckets...),
	)
	if err != nil {
		return nil, err
	}

	active, err := m.Int64UpDownCounter("astra.grpc.requests.active",
		metric.WithDescription("Number of gRPC requests currently being processed."),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, err
	}

	return &grpcInstruments{
		requestsTotal:   total,
		requestDuration: dur,
		requestsActive:  active,
	}, nil
}

func grpcStatusCode(err error) codes.Code {
	if err == nil {
		return codes.OK
	}
	if s, ok := status.FromError(err); ok {
		return s.Code()
	}
	return codes.Internal
}

// UnaryInterceptorMetrics returns a gRPC unary server interceptor that records
// OTel metrics for each RPC call.
//
// If meter initialisation fails (e.g. no MeterProvider configured), the
// interceptor degrades gracefully to a pass-through.
func UnaryInterceptorMetrics(cfg GRPCMetricsConfig) grpc.UnaryServerInterceptor {
	inst, err := newGRPCInstruments(cfg)
	if err != nil {
		return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
			return h(ctx, req)
		}
	}

	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		baseAttrs := []attribute.KeyValue{
			attribute.String("rpc.system", "grpc"),
			attribute.String("rpc.method", info.FullMethod),
		}
		inst.requestsActive.Add(ctx, 1, metric.WithAttributes(baseAttrs...))
		start := time.Now()

		resp, err := handler(ctx, req)

		elapsed := time.Since(start).Seconds()
		finalAttrs := append(baseAttrs,
			attribute.String("rpc.grpc.status_code", grpcStatusCode(err).String()),
		)
		inst.requestsActive.Add(ctx, -1, metric.WithAttributes(baseAttrs...))
		inst.requestsTotal.Add(ctx, 1, metric.WithAttributes(finalAttrs...))
		inst.requestDuration.Record(ctx, elapsed, metric.WithAttributes(finalAttrs...))

		return resp, err
	}
}

// StreamInterceptorMetrics returns a gRPC stream server interceptor that records
// OTel metrics for each streaming RPC. Latency covers the full stream lifetime.
func StreamInterceptorMetrics(cfg GRPCMetricsConfig) grpc.StreamServerInterceptor {
	inst, err := newGRPCInstruments(cfg)
	if err != nil {
		return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, h grpc.StreamHandler) error {
			return h(srv, ss)
		}
	}

	return func(
		srv any,
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		ctx := ss.Context()
		baseAttrs := []attribute.KeyValue{
			attribute.String("rpc.system", "grpc"),
			attribute.String("rpc.method", info.FullMethod),
			attribute.Bool("rpc.stream.client", info.IsClientStream),
			attribute.Bool("rpc.stream.server", info.IsServerStream),
		}
		inst.requestsActive.Add(ctx, 1, metric.WithAttributes(baseAttrs...))
		start := time.Now()

		err := handler(srv, ss)

		elapsed := time.Since(start).Seconds()
		finalAttrs := append(baseAttrs,
			attribute.String("rpc.grpc.status_code", grpcStatusCode(err).String()),
		)
		inst.requestsActive.Add(ctx, -1, metric.WithAttributes(baseAttrs...))
		inst.requestsTotal.Add(ctx, 1, metric.WithAttributes(finalAttrs...))
		inst.requestDuration.Record(ctx, elapsed, metric.WithAttributes(finalAttrs...))

		return err
	}
}
