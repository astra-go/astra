module github.com/astra-go/astra/orm

// go 1.22.0 — downgraded from 1.25.9.
go 1.25.1

// Standalone ORM module — GORM with MySQL, PostgreSQL, ClickHouse, and SQLite drivers.
require (
	github.com/astra-go/astra v0.0.0-00010101000000-000000000000
	github.com/glebarez/sqlite v1.11.0
	gorm.io/driver/mysql v1.5.7
	gorm.io/driver/postgres v1.5.9
	gorm.io/gorm v1.30.0
	modernc.org/sqlite v1.50.1 // indirect
)

require (
	github.com/cespare/xxhash/v2 v2.3.0
	github.com/google/uuid v1.6.0
	go.opentelemetry.io/otel v1.43.0
	go.opentelemetry.io/otel/trace v1.43.0
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/gabriel-vasile/mimetype v1.4.9 // indirect
	github.com/glebarez/go-sqlite v1.21.2 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.26.0 // indirect
	github.com/go-sql-driver/mysql v1.7.0 // indirect
	github.com/goccy/go-json v0.10.3 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20221227161230-091c0ba34f0a // indirect
	github.com/jackc/pgx/v5 v5.5.5 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel/metric v1.43.0 // indirect
	golang.org/x/crypto v0.48.0 // indirect
	golang.org/x/net v0.51.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.35.0 // indirect
	modernc.org/libc v1.72.3 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)

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

replace github.com/astra-go/astra/mq v0.0.0-00010101000000-000000000000 => ../mq

replace github.com/astra-go/astra/notify v0.0.0-00010101000000-000000000000 => ../notify

replace github.com/astra-go/astra/observability v0.0.0-00010101000000-000000000000 => ../observability

replace github.com/astra-go/astra/orm/clickhouse v0.0.0-00010101000000-000000000000 => ./clickhouse

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
