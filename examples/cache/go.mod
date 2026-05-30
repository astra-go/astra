module github.com/astra-go/astra/examples/cache

go 1.25.1

require (
	github.com/astra-go/astra v0.0.0-00010101000000-000000000000
	github.com/astra-go/astra/cache v0.0.0-00010101000000-000000000000
)

replace github.com/astra-go/astra v0.0.0-00010101000000-000000000000 => ../..

replace github.com/astra-go/astra/cache v0.0.0-00010101000000-000000000000 => ../../cache
