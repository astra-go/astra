module github.com/astra-go/astra

go 1.25.1

// Core module — router, middleware, and zero/light-dep utility packages.
// Heavy integrations (OTel, GORM, MQ, Redis, gRPC, …) live in their own
// sub-modules under this monorepo and are versioned independently.
// Run `go mod tidy` after editing this file to refresh the indirect section.
require (
	// Intra-workspace sub-modules (loadbalance/ and test helpers).
	// Zero-version + replace directive → go mod tidy skips VCS lookup.
	github.com/astra-go/astra/loadbalance v0.0.0-00010101000000-000000000000
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
	github.com/astra-go/astra/loadbalance v0.0.0-00010101000000-000000000000 => ./loadbalance
	github.com/astra-go/astra/testutil v0.0.0-00010101000000-000000000000 => ./testutil
)
