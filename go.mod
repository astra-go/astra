module github.com/astra-go/astra

go 1.25.1

// Core module — router, middleware, and zero/light-dep utility packages.
// Heavy integrations (OTel, GORM, MQ, Redis, gRPC, …) live in their own
// sub-modules under this monorepo and are versioned independently.
// Run `go mod tidy` after editing this file to refresh the indirect section.
require (
	github.com/astra-go/astra/testutil v0.1.0

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
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect

	// — shared transitive deps (crypto, net, sys, text) —
	// All pinned to the June-2024 release wave (Go 1.22 era).
	golang.org/x/crypto v0.52.0 // indirect
	golang.org/x/net v0.55.0
	golang.org/x/sys v0.45.0
	golang.org/x/text v0.37.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

require (
	github.com/goccy/go-json v0.10.6
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.22 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
)

require (
	github.com/astra-go/astra/middleware/observability v0.1.0 // test-only
	github.com/astra-go/astra/middleware/security v0.1.0 // test-only
	github.com/golang-jwt/jwt/v5 v5.3.1 // test-only
	github.com/prometheus/client_golang v1.23.2 // test-only
	gopkg.in/yaml.v3 v3.0.1
	modernc.org/sqlite v1.51.0 // test-only
)

require github.com/bytedance/sonic v1.15.1

require (
	github.com/astra-go/astra/cache v0.1.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bytedance/gopkg v0.1.3 // indirect
	github.com/bytedance/sonic/loader v0.5.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cloudwego/base64x v0.1.6 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/klauspost/cpuid/v2 v2.2.10 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.68.0 // indirect
	github.com/prometheus/otlptranslator v1.0.0 // indirect
	github.com/prometheus/procfs v0.20.1 // indirect
	github.com/redis/go-redis/v9 v9.20.0 // indirect
	github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.44.0 // indirect
	go.opentelemetry.io/otel/exporters/prometheus v0.66.0 // indirect
	go.opentelemetry.io/otel/metric v1.44.0 // indirect
	go.opentelemetry.io/otel/sdk v1.44.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.44.0 // indirect
	go.opentelemetry.io/otel/trace v1.44.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/arch v0.0.0-20210923205945-b76863e36670 // indirect
	modernc.org/libc v1.72.5 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)
