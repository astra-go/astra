module github.com/astra-go/astra/taskqueue

// go 1.22.0 — downgraded from 1.25.9.
// Key dep changes:
//   rocketmq/v5     v5.1.3 → v5.0.1   (v5.1+ requires go 1.24)
//   franz-go        v1.20  → v1.17.1  (v1.20 requires go 1.24; v1.17.x requires go 1.21)
//   mongo-driver/v2 v2.5.1 → v2.0.0   (v2.0.0 released 2024-04, requires go 1.22)
//   go-redis        v9.18  → v9.6.1   (aligned with other modules)
go 1.25.1

// Standalone task-queue module — persistent task scheduling with horizontal scaling.
// Brokers: Redis, RabbitMQ, Kafka, MongoDB, RocketMQ.
// Upgrade any broker dependency without affecting the router or OTel stack.
require (
	github.com/apache/rocketmq-clients/golang/v5 v5.1.3
	github.com/google/uuid v1.6.0
	github.com/rabbitmq/amqp091-go v1.11.0
	github.com/redis/go-redis/v9 v9.20.0
	github.com/robfig/cron/v3 v3.0.1
	github.com/twmb/franz-go v1.21.2
	go.mongodb.org/mongo-driver/v2 v2.5.1
)

require (
	contrib.go.opencensus.io/exporter/ocagent v0.7.0 // indirect
	github.com/BurntSushi/toml v1.6.0 // indirect
	github.com/census-instrumentation/opencensus-proto v0.4.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dchest/siphash v1.2.3 // indirect
	github.com/gabriel-vasile/mimetype v1.4.13 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.30.3 // indirect
	github.com/golang/groupcache v0.0.0-20241129210726-2c02b8208cf8 // indirect
	github.com/golang/mock v1.6.0 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.27.1 // indirect
	github.com/klauspost/compress v1.18.6 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/natefinch/lumberjack v2.0.0+incompatible // indirect
	github.com/pierrec/lz4 v2.6.1+incompatible // indirect
	github.com/pierrec/lz4/v4 v4.1.26 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/stretchr/testify v1.11.1 // indirect
	github.com/twmb/franz-go/pkg/kmsg v1.13.1 // indirect
	github.com/valyala/fastrand v1.1.0 // indirect
	github.com/xdg-go/pbkdf2 v1.0.0 // indirect
	github.com/xdg-go/scram v1.2.0 // indirect
	github.com/xdg-go/stringprep v1.0.4 // indirect
	github.com/youmark/pkcs8 v0.0.0-20240726163527-a2c0da244d78 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/otel v1.44.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.44.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/crypto v0.52.0 // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
	golang.org/x/text v0.37.0 // indirect
	google.golang.org/api v0.230.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20251202230838-ff82c1b0f217 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251202230838-ff82c1b0f217 // indirect
	google.golang.org/grpc v1.79.3 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
)
