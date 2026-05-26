module github.com/astra-go/astra/auth

// go 1.22.0 — downgraded from 1.25.9.
go 1.25.8

// Standalone authentication & authorization module.
//   auth/rbac  — Casbin-based RBAC middleware
//   auth/oauth2 — OAuth2/OIDC authorization-code flow with PKCE
require (
	github.com/astra-go/astra v0.0.0-00010101000000-000000000000
	github.com/casbin/casbin/v2 v2.119.0
	golang.org/x/oauth2 v0.34.0
)

require github.com/astra-go/astra/testutil v0.0.0-00010101000000-000000000000

require (
	github.com/astra-go/astra/cache v0.0.0-00010101000000-000000000000 // indirect
	github.com/bmatcuk/doublestar/v4 v4.6.1 // indirect
	github.com/casbin/govaluate v1.3.0 // indirect
	github.com/gabriel-vasile/mimetype v1.4.9 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.26.0 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/goccy/go-json v0.10.3 // indirect
	github.com/golang/mock v1.6.0 // indirect
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
	golang.org/x/time v0.11.0 // indirect
	golang.org/x/tools v0.42.0 // indirect
)

replace (
	github.com/astra-go/astra v0.0.0-00010101000000-000000000000 => ../
	github.com/astra-go/astra/cache v0.0.0-00010101000000-000000000000 => ../cache
	github.com/astra-go/astra/discovery v0.0.0-00010101000000-000000000000 => ../discovery
	github.com/astra-go/astra/testutil v0.0.0-00010101000000-000000000000 => ../testutil
)
