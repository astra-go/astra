module github.com/astra-go/astra/auth

// go 1.22.0 — downgraded from 1.25.9.
go 1.25.1

// Standalone authentication & authorization module.
//   auth/rbac  — Casbin-based RBAC middleware
//   auth/oauth2 — OAuth2/OIDC authorization-code flow with PKCE
require (
	github.com/astra-go/astra v0.1.0
	github.com/casbin/casbin/v2 v2.119.0
	golang.org/x/oauth2 v0.36.0
)

require github.com/astra-go/astra/testutil v0.1.0

require (
	github.com/astra-go/astra/cache v0.1.0 // indirect
	github.com/bmatcuk/doublestar/v4 v4.6.1 // indirect
	github.com/bytedance/gopkg v0.1.3 // indirect
	github.com/bytedance/sonic v1.15.1 // indirect
	github.com/bytedance/sonic/loader v0.5.1 // indirect
	github.com/casbin/govaluate v1.3.0 // indirect
	github.com/cloudwego/base64x v0.1.6 // indirect
	github.com/gabriel-vasile/mimetype v1.4.13 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.30.3 // indirect
	github.com/goccy/go-json v0.10.6 // indirect
	github.com/golang/mock v1.6.0 // indirect
	github.com/klauspost/cpuid/v2 v2.2.10 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
	golang.org/x/arch v0.8.0 // indirect
	golang.org/x/crypto v0.52.0 // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
	golang.org/x/text v0.37.0 // indirect
)

replace github.com/astra-go/astra v0.0.0-00010101000000-000000000000 => ./..

replace github.com/astra-go/astra/cache v0.0.0-00010101000000-000000000000 => ../cache
