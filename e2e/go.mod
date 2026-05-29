module github.com/astra-go/astra/e2e

go 1.25.1

require (
	github.com/astra-go/astra v0.0.0-00010101000000-000000000000
	github.com/astra-go/astra/grpc v0.0.0-00010101000000-000000000000
	github.com/golang-jwt/jwt/v5 v5.3.0
	github.com/google/uuid v1.6.0
	github.com/gorilla/websocket v1.5.3
	golang.org/x/crypto v0.48.0
	google.golang.org/grpc v1.79.3
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/gabriel-vasile/mimetype v1.4.9 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.26.0 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/goccy/go-json v0.10.3 // indirect
	github.com/google/pprof v0.0.0-20241029153458-d1b30febd7db // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.27.1 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/onsi/ginkgo/v2 v2.21.0 // indirect
	github.com/quic-go/qpack v0.5.1 // indirect
	github.com/quic-go/quic-go v0.48.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.42.0 // indirect
	go.opentelemetry.io/otel/metric v1.42.0 // indirect
	go.opentelemetry.io/otel/sdk v1.42.0 // indirect
	go.opentelemetry.io/otel/trace v1.42.0 // indirect
	go.uber.org/mock v0.4.0 // indirect
	golang.org/x/exp v0.0.0-20240719175910-8a7402abbf56 // indirect
	golang.org/x/mod v0.33.0 // indirect
	golang.org/x/net v0.51.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.35.0 // indirect
	golang.org/x/tools v0.42.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20251202230838-ff82c1b0f217 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251202230838-ff82c1b0f217 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace (
	github.com/astra-go/astra v0.0.0-00010101000000-000000000000 => ..
	github.com/astra-go/astra/cache v0.0.0-00010101000000-000000000000 => ../cache
	github.com/astra-go/astra/discovery v0.0.0-00010101000000-000000000000 => ../discovery
	github.com/astra-go/astra/grpc v0.0.0-00010101000000-000000000000 => ../grpc
	github.com/astra-go/astra/testutil v0.0.0-00010101000000-000000000000 => ../testutil
)

replace github.com/astra-go/astra/alert v0.0.0-00010101000000-000000000000 => ../alert

replace github.com/astra-go/astra/auth v0.0.0-00010101000000-000000000000 => ../auth

replace github.com/astra-go/astra/benchmarks v0.0.0-00010101000000-000000000000 => ../benchmarks

replace github.com/astra-go/astra/client v0.0.0-00010101000000-000000000000 => ../client

replace github.com/astra-go/astra/config v0.0.0-00010101000000-000000000000 => ../config

replace github.com/astra-go/astra/dtx/orm v0.0.0-00010101000000-000000000000 => ../dtx/orm

replace github.com/astra-go/astra/dtx/redis v0.0.0-00010101000000-000000000000 => ../dtx/redis

replace github.com/astra-go/astra/e2e/orm v0.0.0-00010101000000-000000000000 => ./orm

replace github.com/astra-go/astra/examples/techempower v0.0.0-00010101000000-000000000000 => ../examples/techempower

replace github.com/astra-go/astra/loadbalance v0.0.0-00010101000000-000000000000 => ../loadbalance

replace github.com/astra-go/astra/lock v0.0.0-00010101000000-000000000000 => ../lock

replace github.com/astra-go/astra/lua v0.0.0-00010101000000-000000000000 => ../lua

replace github.com/astra-go/astra/middleware/observability v0.0.0-00010101000000-000000000000 => ../middleware/observability

replace github.com/astra-go/astra/middleware/security v0.0.0-00010101000000-000000000000 => ../middleware/security

replace github.com/astra-go/astra/mongodb v0.0.0-00010101000000-000000000000 => ../mongodb

replace github.com/astra-go/astra/mq v0.0.0-00010101000000-000000000000 => ../mq

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

replace github.com/astra-go/astra/examples/basic v0.0.0-00010101000000-000000000000 => ../examples/basic

replace github.com/astra-go/astra/examples/cache v0.0.0-00010101000000-000000000000 => ../examples/cache

replace github.com/astra-go/astra/examples/jwt v0.0.0-00010101000000-000000000000 => ../examples/jwt

replace github.com/astra-go/astra/examples/quickstart v0.0.0-00010101000000-000000000000 => ../examples/quickstart

replace github.com/astra-go/astra/examples/websocket v0.0.0-00010101000000-000000000000 => ../examples/websocket

replace github.com/astra-go/astra/quic v0.0.0-00010101000000-000000000000 => ../quic

replace github.com/astra-go/astra/rule v0.0.0-00010101000000-000000000000 => ../rule

replace github.com/astra-go/astra/examples/quic v0.0.0-00010101000000-000000000000 => ../examples/quic
