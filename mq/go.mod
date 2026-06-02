module github.com/astra-go/astra/mq

// go 1.22.0 — downgraded from 1.25.9.
// Key dep changes (version → last go-1.22-compatible release):
//   nats.go      v1.51  → v1.37.0  (v1.38+ requires go 1.23)
//   pulsar       v0.19  → v0.12.1  (v0.13+ requires go 1.22 but v0.19 requires go 1.24)
//   rocketmq/v5  v5.1.3 → v5.0.1   (v5.1+ requires go 1.24)
//   franz-go     v1.20  → v1.17.1  (v1.18.0 bumped to go 1.22; v1.20 requires go 1.24)
//   paho         v1.5.1 → v1.4.3   (v1.5.x requires go 1.24)
go 1.25.1








































