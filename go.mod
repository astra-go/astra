module github.com/astra-go/astra

go 1.25.1

// Core module — router, middleware, and zero/light-dep utility packages.
// Heavy integrations (OTel, GORM, MQ, Redis, gRPC, …) live in their own
// sub-modules under this monorepo and are versioned independently.
// Run `go mod tidy` after editing this file to refresh the indirect section.
require (
	github.com/astra-go/astra/testutil v1.0.5

	// Request validation (validate/ package)
	github.com/go-playground/validator/v10 v10.30.3

	// WebSocket upgrade (websocket/ package)
	github.com/gorilla/websocket v1.5.3

	// Cron scheduler (cron/ package — used by runner/cron backend)
	github.com/robfig/cron/v3 v3.0.1
)

// Indirect dependencies for the core module's direct deps.
// Run `go mod tidy` in this directory after any dependency change.
require (
	// — go-playground/validator/v10 transitive deps —
	github.com/gabriel-vasile/mimetype v1.4.13 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect

	// — shared transitive deps (crypto, net, sys, text) —
	// All pinned to the June-2024 release wave (Go 1.22 era).
	golang.org/x/crypto v0.52.0 // indirect
	golang.org/x/net v0.55.0
	golang.org/x/sys v0.45.0
	golang.org/x/text v0.37.0 // indirect
)

require (
	github.com/goccy/go-json v0.10.6
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.22 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
)

require (
	github.com/astra-go/astra/middleware/security v1.0.5 // test-only
	github.com/golang-jwt/jwt/v5 v5.3.1 // test-only
	gopkg.in/yaml.v3 v3.0.1
	modernc.org/sqlite v1.51.0 // test-only
)

require (
	github.com/bytedance/sonic v1.15.1
	github.com/hashicorp/golang-lru/v2 v2.0.7
	github.com/lib/pq v1.12.3
)

require (
	github.com/astra-go/astra/cache v1.0.5 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bytedance/gopkg v0.1.3 // indirect
	github.com/bytedance/sonic/loader v0.5.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cloudwego/base64x v0.1.6 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/klauspost/cpuid/v2 v2.2.10 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_golang v1.20.5 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.62.0 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	github.com/redis/go-redis/v9 v9.20.0 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/arch v0.8.0 // indirect
	google.golang.org/protobuf v1.36.1 // indirect
	modernc.org/libc v1.72.5 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)

replace github.com/astra-go/astra/cache v1.0.5 => ./cache

replace github.com/astra-go/astra/middleware/security v1.0.5 => ./middleware/security

replace github.com/astra-go/astra/testutil v1.0.5 => ./testutil

replace github.com/astra-go/astra/taskqueue v1.0.5 => ./taskqueue

replace github.com/astra-go/astra/middleware/security v1.0.5 => ./middleware/security

replace github.com/astra-go/astra/notify v1.0.5 => ./notify

replace github.com/astra-go/astra/loadbalance v1.0.5 => ./loadbalance

replace github.com/astra-go/modproxy v1.0.5 => ./tools/modproxy

replace github.com/astra-go/astra/cache v1.0.5 => ./cache

replace github.com/astra-go/astra/grpc v1.0.5 => ./grpc

replace github.com/astra-go/astra/config v1.0.5 => ./config

replace github.com/astra-go/astra/config/vault v1.0.5 => ./config/vault

replace github.com/astra-go/astra/auth v1.0.5 => ./auth

replace github.com/astra-go/astra/alert v1.0.5 => ./alert

replace github.com/astra-go/astra/runner v1.0.5 => ./runner

replace github.com/astra-go/astra/stream v1.0.5 => ./stream

replace github.com/astra-go/astra/discovery v1.0.5 => ./discovery

replace github.com/astra-go/astra/lock v1.0.5 => ./lock

replace github.com/astra-go/astra/otel v1.0.5 => ./otel

replace github.com/astra-go/astra/mongodb v1.0.5 => ./mongodb

replace github.com/astra-go/astra/storage v1.0.5 => ./storage

replace github.com/astra-go/astra/rule v1.0.5 => ./rule

replace github.com/astra-go/astra/search v1.0.5 => ./search

replace github.com/astra-go/astra/magefiles v1.0.5 => ./magefiles

replace github.com/astra-go/astra/dtx/redis v1.0.5 => ./dtx/redis

replace github.com/astra-go/astra/dtx/orm v1.0.5 => ./dtx/orm

replace github.com/astra-go/astra/examples/jwt v1.0.5 => ./examples/jwt

replace github.com/astra-go/astra/examples/websocket v1.0.5 => ./examples/websocket

replace github.com/astra-go/astra/examples/cache v1.0.5 => ./examples/cache

replace example/wasm v1.0.5 => ./examples/wasm

replace github.com/astra-go/astra/examples/basic v1.0.5 => ./examples/basic

replace github.com/astra-go/astra/examples/techempower v1.0.5 => ./examples/techempower

replace github.com/astra-go/astra/examples/reference-blog v1.0.5 => ./examples/reference-blog

replace github.com/astra-go/astra/examples/quickstart v1.0.5 => ./examples/quickstart

replace example/crud v1.0.5 => ./examples/crud

replace example/orm v1.0.5 => ./examples/orm

replace github.com/astra-go/astra/examples/showcase v1.0.5 => ./examples/showcase

replace github.com/astra-go/astra/examples/quic v1.0.5 => ./examples/quic

replace github.com/astra-go/astra/benchmarks v1.0.5 => ./benchmarks

replace github.com/astra-go/astra/mq v1.0.5 => ./mq

replace github.com/astra-go/astra/orm/clickhouse v1.0.5 => ./orm/clickhouse

replace github.com/astra-go/astra/orm v1.0.5 => ./orm

replace github.com/astra-go/astra/e2e v1.0.5 => ./e2e

replace github.com/astra-go/astra/e2e/search v1.0.5 => ./e2e/search

replace github.com/astra-go/astra/e2e/orm v1.0.5 => ./e2e/orm

replace github.com/astra-go/astra/testutil v1.0.5 => ./testutil

replace github.com/astra-go/astra/client v1.0.5 => ./client

replace github.com/astra-go/astra/quic v1.0.5 => ./quic

replace github.com/astra-go/astra/session v1.0.5 => ./session
