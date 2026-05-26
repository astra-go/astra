# Contributing Guide

Thank you for considering contributing to Astra!

## Development Environment

```bash
git clone https://github.com/astra-go/astra.git
cd astra
go mod download

# Run all tests
go test ./... -race

# Run vet + staticcheck
go vet ./...
staticcheck ./...
```

## Commit Convention

Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat(middleware): add CSRF double-submit cookie support
fix(netengine): drain addCh on shutdown to avoid conn leak
docs(migration): add v0-to-v1 migration guide
test(alert): add For duration delay test
```

| Prefix | Purpose |
|--------|---------|
| `feat` | New feature |
| `fix` | Bug fix |
| `docs` | Documentation changes |
| `test` | Test-related |
| `refactor` | Refactor (no behavior change) |
| `perf` | Performance improvement |
| `chore` | Build / CI related |

## Pull Request Process

1. Fork the repository and create a feature branch `feat/your-feature`
2. Write code + tests (coverage must not drop below the current level)
3. Confirm `go test ./... -race` passes
4. Update the `[Unreleased]` section of `CHANGELOG.md`
5. Submit a PR and fill in the template describing the reason for the change

## Documentation Updates

```bash
pip install mkdocs-material

# Local preview
mkdocs serve

# Build static site
mkdocs build
```
