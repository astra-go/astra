# Contributing to Showcase

Thanks for taking the time to contribute. This document covers the development
workflow, coding conventions, and review checklist for the Showcase reference
application.

## Getting started

```bash
# 1. Fork and clone the monorepo
git clone https://github.com/astra-go/astra.git
cd astra/examples/showcase

# 2. Start infrastructure
make dev

# 3. Run the API server
make run-api
```

## Development workflow

```
feature branch → PR → review → squash merge to main
```

- Branch from `main`. Name branches `feat/<topic>`, `fix/<topic>`, or `docs/<topic>`.
- Keep PRs focused. One logical change per PR.
- All CI checks must pass before merge.

## Code conventions

- Follow the existing package layout: `domain` → `repository` → `service` → `handler`.
- Handlers are thin: bind input, call service, render output. No business logic.
- Repositories are tenant-scoped at construction time — never pass `tenant_id` as a query argument.
- Errors are wrapped with `fmt.Errorf("context: %w", err)` and mapped to HTTP status codes in `service/errors.go`.
- No comments unless the *why* is non-obvious. Well-named identifiers are self-documenting.

## Testing

```bash
# Unit tests (no external deps)
make test

# Integration tests (requires Docker)
make test-integration

# Race detector
go test -race ./...
```

New features require:
- Unit tests covering the happy path and at least one error path.
- Integration tests for any new repository method.

## Performance testing

```bash
# Install k6: https://k6.io/docs/get-started/installation/

# Smoke test (1 VU, 30 s)
make bench-smoke

# Load test (ramp to 50 VU)
make bench

# Stress test (ramp to 200 VU)
make bench-stress
```

Results are written to `perf/results.json`.

Thresholds (enforced by k6):
- `http_req_duration` p95 < 500 ms
- `order_create_duration` p95 < 1 s
- Error rate < 1 %

## Pull request checklist

- [ ] `go build ./...` passes
- [ ] `go test ./...` passes (no `-tags integration` required for basic CI)
- [ ] `go vet ./...` clean
- [ ] New public types/functions have a one-line doc comment
- [ ] README / API docs updated if endpoints or config changed
- [ ] No secrets or credentials committed

## Reporting issues

Open a GitHub issue with:
1. What you expected to happen
2. What actually happened
3. Steps to reproduce (minimal reproduction preferred)
4. Go version and OS
