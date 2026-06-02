module github.com/astra-go/astra/observability

go 1.25.1

// Unified observability facade for Astra.
// Depends on the core module (middleware, log) and the otel sub-module (SDK).
require (
	github.com/astra-go/astra v0.1.0
	github.com/astra-go/astra/middleware/observability v0.1.0
	github.com/astra-go/astra/otel v0.1.0
	github.com/astra-go/astra/testutil v0.1.0
	github.com/prometheus/client_golang v1.23.2
)

require (
	github.com/astra-go/astra/cache v0.1.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bytedance/gopkg v0.1.3 // indirect
	github.com/bytedance/sonic v1.15.1 // indirect
	github.com/bytedance/sonic/loader v0.5.1 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cloudwego/base64x v0.1.6 // indirect
	github.com/gabriel-vasile/mimetype v1.4.13 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.30.3 // indirect
	github.com/goccy/go-json v0.10.6 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.27.1 // indirect
	github.com/klauspost/cpuid/v2 v2.2.10 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.68.0 // indirect
	github.com/prometheus/otlptranslator v1.0.0 // indirect
	github.com/prometheus/procfs v0.20.1 // indirect
	github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.44.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.28.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.28.0 // indirect
	go.opentelemetry.io/otel/exporters/prometheus v0.66.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.28.0 // indirect
	go.opentelemetry.io/otel/metric v1.44.0 // indirect
	go.opentelemetry.io/otel/sdk v1.44.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.44.0 // indirect
	go.opentelemetry.io/otel/trace v1.44.0 // indirect
	go.opentelemetry.io/proto/otlp v1.7.1 // indirect
	golang.org/x/arch v0.0.0-20210923205945-b76863e36670 // indirect
	golang.org/x/crypto v0.52.0 // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
	golang.org/x/text v0.37.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20251202230838-ff82c1b0f217 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251202230838-ff82c1b0f217 // indirect
	google.golang.org/grpc v1.79.3 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace github.com/astra-go/astra v0.0.0-00010101000000-000000000000 => ..

replace github.com/astra-go/astra/alert v0.0.0-00010101000000-000000000000 => ../alert

replace github.com/astra-go/astra/auth v0.0.0-00010101000000-000000000000 => ../auth

replace github.com/astra-go/astra/benchmarks v0.0.0-00010101000000-000000000000 => ../benchmarks

replace github.com/astra-go/astra/cache v0.0.0-00010101000000-000000000000 => ../cache

replace github.com/astra-go/astra/client v0.0.0-00010101000000-000000000000 => ../client

replace github.com/astra-go/astra/config v0.0.0-00010101000000-000000000000 => ../config

replace github.com/astra-go/astra/discovery v0.0.0-00010101000000-000000000000 => ../discovery

replace github.com/astra-go/astra/dtx/orm v0.0.0-00010101000000-000000000000 => ../dtx/orm

replace github.com/astra-go/astra/dtx/redis v0.0.0-00010101000000-000000000000 => ../dtx/redis

replace github.com/astra-go/astra/e2e v0.0.0-00010101000000-000000000000 => ../e2e

replace github.com/astra-go/astra/e2e/orm v0.0.0-00010101000000-000000000000 => ../e2e/orm

replace github.com/astra-go/astra/examples/basic v0.0.0-00010101000000-000000000000 => ../examples/basic

replace github.com/astra-go/astra/examples/cache v0.0.0-00010101000000-000000000000 => ../examples/cache

replace github.com/astra-go/astra/examples/jwt v0.0.0-00010101000000-000000000000 => ../examples/jwt

replace github.com/astra-go/astra/examples/quic v0.0.0-00010101000000-000000000000 => ../examples/quic

replace github.com/astra-go/astra/examples/quickstart v0.0.0-00010101000000-000000000000 => ../examples/quickstart

replace github.com/astra-go/astra/examples/techempower v0.0.0-00010101000000-000000000000 => ../examples/techempower

replace github.com/astra-go/astra/examples/websocket v0.0.0-00010101000000-000000000000 => ../examples/websocket

replace github.com/astra-go/astra/grpc v0.0.0-00010101000000-000000000000 => ../grpc

replace github.com/astra-go/astra/loadbalance v0.0.0-00010101000000-000000000000 => ../loadbalance

replace github.com/astra-go/astra/lock v0.0.0-00010101000000-000000000000 => ../lock

replace github.com/astra-go/astra/lua v0.0.0-00010101000000-000000000000 => ../lua

replace github.com/astra-go/astra/middleware/observability v0.0.0-00010101000000-000000000000 => ../middleware/observability

replace github.com/astra-go/astra/middleware/security v0.0.0-00010101000000-000000000000 => ../middleware/security

replace github.com/astra-go/astra/mongodb v0.0.0-00010101000000-000000000000 => ../mongodb

replace github.com/astra-go/astra/mq v0.0.0-00010101000000-000000000000 => ../mq

replace github.com/astra-go/astra/notify v0.0.0-00010101000000-000000000000 => ../notify

replace github.com/astra-go/astra/orm v0.0.0-00010101000000-000000000000 => ../orm

replace github.com/astra-go/astra/orm/clickhouse v0.0.0-00010101000000-000000000000 => ../orm/clickhouse

replace github.com/astra-go/astra/otel v0.0.0-00010101000000-000000000000 => ../otel

replace github.com/astra-go/astra/quic v0.0.0-00010101000000-000000000000 => ../quic

replace github.com/astra-go/astra/rule v0.0.0-00010101000000-000000000000 => ../rule

replace github.com/astra-go/astra/runner v0.0.0-00010101000000-000000000000 => ../runner

replace github.com/astra-go/astra/search v0.0.0-00010101000000-000000000000 => ../search

replace github.com/astra-go/astra/session v0.0.0-00010101000000-000000000000 => ../session

replace github.com/astra-go/astra/storage v0.0.0-00010101000000-000000000000 => ../storage

replace github.com/astra-go/astra/stream v0.0.0-00010101000000-000000000000 => ../stream

replace github.com/astra-go/astra/taskqueue v0.0.0-00010101000000-000000000000 => ../taskqueue

replace github.com/astra-go/astra/testutil v0.0.0-00010101000000-000000000000 => ../testutil
