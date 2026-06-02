module github.com/astra-go/astra/mq/rocketmq

go 1.25.1

require (
	github.com/apache/rocketmq-clients/golang/v5 v5.1.3
	github.com/astra-go/astra/mq v0.1.0
)

require (
	github.com/golang/mock v1.7.0-rc.1 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/natefinch/lumberjack v2.0.0+incompatible // indirect
	github.com/pkg/errors v0.9.1 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/net v0.51.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.35.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20251202230838-ff82c1b0f217 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251202230838-ff82c1b0f217 // indirect
	google.golang.org/grpc v1.79.3 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace github.com/astra-go/astra v0.0.0-00010101000000-000000000000 => ../..

replace github.com/astra-go/astra/alert v0.0.0-00010101000000-000000000000 => ../../alert

replace github.com/astra-go/astra/auth v0.0.0-00010101000000-000000000000 => ../../auth

replace github.com/astra-go/astra/benchmarks v0.0.0-00010101000000-000000000000 => ../../benchmarks

replace github.com/astra-go/astra/cache v0.0.0-00010101000000-000000000000 => ../../cache

replace github.com/astra-go/astra/client v0.0.0-00010101000000-000000000000 => ../../client

replace github.com/astra-go/astra/config v0.0.0-00010101000000-000000000000 => ../../config

replace github.com/astra-go/astra/discovery v0.0.0-00010101000000-000000000000 => ../../discovery

replace github.com/astra-go/astra/dtx/orm v0.0.0-00010101000000-000000000000 => ../../dtx/orm

replace github.com/astra-go/astra/dtx/redis v0.0.0-00010101000000-000000000000 => ../../dtx/redis

replace github.com/astra-go/astra/e2e v0.0.0-00010101000000-000000000000 => ../../e2e

replace github.com/astra-go/astra/e2e/orm v0.0.0-00010101000000-000000000000 => ../../e2e/orm

replace github.com/astra-go/astra/examples/basic v0.0.0-00010101000000-000000000000 => ../../examples/basic

replace github.com/astra-go/astra/examples/cache v0.0.0-00010101000000-000000000000 => ../../examples/cache

replace github.com/astra-go/astra/examples/jwt v0.0.0-00010101000000-000000000000 => ../../examples/jwt

replace github.com/astra-go/astra/examples/quic v0.0.0-00010101000000-000000000000 => ../../examples/quic

replace github.com/astra-go/astra/examples/quickstart v0.0.0-00010101000000-000000000000 => ../../examples/quickstart

replace github.com/astra-go/astra/examples/techempower v0.0.0-00010101000000-000000000000 => ../../examples/techempower

replace github.com/astra-go/astra/examples/websocket v0.0.0-00010101000000-000000000000 => ../../examples/websocket

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

replace github.com/astra-go/astra/quic v0.0.0-00010101000000-000000000000 => ../../quic

replace github.com/astra-go/astra/rule v0.0.0-00010101000000-000000000000 => ../../rule

replace github.com/astra-go/astra/runner v0.0.0-00010101000000-000000000000 => ../../runner

replace github.com/astra-go/astra/search v0.0.0-00010101000000-000000000000 => ../../search

replace github.com/astra-go/astra/session v0.0.0-00010101000000-000000000000 => ../../session

replace github.com/astra-go/astra/storage v0.0.0-00010101000000-000000000000 => ../../storage

replace github.com/astra-go/astra/stream v0.0.0-00010101000000-000000000000 => ../../stream

replace github.com/astra-go/astra/taskqueue v0.0.0-00010101000000-000000000000 => ../../taskqueue

replace github.com/astra-go/astra/testutil v0.0.0-00010101000000-000000000000 => ../../testutil
