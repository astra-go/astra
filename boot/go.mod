module github.com/astra-go/astra/boot

go 1.25.1

require (
	github.com/astra-go/astra v0.0.0-00010101000000-000000000000
	github.com/astra-go/astra/config v0.0.0-00010101000000-000000000000
)

// Local development replacements — must match the monorepo structure.
replace github.com/astra-go/astra v0.0.0-00010101000000-000000000000 => ..
replace github.com/astra-go/astra/config v0.0.0-00010101000000-000000000000 => ../config
