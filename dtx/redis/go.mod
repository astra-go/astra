module github.com/astra-go/astra/dtx/redis

go 1.25.1

// Standalone Saga Redis-persistence module.
// Provides a dtx.StateStore and dtx.Recovery backed by Redis.
require (
	github.com/astra-go/astra v0.0.0-00010101000000-000000000000
	github.com/redis/go-redis/v9 v9.19.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/stretchr/testify v1.11.1 // indirect
	go.uber.org/atomic v1.11.0 // indirect
)

replace github.com/astra-go/astra v0.0.0-00010101000000-000000000000 => ../..
