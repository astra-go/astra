module github.com/astra-go/astra/lua

// go 1.22.0 — downgraded from 1.25.9.
// Key dep changes:
//   go-redis v9.18 → v9.6.1 (v9.6.x is the last series requiring only go 1.21)
//   gopher-lua v1.1.1 (v1.1.2 bumped to go 1.23; v1.1.1 declares go 1.17)
go 1.25.1

// Standalone Lua scripting module (gopher-lua engine + Redis EVAL bindings).
require (
	github.com/redis/go-redis/v9 v9.19.0
	github.com/yuin/gopher-lua v1.1.1
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/stretchr/testify v1.11.1 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
)








































