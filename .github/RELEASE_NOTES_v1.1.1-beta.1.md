## 🚀 v1.0.5-beta.1 — Preview Release

We're excited to share this preview release with the community! This version includes enterprise-grade features and a reference application for easier onboarding.

### ✨ What's New

- **Reference Application** (`examples/reference-blog`): A complete blog platform demonstrating all Astra features — API server, comment service, async worker, with integration tests and benchmarks
- **Tenant Quota Middleware** (`middleware/security/`): Per-tenant QPS, concurrency, and daily request limits with Prometheus metrics — production-ready multi-tenancy enforcement
- **Config Hot-Reload Adapters** (`config/`): Nacos and Apollo Watchable source implementations + `WatchKey` for granular configuration hot-reload

### 📋 Known Limitations

- `middleware/security/` requires `go get github.com/prometheus/client_golang` before building
- Reference application integration tests require Docker (PostgreSQL, Kafka, Elasticsearch)
- This is a **pre-release** — APIs may change before stable

### 🤝 We Need Your Feedback!

This is a preview release specifically to gather community feedback. Please:
- 🐛 [Report bugs](https://github.com/astra-go/astra/issues/new?template=bug_report.yml)
- 💡 [Suggest features](https://github.com/astra-go/astra/issues/new?template=feature_request.yml)
- 💬 [Ask questions](https://github.com/astra-go/astra/issues/new?template=question.yml)
- 📝 [Share feedback](https://github.com/astra-go/astra/issues/new?template=feedback.yml)
- 💬 [Start a discussion](https://github.com/astra-go/astra/discussions)

**Full Changelog**: https://github.com/astra-go/astra/compare/v1.0.4...v1.0.5-beta.1
