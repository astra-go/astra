module github.com/astra-go/astra

go 1.25.1

// Core module — router, middleware, and zero/light-dep utility packages.
// Heavy integrations (OTel, GORM, MQ, Redis, gRPC, …) live in their own
// sub-modules under this monorepo and are versioned independently.
// Run `go mod tidy` after editing this file to refresh the indirect section.
require (
	github.com/astra-go/astra/testutil v0.0.0-00010101000000-000000000000

	// Request validation (validate/ package)
	github.com/go-playground/validator/v10 v10.26.0

	// WebSocket upgrade (websocket/ package)
	github.com/gorilla/websocket v1.5.3

	// Cron scheduler (cron/ package — used by runner/cron backend)
	github.com/robfig/cron/v3 v3.0.1
)

// Indirect dependencies for the core module's direct deps.
// Run `go mod tidy` in this directory after any dependency change.
require (
	// — go-playground/validator/v10 transitive deps —
	github.com/gabriel-vasile/mimetype v1.4.9 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect

	// — shared transitive deps (crypto, net, sys, text) —
	// All pinned to the June-2024 release wave (Go 1.22 era).
	golang.org/x/crypto v0.48.0 // indirect
	golang.org/x/net v0.51.0
	golang.org/x/sys v0.42.0
	golang.org/x/text v0.35.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

require (
	github.com/goccy/go-json v0.10.3
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
)

require (
	github.com/astra-go/astra/middleware/observability v0.0.0-00010101000000-000000000000 // test-only
	github.com/astra-go/astra/middleware/security v0.0.0-00010101000000-000000000000 // test-only
	github.com/golang-jwt/jwt/v5 v5.3.1 // test-only
	github.com/prometheus/client_golang v1.23.2 // test-only
	gopkg.in/yaml.v3 v3.0.1
	modernc.org/sqlite v1.50.1 // test-only
)

require (
	github.com/astra-go/astra/cache v0.0.0-00010101000000-000000000000 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.67.5 // indirect
	github.com/prometheus/otlptranslator v1.0.0 // indirect
	github.com/prometheus/procfs v0.19.2 // indirect
	github.com/redis/go-redis/v9 v9.19.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.43.0 // indirect
	go.opentelemetry.io/otel/exporters/prometheus v0.64.0 // indirect
	go.opentelemetry.io/otel/metric v1.43.0 // indirect
	go.opentelemetry.io/otel/sdk v1.42.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.42.0 // indirect
	go.opentelemetry.io/otel/trace v1.43.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.yaml.in/yaml/v2 v2.4.3 // indirect
	modernc.org/libc v1.72.3 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)

// Local replace directives — go mod tidy does not honor go.work replace
// during version resolution, so we mirror them here for the intra-workspace deps.
replace (
	github.com/astra-go/astra/cache v0.0.0-00010101000000-000000000000 => ./cache
	github.com/astra-go/astra/loadbalance v0.0.0-00010101000000-000000000000 => ./loadbalance
	github.com/astra-go/astra/testutil v0.0.0-00010101000000-000000000000 => ./testutil
)

replace github.com/astra-go/astra/lock v0.0.0-00010101000000-000000000000 => ./lock

replace github.com/astra-go/astra/middleware/security v0.0.0-00010101000000-000000000000 => ./middleware/security

replace github.com/astra-go/astra/otel v0.0.0-00010101000000-000000000000 => ./otel

replace github.com/astra-go/astra/session v0.0.0-00010101000000-000000000000 => ./session

replace github.com/astra-go/astra/discovery v0.0.0-00010101000000-000000000000 => ./discovery

replace github.com/astra-go/astra/notify v0.0.0-00010101000000-000000000000 => ./notify

replace github.com/astra-go/astra/orm v0.0.0-00010101000000-000000000000 => ./orm

replace github.com/astra-go/astra/client v0.0.0-00010101000000-000000000000 => ./client

replace github.com/astra-go/astra/search v0.0.0-00010101000000-000000000000 => ./search

replace github.com/astra-go/astra/config v0.0.0-00010101000000-000000000000 => ./config

replace github.com/astra-go/astra/lua v0.0.0-00010101000000-000000000000 => ./lua

replace github.com/astra-go/astra/mq v0.0.0-00010101000000-000000000000 => ./mq

replace github.com/astra-go/astra/orm/clickhouse v0.0.0-00010101000000-000000000000 => ./orm/clickhouse

replace github.com/astra-go/astra/dtx/redis v0.0.0-00010101000000-000000000000 => ./dtx/redis

replace github.com/astra-go/astra/e2e v0.0.0-00010101000000-000000000000 => ./e2e

replace github.com/astra-go/astra/examples/techempower v0.0.0-00010101000000-000000000000 => ./examples/techempower

replace github.com/astra-go/astra/middleware/observability v0.0.0-00010101000000-000000000000 => ./middleware/observability

replace github.com/astra-go/astra/mongodb v0.0.0-00010101000000-000000000000 => ./mongodb

replace github.com/astra-go/astra/taskqueue v0.0.0-00010101000000-000000000000 => ./taskqueue

replace github.com/astra-go/astra/alert v0.0.0-00010101000000-000000000000 => ./alert

replace github.com/astra-go/astra/auth v0.0.0-00010101000000-000000000000 => ./auth

replace github.com/astra-go/astra/grpc v0.0.0-00010101000000-000000000000 => ./grpc

replace github.com/astra-go/astra/runner v0.0.0-00010101000000-000000000000 => ./runner

replace github.com/astra-go/astra/stream v0.0.0-00010101000000-000000000000 => ./stream

replace github.com/astra-go/astra/dtx/orm v0.0.0-00010101000000-000000000000 => ./dtx/orm

replace github.com/astra-go/astra/e2e/orm v0.0.0-00010101000000-000000000000 => ./e2e/orm

replace github.com/astra-go/astra/observability v0.0.0-00010101000000-000000000000 => ./observability

replace github.com/astra-go/astra/storage v0.0.0-00010101000000-000000000000 => ./storage

replace github.com/astra-go/astra/benchmarks v0.0.0-00010101000000-000000000000 => ./benchmarks

replace github.com/astra-go/astra/examples/jwt v0.0.0-00010101000000-000000000000 => ./examples/jwt

replace github.com/astra-go/astra/examples/basic v0.0.0-00010101000000-000000000000 => ./examples/basic

replace github.com/astra-go/astra/examples/quickstart v0.0.0-00010101000000-000000000000 => ./examples/quickstart

replace github.com/astra-go/astra/quic v0.0.0-00010101000000-000000000000 => ./quic

replace github.com/astra-go/astra/examples/cache v0.0.0-00010101000000-000000000000 => ./examples/cache

replace github.com/astra-go/astra/examples/websocket v0.0.0-00010101000000-000000000000 => ./examples/websocket
