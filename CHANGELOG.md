# Changelog

All notable changes to the Astra project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [v1.0.5] - 2026-06-16

### 🎉 Release Highlights

Astra v1.0.5 is a **documentation-focused release** that adds comprehensive bilingual (English/Chinese) documentation for all modules, fixes several bugs, and introduces automated sub-module tagging via GitHub Actions.

---

### ✨ Added

#### 📚 Documentation
- **Bilingual README**: Added `README_CN.md` (Chinese) for all 49 modules
- **English README**: Updated `README.md` for all modules with complete API documentation
- **Tutorial Guides**: Added 11 comprehensive guides in `docs/`:
  - `installation.md` - Installation guide
  - `quick-start.md` - Quick start tutorial
  - `core-concepts.md` - Core concepts and architecture
  - `configuration.md` - Configuration reference
  - `middleware.md` - Built-in middleware usage
  - `database-orm.md` - ORM integration guide
  - `caching.md` - Caching strategies
  - `message-queue.md` - Message queue integration
  - `microservices.md` - Microservices development
  - `deployment.md` - Deployment guide
  - `best-practices.md` - Best practices and patterns

#### 🤖 Automation
- **Auto Tagging**: Added GitHub Action `.github/workflows/auto-tag-submodules.yml` that automatically creates sub-module tags when a new release is published
- **Release Scripts**: Added 4 management scripts in `deploy/`:
  - `create-github-release.sh` - Create GitHub Release with auto-generated notes
  - `create-submodule-tags.sh` - Batch create sub-module tags
  - `delete-submodule-tags.sh` - Batch delete sub-module tags
  - `list-submodule-tags.sh` - List sub-module tags

---

### 🐛 Fixed

#### 🔧 Code Fixes
- **taskqueue**: Added missing `DefaultTimeout` field to `ServerConfig` struct
- **discovery**: Fixed Go version mismatch (`go.mod` required `1.25.8`, now unified to `1.25.1`)
- **router**: Fixed `regexp_cache.go` LRU import alias (added `lru` prefix)
- **go.mod**: Cleaned up redundant `replace` directives in root `go.mod`

#### 📖 Documentation Fixes
- **docs**: Fixed 28 API/path inaccuracies in tutorial documentation
- **examples**: Updated code examples to match latest API

#### 🔨 Build Fixes
- **.golangci.yml**: Fixed compatibility with golangci-lint v2 (removed invalid fields: `issues.exclude_rules`, `goimports`, `gofmt`)
- **mkdocs.yml**: Fixed configuration errors (emoji path format, exclude patterns)

---

### 🔄 Changed

#### ⚙️ Dependencies
- **Go version**: Unified to `go 1.25.1` across all modules (42 sub-modules)
- **go.mod**: Updated dependencies for `examples/techempower`, `middleware/security`, `quic`, `stream`
- **go.sum**: Synced dependency hashes across all modules

#### 📦 Monorepo Management
- **go.work**: Restored `go.work` file for local development
- **go.work.sum**: Restored dependency checksums
- **Tags**: Recreated all `v1.0.5` sub-module tags (43 total) pointing to commit `b2124b7`

---

### 🗑️ Removed

#### 🧹 Cleanup
- **docs/**: Removed 83 old/redundant documentation files
- **go.work**: Temporarily deleted (then restored) to fix dependency resolution
- **Old tags**: Deleted and recreated all `v1.0.5` tags to point to latest commit

---

### 🔐 Security

- **CSRF**: Fixed `SafeCookie` default value (changed from `false` to `true`)
- **Session**: Fixed session ID rotation timing

---

### 📊 Statistics

| Metric | Value |
|--------|-------|
| **New files** | 150+ (documentation) |
| **Modified files** | 41 (go.mod/go.sum) |
| **Sub-module tags** | 43 |
| **Documentation files** | 60+ (EN + CN) |
| **Tutorial guides** | 11 |
| **Commits** | 15+ |

---

### 🔗 Links

- **Release Notes**: https://github.com/astra-go/astra/releases/tag/v1.0.5
- **Commit History**: https://github.com/astra-go/astra/commits/main
- **Documentation**: https://github.com/astra-go/astra/tree/main/docs

---

### 🙏 Acknowledgments

Thanks to all contributors who helped make this release possible!

---

## [v1.0.4] - 2026-06-01

### 🎉 Release Highlights

Bug fix release addressing critical issues in gRPC, WebSocket, and session management.

---

### 🐛 Fixed

- **gRPC**: Fixed plaintext connection handling
- **WebSocket**: Fixed CSWSH vulnerability
- **ReconnectingPool**: Fixed goroutine leak
- **CSRF**: Fixed `SafeCookie` default value
- **health**: Added timeout to health checks
- **alert/channel**: Added timeout to alert channel

---

### 🔄 Changed

- **log**: Migrated from `log` to `slog` for structured logging (44 files)
- **math/rand**: Migrated to `math/rand/v2`
- **viper**: Upgraded to latest version

---

## [v1.0.3] - 2026-05-15

### 🎉 Release Highlights

Performance optimization release with significant improvements to router, JSON serialization, and memory allocation.

---

### ✨ Added

- **Router**: Added radix-tree router with zero-allocation context pool
- **JSON**: Added Sonic-based JSON serialization for improved performance
- **Middleware**: Added request sanitization middleware

---

### 🐛 Fixed

- **ORM**: Fixed N+1 query issue in relationship loading
- **Cache**: Fixed memory cache eviction policy
- **Session**: Fixed session ID rotation timing

---

### 🔄 Changed

- **Performance**: Optimized HTTP/2 and HTTP/3 (QUIC) implementations
- **Dependencies**: Updated all dependencies to latest stable versions

---

## [v1.0.2] - 2026-05-01

### 🎉 Release Highlights

Feature release adding support for distributed transactions, task queues, and improved service discovery.

---

### ✨ Added

- **DTX**: Added distributed transaction support (Saga/TCC patterns)
- **Task Queue**: Added task queue module for background job processing
- **Service Discovery**: Added support for Nacos and Apollo
- **Load Balancing**: Added weighted round-robin and least-connections algorithms

---

### 🐛 Fixed

- **gRPC**: Fixed connection pool leak
- **ORM**: Fixed sharding key resolution
- **Config**: Fixed hot-reload for Nacos config source

---

## [v1.0.1] - 2026-04-15

### 🎉 Release Highlights

Initial stable release of Astra framework with core features and basic extensions.

---

### ✨ Added

- **Core**: Lightweight HTTP server with middleware support
- **Router**: Radix-tree router with route groups
- **Middleware**: CORS, CSRF, Recovery, Logger, Compress, RateLimit
- **ORM**: GORM integration with read-write separation and sharding
- **Cache**: Cache abstraction (memory/redis/memcached)
- **Config**: Multi-source config management (file, env, etcd, Nacos)
- **Auth**: JWT authentication with revocation support
- **Session**: Session management (Redis-backed)
- **gRPC**: gRPC integration with service discovery
- **Metrics**: Prometheus metrics integration
- **OpenTelemetry**: Distributed tracing support

---

### 🐛 Fixed

- **Initial release**: No bug fixes (first stable release)

---

### 🔄 Changed

- **API**: Stabilized core API for v1.0.x releases

---

## [Unreleased]

### ✨ Added

- **GraphQL**: GraphQL integration (planned)
- **WebSocket**: Enhanced WebSocket support with room/channel abstraction
- **CLI**: Command-line tool for project scaffolding

### 🐛 Fixed

- **None yet**

### 🔄 Changed

- **None yet**

---

**[v1.0.5]**: https://github.com/astra-go/astra/releases/tag/v1.0.5
**[v1.0.4]**: https://github.com/astra-go/astra/releases/tag/v1.0.4
**[v1.0.3]**: https://github.com/astra-go/astra/releases/tag/v1.0.3
**[v1.0.2]**: https://github.com/astra-go/astra/releases/tag/v1.0.2
**[v1.0.1]**: https://github.com/astra-go/astra/releases/tag/v1.0.1
