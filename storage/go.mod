module github.com/astra-go/astra/storage

// go 1.22.0 — downgraded from 1.25.9.
// Key dep changes:
//   aws-sdk-go-v2  v1.41 → v1.30.3   (v1.36+ requires go 1.23; v1.30.x requires go 1.21)
//   aws credentials v1.19 → v1.17.27  (aligned with aws-sdk v1.30.x)
//   aws s3          v1.99 → v1.58.3   (aligned with aws-sdk v1.30.x)
go 1.25.0

// Standalone object-storage module (S3 / Aliyun OSS / Tencent COS).
// Upgrade cloud SDKs independently of the router or ORM layer.
require (
	github.com/aliyun/aliyun-oss-go-sdk v3.0.2+incompatible
	github.com/aws/aws-sdk-go-v2 v1.36.3
	github.com/aws/aws-sdk-go-v2/credentials v1.17.62
	github.com/aws/aws-sdk-go-v2/service/s3 v1.58.3
	github.com/tencentyun/cos-go-sdk-v5 v0.7.73
)

require (
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.6.3 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.3.34 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.6.34 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.3.15 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.12.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.3.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.12.15 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.17.15 // indirect
	github.com/aws/smithy-go v1.22.2 // indirect
	github.com/clbanning/mxj v1.8.4 // indirect
	github.com/google/go-querystring v1.0.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/mozillazg/go-httpheader v0.2.1 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	golang.org/x/time v0.11.0 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
)
