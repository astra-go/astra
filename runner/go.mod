module github.com/astra-go/astra/runner

// go 1.22.0 — downgraded from 1.25.9.
go 1.25.1

// Standalone task-runner module — unified scheduler interface with four backends.
require (
	github.com/astra-go/astra v1.0.2
	github.com/astra-go/astra/taskqueue v1.0.2
	github.com/go-co-op/gocron/v2 v2.11.0
)

require (
	github.com/google/uuid v1.6.0 // indirect
	github.com/jonboulle/clockwork v0.4.0 // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	golang.org/x/exp v0.0.0-20240719175910-8a7402abbf56 // indirect
)
