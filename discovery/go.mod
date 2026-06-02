module github.com/astra-go/astra/discovery

// go 1.22.0 — downgraded from 1.25.9.
// Key dep changes:
//   etcd v3.6 → v3.5.16 (v3.5.x requires go 1.21; v3.6 requires go 1.23)
//   k8s  v0.35 → v0.31.3 (v0.31.x requires go 1.22; v0.32+ requires go 1.23)
//   consul v1.34 → v1.28.3 (v1.28.x is the last series requiring only go 1.22)
go 1.25.1

require github.com/astra-go/astra/testutil v0.1.0

require (
	github.com/astra-go/astra v0.1.0 // indirect
	github.com/astra-go/astra/cache v0.1.0 // indirect
	github.com/bytedance/gopkg v0.1.3 // indirect
	github.com/bytedance/sonic v1.15.1 // indirect
	github.com/bytedance/sonic/loader v0.5.1 // indirect
	github.com/cloudwego/base64x v0.1.6 // indirect
	github.com/gabriel-vasile/mimetype v1.4.13 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.30.3 // indirect
	github.com/goccy/go-json v0.10.6 // indirect
	github.com/klauspost/cpuid/v2 v2.2.10 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
	golang.org/x/arch v0.0.0-20210923205945-b76863e36670 // indirect
	golang.org/x/crypto v0.52.0 // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
	golang.org/x/text v0.37.0 // indirect
)

replace github.com/astra-go/astra v0.0.0-00010101000000-000000000000 => ..
