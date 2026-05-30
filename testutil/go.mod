module github.com/astra-go/astra/testutil

// go 1.22.0 — downgraded from 1.25.9.
go 1.25.1

// Standalone testing utilities module.
require (
	github.com/astra-go/astra v0.0.0-00010101000000-000000000000
	github.com/astra-go/astra/cache v0.0.0-00010101000000-000000000000
)

require (
	github.com/gabriel-vasile/mimetype v1.4.9 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.26.0 // indirect
	github.com/goccy/go-json v0.10.3 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/stretchr/testify v1.11.1 // indirect
	golang.org/x/crypto v0.48.0 // indirect
	golang.org/x/net v0.51.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.35.0 // indirect
)

replace github.com/astra-go/astra v0.0.0-00010101000000-000000000000 => ..

replace github.com/astra-go/astra/cache v0.0.0-00010101000000-000000000000 => ../cache
