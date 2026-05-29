module example/wasm

go 1.22.0

require github.com/astra-go/astra v0.0.0-00010101000000-000000000000

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/gabriel-vasile/mimetype v1.4.9 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.26.0 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/goccy/go-json v0.10.3 // indirect
	github.com/golang-jwt/jwt/v5 v5.2.3 // indirect
	github.com/google/pprof v0.0.0-20241029153458-d1b30febd7db // indirect
	github.com/klauspost/compress v1.18.5 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/onsi/ginkgo/v2 v2.21.0 // indirect
	github.com/prometheus/client_golang v1.20.5 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.62.0 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	github.com/quic-go/qpack v0.5.1 // indirect
	github.com/quic-go/quic-go v0.48.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.42.0 // indirect
	go.opentelemetry.io/otel/metric v1.42.0 // indirect
	go.opentelemetry.io/otel/trace v1.42.0 // indirect
	go.uber.org/mock v0.4.0 // indirect
	golang.org/x/crypto v0.48.0 // indirect
	golang.org/x/exp v0.0.0-20240719175910-8a7402abbf56 // indirect
	golang.org/x/mod v0.33.0 // indirect
	golang.org/x/net v0.33.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.35.0 // indirect
	golang.org/x/tools v0.42.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	modernc.org/gc/v3 v3.0.0-20240107210532-573471604cb6 // indirect
	modernc.org/libc v1.61.0 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.8.0 // indirect
	modernc.org/sqlite v1.33.1 // indirect
	modernc.org/strutil v1.2.1 // indirect
	modernc.org/token v1.1.0 // indirect
)

replace (
	github.com/astra-go/astra v0.0.0-00010101000000-000000000000 => ../..
	github.com/astra-go/astra/cache v0.0.0-00010101000000-000000000000 => ../../cache
	github.com/astra-go/astra/discovery v0.0.0-00010101000000-000000000000 => ../../discovery
	github.com/astra-go/astra/testutil v0.0.0-00010101000000-000000000000 => ../../testutil
)

replace github.com/astra-go/astra/alert v0.0.0-00010101000000-000000000000 => ../../alert

replace github.com/astra-go/astra/auth v0.0.0-00010101000000-000000000000 => ../../auth

replace github.com/astra-go/astra/benchmarks v0.0.0-00010101000000-000000000000 => ../../benchmarks

replace github.com/astra-go/astra/client v0.0.0-00010101000000-000000000000 => ../../client

replace github.com/astra-go/astra/config v0.0.0-00010101000000-000000000000 => ../../config

replace github.com/astra-go/astra/dtx/orm v0.0.0-00010101000000-000000000000 => ../../dtx/orm

replace github.com/astra-go/astra/dtx/redis v0.0.0-00010101000000-000000000000 => ../../dtx/redis

replace github.com/astra-go/astra/e2e v0.0.0-00010101000000-000000000000 => ../../e2e

replace github.com/astra-go/astra/e2e/orm v0.0.0-00010101000000-000000000000 => ../../e2e/orm

replace github.com/astra-go/astra/examples/techempower v0.0.0-00010101000000-000000000000 => ../techempower

replace github.com/astra-go/astra/grpc v0.0.0-00010101000000-000000000000 => ../../grpc

replace github.com/astra-go/astra/loadbalance v0.0.0-00010101000000-000000000000 => ../../loadbalance

replace github.com/astra-go/astra/lock v0.0.0-00010101000000-000000000000 => ../../lock

replace github.com/astra-go/astra/lua v0.0.0-00010101000000-000000000000 => ../../lua

replace github.com/astra-go/astra/middleware/observability v0.0.0-00010101000000-000000000000 => ../../middleware/observability

replace github.com/astra-go/astra/middleware/security v0.0.0-00010101000000-000000000000 => ../../middleware/security

replace github.com/astra-go/astra/mongodb v0.0.0-00010101000000-000000000000 => ../../mongodb

replace github.com/astra-go/astra/mq v0.0.0-00010101000000-000000000000 => ../../mq

replace github.com/astra-go/astra/notify v0.0.0-00010101000000-000000000000 => ../../notify

replace github.com/astra-go/astra/observability v0.0.0-00010101000000-000000000000 => ../../observability

replace github.com/astra-go/astra/orm v0.0.0-00010101000000-000000000000 => ../../orm

replace github.com/astra-go/astra/orm/clickhouse v0.0.0-00010101000000-000000000000 => ../../orm/clickhouse

replace github.com/astra-go/astra/otel v0.0.0-00010101000000-000000000000 => ../../otel

replace github.com/astra-go/astra/runner v0.0.0-00010101000000-000000000000 => ../../runner

replace github.com/astra-go/astra/search v0.0.0-00010101000000-000000000000 => ../../search

replace github.com/astra-go/astra/session v0.0.0-00010101000000-000000000000 => ../../session

replace github.com/astra-go/astra/storage v0.0.0-00010101000000-000000000000 => ../../storage

replace github.com/astra-go/astra/stream v0.0.0-00010101000000-000000000000 => ../../stream

replace github.com/astra-go/astra/taskqueue v0.0.0-00010101000000-000000000000 => ../../taskqueue

replace github.com/astra-go/astra/examples/basic v0.0.0-00010101000000-000000000000 => ../basic

replace github.com/astra-go/astra/examples/cache v0.0.0-00010101000000-000000000000 => ../cache

replace github.com/astra-go/astra/examples/jwt v0.0.0-00010101000000-000000000000 => ../jwt

replace github.com/astra-go/astra/examples/quickstart v0.0.0-00010101000000-000000000000 => ../quickstart

replace github.com/astra-go/astra/examples/websocket v0.0.0-00010101000000-000000000000 => ../websocket

replace github.com/astra-go/astra/quic v0.0.0-00010101000000-000000000000 => ../../quic

replace github.com/astra-go/astra/rule v0.0.0-00010101000000-000000000000 => ../../rule

replace github.com/astra-go/astra/examples/quic v0.0.0-00010101000000-000000000000 => ../quic
