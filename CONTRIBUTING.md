# Contributing to Astra

Thanks for your interest in contributing to Astra! This guide covers everything you need to get started.

## 📋 Table of Contents

- [Development Environment](#development-environment)
- [Project Structure](#project-structure)
- [Code Standards](#code-standards)
- [Commit Convention](#commit-convention)
- [PR Process](#pr-process)
- [Feedback Channels](#feedback-channels)

## Development Environment

### Prerequisites

- Go 1.22+
- Git
- Docker (for integration tests, optional)
- Make

### Setup

```bash
# Clone the repository
git clone https://github.com/astra-go/astra.git
cd astra

# Install dependencies
go mod download

# Verify build
make build

# Run tests
make test
```

### Running Tests

```bash
# Unit tests
go test ./...

# Integration tests (requires Docker)
go test -tags integration ./...

# Benchmarks
go test -bench=. -benchmem ./benchmarks/...
```

## Project Structure

```
astra/
├── astra.go          # Core framework (App, Context, Router)
├── app.go            # Application lifecycle
├── router/           # Radix-tree router
├── middleware/        # Built-in middleware
├── binding/          # Request binding & validation
├── config/           # Multi-source configuration
├── mq/               # Message queue abstraction
├── cache/            # Cache (LRU + Redis)
├── orm/              # GORM wrapper
├── grpc/             # gRPC stack
├── otel/             # OpenTelemetry
├── examples/         # Example applications
├── docs/             # Documentation
└── cmd/astractl/     # CLI tool
```

## Code Standards

### Go Guidelines

- Follow [Effective Go](https://go.dev/doc/effective_go) conventions
- Run `go fmt ./...` before committing
- Run `go vet ./...` to catch common issues
- Write tests for new features (`go test -cover ./...`)
- Keep public APIs backward-compatible within a major version

### Architecture Constraints

- Run `make check-arch` to verify module count and structure rules
- See `docs/adr/` for Architecture Decision Records

## Commit Convention

We follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <description>

[optional body]

[optional footer]
```

### Types

| Type | Purpose |
|------|---------|
| `feat` | New feature |
| `fix` | Bug fix |
| `docs` | Documentation only |
| `refactor` | Code restructuring (no feature/fix) |
| `perf` | Performance improvement |
| `test` | Test additions or changes |
| `chore` | Build process, dependencies, tooling |
| `ci` | CI/CD changes |

### Scopes

Use the module name: `router`, `middleware`, `config`, `mq`, `orm`, `app`, `context`, `cli`, etc.

### Examples

```
feat(middleware): add tenant quota middleware with QPS limits
fix(router): resolve regex route collision on overlapping patterns
docs(readme): add quickstart guide
perf(context): eliminate mutex overhead in KV store
test(orm): add ClickHouse integration tests
chore(deps): bump gorm to v1.25.0
```

## PR Process

1. **Fork & Branch**: Create a feature branch from `main`
   ```bash
   git checkout -b feat/your-feature main
   ```

2. **Develop & Test**: Make changes, write tests, ensure `make build` and `make test` pass

3. **Commit**: Use conventional commits (see above)

4. **Push & PR**: Push to your fork and open a PR against `main`
   ```bash
   git push origin feat/your-feature
   ```

5. **PR Template**: Fill in the PR description — what changed, why, and how to test

6. **Review**: A maintainer will review. Address feedback and update as needed

7. **Merge**: Squash-merge preferred. Maintainers handle version bumps

## Feedback Channels

We value all kinds of feedback! Choose the channel that fits best:

### 🐛 Bug Reports

Found something broken? File a bug report with reproduction steps:

→ [New Bug Report](https://github.com/astra-go/astra/issues/new?template=bug_report.yml)

### 💡 Feature Requests

Have an idea? We'd love to hear it:

→ [New Feature Request](https://github.com/astra-go/astra/issues/new?template=feature_request.yml)

### ❓ Questions

Stuck on something? Ask the community:

→ [New Question](https://github.com/astra-go/astra/issues/new?template=question.yml)

### 📝 General Feedback

Thoughts on DX, docs, or anything else:

→ [Share Feedback](https://github.com/astra-go/astra/issues/new?template=feedback.yml)

### 💬 Discussions

For open-ended conversations, show-and-tell, or architecture discussions:

→ [GitHub Discussions](https://github.com/astra-go/astra/discussions)

---

## 📄 License

By contributing, you agree that your contributions will be licensed under the Apache License 2.0.
