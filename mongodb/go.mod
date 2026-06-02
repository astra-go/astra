module github.com/astra-go/astra/mongodb

// go 1.22.0 — downgraded from 1.25.9.
// Key dep changes:
//   mongo-driver/v2 v2.5.1 → v2.0.0 (v2.0.0 released 2024-04, requires go 1.22)
go 1.25.1

// Standalone MongoDB module — upgrade mongo-driver independently of ORM or MQ.
require go.mongodb.org/mongo-driver/v2 v2.5.1

require (
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/klauspost/compress v1.18.6 // indirect
	github.com/xdg-go/pbkdf2 v1.0.0 // indirect
	github.com/xdg-go/scram v1.2.0 // indirect
	github.com/xdg-go/stringprep v1.0.4 // indirect
	github.com/youmark/pkcs8 v0.0.0-20240726163527-a2c0da244d78 // indirect
	golang.org/x/crypto v0.52.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/text v0.37.0 // indirect
)
