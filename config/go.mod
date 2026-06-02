module github.com/astra-go/astra/config

// go 1.22.0 — downgraded from 1.25.9.
// Key dep changes:
//   fsnotify v1.9.0 → v1.7.0 (v1.7.x released 2023, requires go 1.17)
//   Other deps (BurntSushi/toml, agollo, nacos-sdk, yaml.v3) are unchanged —
//   all require go 1.21 or earlier.
go 1.25.1

// Standalone config module — YAML/TOML/env file sources with fsnotify hot-reload,
// plus Apollo (agollo) and Nacos remote configuration backends.
require (
	github.com/BurntSushi/toml v1.6.0
	github.com/fsnotify/fsnotify v1.9.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/hashicorp/consul/api v1.28.3
	go.etcd.io/etcd/client/v3 v3.6.10
)

require (
	github.com/armon/go-metrics v0.4.1 // indirect
	github.com/coreos/go-semver v0.3.1 // indirect
	github.com/coreos/go-systemd/v22 v22.5.0 // indirect
	github.com/fatih/color v1.16.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.27.1 // indirect
	github.com/hashicorp/consul/proto-public v0.8.0 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-hclog v1.5.0 // indirect
	github.com/hashicorp/go-immutable-radix v1.3.1 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-rootcerts v1.0.2 // indirect
	github.com/hashicorp/go-version v1.7.0 // indirect
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/hashicorp/serf v0.10.1 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/miekg/dns v1.1.43 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/stretchr/objx v0.5.3 // indirect
	go.etcd.io/etcd/api/v3 v3.6.10 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.6.10 // indirect
	go.opentelemetry.io/otel v1.42.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.42.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/exp v0.0.0-20240719175910-8a7402abbf56 // indirect
	golang.org/x/net v0.51.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.35.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20251202230838-ff82c1b0f217 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251202230838-ff82c1b0f217 // indirect
	google.golang.org/grpc v1.79.3 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

// Downgrade proto-public to v0.6.5 (go 1.25.0) because v0.8.0 requires go >= 1.25.8,
// but the system only has go 1.25.1. The consul/api v1.28.3 module itself only needs go 1.19.
replace github.com/hashicorp/consul/proto-public v0.8.0 => github.com/hashicorp/consul/proto-public v0.6.5

replace github.com/astra-go/astra v0.0.0-00010101000000-000000000000 => ..
