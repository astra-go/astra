module github.com/astra-go/astra/examples/reference-blog

go 1.25.1

require (
	github.com/astra-go/astra v0.0.0-00010101000000-000000000000
	github.com/astra-go/astra/auth v0.0.0-00010101000000-000000000000
	github.com/astra-go/astra/cache v0.0.0-00010101000000-000000000000
	github.com/astra-go/astra/grpc v0.0.0-00010101000000-000000000000
	github.com/astra-go/astra/mq v0.0.0-00010101000000-000000000000
	github.com/astra-go/astra/orm v0.0.0-00010101000000-000000000000
	github.com/astra-go/astra/otel v0.0.0-00010101000000-000000000000
	github.com/astra-go/astra/search v0.0.0-00010101000000-000000000000
	github.com/golang-jwt/jwt/v5 v5.2.1
	github.com/google/uuid v1.6.0

	github.com/stretchr/testify v1.10.0
	golang.org/x/crypto v0.33.0
	google.golang.org/grpc v1.72.0
	google.golang.org/protobuf v1.36.5
	gorm.io/driver/sqlite v1.6.0
	gorm.io/gorm v1.30.0
)

replace github.com/astra-go/astra v0.0.0-00010101000000-000000000000 => ../..

replace github.com/astra-go/astra/auth v0.0.0-00010101000000-000000000000 => ../../auth

replace github.com/astra-go/astra/cache v0.0.0-00010101000000-000000000000 => ../../cache

replace github.com/astra-go/astra/grpc v0.0.0-00010101000000-000000000000 => ../../grpc

replace github.com/astra-go/astra/mq v0.0.0-00010101000000-000000000000 => ../../mq

replace github.com/astra-go/astra/orm v0.0.0-00010101000000-000000000000 => ../../orm

replace github.com/astra-go/astra/otel v0.0.0-00010101000000-000000000000 => ../../otel

replace github.com/astra-go/astra/search v0.0.0-00010101000000-000000000000 => ../../search
