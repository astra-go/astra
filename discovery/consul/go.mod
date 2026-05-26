module github.com/astra-go/astra/discovery/consul

go 1.25.1

// Consul backend for the discovery module.
// Uses the Consul HTTP API directly — no consul SDK dependency.
require github.com/astra-go/astra/discovery v0.0.0-00010101000000-000000000000

replace github.com/astra-go/astra/discovery v0.0.0-00010101000000-000000000000 => ..
