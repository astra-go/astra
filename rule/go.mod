module github.com/astra-go/astra/rule

go 1.25.1

require (
	github.com/expr-lang/expr v1.17.8
	github.com/redis/go-redis/v9 v9.20.0 // indirect for lua build tag
	github.com/yuin/gopher-lua v1.1.1 // indirect for lua build tag
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
)
