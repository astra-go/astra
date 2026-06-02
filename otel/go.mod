module github.com/astra-go/astra/otel

// go 1.22.0 — downgraded from 1.25.9.
// OTel SDK v1.28.0 is the last release whose own go.mod requires only go 1.22.
// go.opentelemetry.io/otel v1.30.0 bumped the requirement to go 1.23.
go 1.25.1

// Standalone OpenTelemetry integration module.
// Depends only on the upstream OTel SDK and gRPC transport — zero coupling
// with the Astra router, GORM, or any MQ client.
require (
	github.com/prometheus/client_golang v1.23.2
	go.opentelemetry.io/otel v1.42.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.28.0
	// prometheus exporter v0.50.0 corresponds to SDK v1.28.0
	go.opentelemetry.io/otel/exporters/prometheus v0.64.0
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.28.0
	go.opentelemetry.io/otel/sdk v1.42.0
	go.opentelemetry.io/otel/sdk/metric v1.42.0
	go.opentelemetry.io/otel/trace v1.42.0
	// grpc v1.65.0 is the last series whose own go.mod requires only go 1.21.
	// grpc v1.68.0 bumped the requirement to go 1.22; v1.70+ to go 1.23.
	google.golang.org/grpc v1.79.3
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.27.1 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.67.5 // indirect
	github.com/prometheus/otlptranslator v1.0.0 // indirect
	github.com/prometheus/procfs v0.19.2 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.28.0 // indirect
	go.opentelemetry.io/otel/metric v1.42.0 // indirect
	go.opentelemetry.io/proto/otlp v1.7.1 // indirect
	go.yaml.in/yaml/v2 v2.4.3 // indirect
	golang.org/x/net v0.51.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.35.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20251202230838-ff82c1b0f217 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251202230838-ff82c1b0f217 // indirect
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

replace github.com/astra-go/astra/observability v0.0.0-00010101000000-000000000000 => ../observability

replace github.com/astra-go/astra/orm v0.0.0-00010101000000-000000000000 => ../orm

replace github.com/astra-go/astra/orm/clickhouse v0.0.0-00010101000000-000000000000 => ../orm/clickhouse

replace github.com/astra-go/astra/quic v0.0.0-00010101000000-000000000000 => ../quic

replace github.com/astra-go/astra/rule v0.0.0-00010101000000-000000000000 => ../rule

replace github.com/astra-go/astra/runner v0.0.0-00010101000000-000000000000 => ../runner

replace github.com/astra-go/astra/search v0.0.0-00010101000000-000000000000 => ../search

replace github.com/astra-go/astra/session v0.0.0-00010101000000-000000000000 => ../session

replace github.com/astra-go/astra/storage v0.0.0-00010101000000-000000000000 => ../storage

replace github.com/astra-go/astra/stream v0.0.0-00010101000000-000000000000 => ../stream

replace github.com/astra-go/astra/taskqueue v0.0.0-00010101000000-000000000000 => ../taskqueue

replace github.com/astra-go/astra/testutil v0.0.0-00010101000000-000000000000 => ../testutil
