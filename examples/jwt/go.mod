module github.com/astra-go/astra/examples/jwt

go 1.25.1

require (
	github.com/astra-go/astra v0.0.0-00010101000000-000000000000
	github.com/astra-go/astra/middleware/security v0.0.0-00010101000000-000000000000
	github.com/golang-jwt/jwt/v5 v5.3.0
	golang.org/x/crypto v0.48.0
)

replace github.com/astra-go/astra v0.0.0-00010101000000-000000000000 => ../..

replace github.com/astra-go/astra/middleware/security v0.0.0-00010101000000-000000000000 => ../../middleware/security
