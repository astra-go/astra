module github.com/astra-go/astra/examples/techempower

// Standalone TechEmpower-style benchmark server.
// Depends only on the core astra module (no ORM, no middleware extras).
go 1.25.1

require github.com/astra-go/astra v0.0.0-00010101000000-000000000000

require (
	github.com/gabriel-vasile/mimetype v1.4.9 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.26.0 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/goccy/go-json v0.10.3 // indirect
	github.com/google/pprof v0.0.0-20241029153458-d1b30febd7db // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/onsi/ginkgo/v2 v2.21.0 // indirect
	github.com/quic-go/qpack v0.5.1 // indirect
	github.com/quic-go/quic-go v0.48.0 // indirect
	go.uber.org/mock v0.4.0 // indirect
	golang.org/x/crypto v0.48.0 // indirect
	golang.org/x/exp v0.0.0-20240719175910-8a7402abbf56 // indirect
	golang.org/x/mod v0.33.0 // indirect
	golang.org/x/net v0.51.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.35.0 // indirect
	golang.org/x/tools v0.42.0 // indirect
)

replace (
	github.com/astra-go/astra v0.0.0-00010101000000-000000000000 => ../../
	github.com/astra-go/astra/cache v0.0.0-00010101000000-000000000000 => ../../cache
	github.com/astra-go/astra/discovery v0.0.0-00010101000000-000000000000 => ../../discovery
	github.com/astra-go/astra/middleware/observability v0.0.0-00010101000000-000000000000 => ../../middleware/observability
	github.com/astra-go/astra/middleware/security v0.0.0-00010101000000-000000000000 => ../../middleware/security
	github.com/astra-go/astra/testutil v0.0.0-00010101000000-000000000000 => ../../testutil
)
