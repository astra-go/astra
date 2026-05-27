module github.com/astra-go/astra/mq

// go 1.22.0 — downgraded from 1.25.9.
// Key dep changes (version → last go-1.22-compatible release):
//   nats.go      v1.51  → v1.37.0  (v1.38+ requires go 1.23)
//   pulsar       v0.19  → v0.12.1  (v0.13+ requires go 1.22 but v0.19 requires go 1.24)
//   rocketmq/v5  v5.1.3 → v5.0.1   (v5.1+ requires go 1.24)
//   franz-go     v1.20  → v1.17.1  (v1.18.0 bumped to go 1.22; v1.20 requires go 1.24)
//   paho         v1.5.1 → v1.4.3   (v1.5.x requires go 1.24)
go 1.25.0

replace (
	github.com/astra-go/astra v0.0.0-00010101000000-000000000000 => ..
	github.com/astra-go/astra/cache v0.0.0-00010101000000-000000000000 => ../cache
	github.com/astra-go/astra/discovery v0.0.0-00010101000000-000000000000 => ../discovery
	github.com/astra-go/astra/testutil v0.0.0-00010101000000-000000000000 => ../testutil
)

replace github.com/astra-go/astra/alert v0.0.0-00010101000000-000000000000 => ../alert

replace github.com/astra-go/astra/auth v0.0.0-00010101000000-000000000000 => ../auth

replace github.com/astra-go/astra/benchmarks v0.0.0-00010101000000-000000000000 => ../benchmarks

replace github.com/astra-go/astra/client v0.0.0-00010101000000-000000000000 => ../client

replace github.com/astra-go/astra/config v0.0.0-00010101000000-000000000000 => ../config

replace github.com/astra-go/astra/dtx/orm v0.0.0-00010101000000-000000000000 => ../dtx/orm

replace github.com/astra-go/astra/dtx/redis v0.0.0-00010101000000-000000000000 => ../dtx/redis

replace github.com/astra-go/astra/e2e v0.0.0-00010101000000-000000000000 => ../e2e

replace github.com/astra-go/astra/e2e/orm v0.0.0-00010101000000-000000000000 => ../e2e/orm

replace github.com/astra-go/astra/examples/techempower v0.0.0-00010101000000-000000000000 => ../examples/techempower

replace github.com/astra-go/astra/grpc v0.0.0-00010101000000-000000000000 => ../grpc

replace github.com/astra-go/astra/loadbalance v0.0.0-00010101000000-000000000000 => ../loadbalance

replace github.com/astra-go/astra/lock v0.0.0-00010101000000-000000000000 => ../lock

replace github.com/astra-go/astra/lua v0.0.0-00010101000000-000000000000 => ../lua

replace github.com/astra-go/astra/middleware/observability v0.0.0-00010101000000-000000000000 => ../middleware/observability

replace github.com/astra-go/astra/middleware/security v0.0.0-00010101000000-000000000000 => ../middleware/security

replace github.com/astra-go/astra/mongodb v0.0.0-00010101000000-000000000000 => ../mongodb

replace github.com/astra-go/astra/notify v0.0.0-00010101000000-000000000000 => ../notify

replace github.com/astra-go/astra/observability v0.0.0-00010101000000-000000000000 => ../observability

replace github.com/astra-go/astra/orm v0.0.0-00010101000000-000000000000 => ../orm

replace github.com/astra-go/astra/orm/clickhouse v0.0.0-00010101000000-000000000000 => ../orm/clickhouse

replace github.com/astra-go/astra/otel v0.0.0-00010101000000-000000000000 => ../otel

replace github.com/astra-go/astra/runner v0.0.0-00010101000000-000000000000 => ../runner

replace github.com/astra-go/astra/search v0.0.0-00010101000000-000000000000 => ../search

replace github.com/astra-go/astra/session v0.0.0-00010101000000-000000000000 => ../session

replace github.com/astra-go/astra/storage v0.0.0-00010101000000-000000000000 => ../storage

replace github.com/astra-go/astra/stream v0.0.0-00010101000000-000000000000 => ../stream

replace github.com/astra-go/astra/taskqueue v0.0.0-00010101000000-000000000000 => ../taskqueue
