module github.com/astra-go/astra/dtx/redis

go 1.25.1

// Standalone Saga Redis-persistence module.
// Provides a dtx.StateStore and dtx.Recovery backed by Redis.
require (
	github.com/astra-go/astra v1.0.2
	github.com/redis/go-redis/v9 v9.20.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/stretchr/testify v1.11.1 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/arch v0.8.0 // indirect
)

