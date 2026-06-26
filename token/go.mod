module github.com/astra-go/astra/token

go 1.25.1

// Token lifecycle management — opaque Access + Refresh token pairs.
// Memory backend (zero deps) in this module; Redis backend in redis.go.
require github.com/redis/go-redis/v9 v9.21.0

require github.com/cespare/xxhash/v2 v2.3.0 // indirect
require github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
