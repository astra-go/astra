module github.com/astra-go/astra

go 1.25.1

// Core module — router, middleware, and zero/light-dep utility packages.
// Heavy integrations (OTel, GORM, MQ, Redis, gRPC, …) live in their own
// sub-modules under this monorepo and are versioned independently.
// Run `go mod tidy` after editing this file to refresh the indirect section.
require (
	// Intra-workspace sub-modules (loadbalance/ and test helpers).
	// Zero-version + replace directive → go mod tidy skips VCS lookup.
	github.com/astra-go/astra/discovery v0.0.0-00010101000000-000000000000
	github.com/astra-go/astra/middleware/observability v0.0.0-00010101000000-000000000000
	github.com/astra-go/astra/middleware/security v0.0.0-00010101000000-000000000000
	github.com/astra-go/astra/testutil v0.0.0-00010101000000-000000000000

	// Request validation (validate/ package)
	github.com/go-playground/validator/v10 v10.26.0

	// WebSocket upgrade (websocket/ package)
	github.com/gorilla/websocket v1.5.3

	// HTTP/3 server (app_quic.go — method on *App, must stay in core)
	// v0.48.x is the last series whose own go.mod requires only go 1.22.
	github.com/quic-go/quic-go v0.48.0

	// Cron scheduler (cron/ package — used by runner/cron backend)
	github.com/robfig/cron/v3 v3.0.1
)

// Indirect dependencies for the core module's direct deps.
// Run `go mod tidy` in this directory after any dependency change.
require (
	// — prometheus/client_golang transitive deps —
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect

	// — go-playground/validator/v10 transitive deps —
	github.com/gabriel-vasile/mimetype v1.4.9 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/prometheus/common v0.67.5 // indirect
	github.com/prometheus/procfs v0.19.2 // indirect

	// — quic-go transitive deps —
	// qpack v0.5.1 corresponds to quic-go v0.48.x.
	github.com/quic-go/qpack v0.5.1 // indirect

	// — shared transitive deps (crypto, net, sys, text) —
	// All pinned to the June-2024 release wave (Go 1.22 era).
	golang.org/x/crypto v0.48.0
	golang.org/x/exp v0.0.0-20240719175910-8a7402abbf56 // indirect
	golang.org/x/mod v0.33.0 // indirect
	golang.org/x/net v0.51.0
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.42.0
	golang.org/x/text v0.35.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

require (
	github.com/golang-jwt/jwt/v5 v5.3.0
	github.com/prometheus/client_golang v1.23.2
	github.com/prometheus/client_model v0.6.2
	go.opentelemetry.io/otel/trace v1.42.0
	gopkg.in/yaml.v3 v3.0.1
	modernc.org/sqlite v1.33.1
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/onsi/gomega v1.35.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/otlptranslator v1.0.0 // indirect
	github.com/redis/go-redis/v9 v9.19.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.42.0 // indirect
	go.opentelemetry.io/otel/exporters/prometheus v0.64.0 // indirect
	go.opentelemetry.io/otel/metric v1.42.0 // indirect
	go.opentelemetry.io/otel/sdk v1.42.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.42.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.yaml.in/yaml/v2 v2.4.3 // indirect
	modernc.org/gc/v3 v3.0.0-20240107210532-573471604cb6 // indirect
	modernc.org/strutil v1.2.1 // indirect
	modernc.org/token v1.1.0 // indirect
)

require (
	github.com/astra-go/astra/cache v0.0.0-00010101000000-000000000000
	github.com/goccy/go-json v0.10.3
	github.com/google/pprof v0.0.0-20241029153458-d1b30febd7db // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/onsi/ginkgo/v2 v2.21.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	go.uber.org/mock v0.4.0 // indirect
	golang.org/x/tools v0.42.0 // indirect
	modernc.org/libc v1.61.0 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.8.0 // indirect
)

// Local replace directives — go mod tidy does not honor go.work replace
// during version resolution, so we mirror them here for the intra-workspace deps.
replace (
	github.com/astra-go/astra/cache v0.0.0-00010101000000-000000000000 => ./cache
	github.com/astra-go/astra/discovery v0.0.0-00010101000000-000000000000 => ./discovery
	github.com/astra-go/astra/middleware/observability v0.0.0-00010101000000-000000000000 => ./middleware/observability
	github.com/astra-go/astra/middleware/security v0.0.0-00010101000000-000000000000 => ./middleware/security
	github.com/astra-go/astra/testutil v0.0.0-00010101000000-000000000000 => ./testutil
)
