# ─── Workspace / module maintenance ──────────────────────────────────────────

MAGE := mage -d magefiles

.PHONY: sync-replaces
sync-replaces: ## Sync intra-workspace replace directives across all go.mod files
	$(MAGE) syncReplaces

.PHONY: check-replaces
check-replaces: ## CI check: fail if any go.mod has missing/stale intra-workspace replace directives
	$(MAGE) checkReplaces

.PHONY: tidy
tidy: ## Run go mod tidy across all workspace modules in topological order
	$(MAGE) tidy

.PHONY: affected-modules
affected-modules: ## Print modules affected by current branch (BASE=ref, ALL=1 for all)
	$(MAGE) affectedModules

.PHONY: check-dep-versions
check-dep-versions: ## Detect version splits across workspace modules
	$(MAGE) checkDepVersions

.PHONY: check-go-versions
check-go-versions: ## CI check: fail if any go.mod declares a Go version below the core minimum
	$(MAGE) checkGoVersions

.PHONY: check-test-deps
check-test-deps: ## Detect test-only packages declared as production deps
	$(MAGE) checkTestDeps

.PHONY: check-arch
check-arch: ## CI check: enforce architecture fitness rules (ADR-001 core deps)
	$(MAGE) checkCoreDeps

.PHONY: check-module-count
check-module-count: ## CI check: enforce module count limit (ADR-005)
	$(MAGE) checkModuleCount

.PHONY: install-hooks
install-hooks: ## Install git pre-commit hook
	$(MAGE) installHooks

.PHONY: release
release: ## Create lockstep version tags (VERSION=vX.Y.Z required; DRY_RUN=1, PUSH=1 optional)
	$(MAGE) release

# ─── Benchmark targets ────────────────────────────────────────────────────────
#
# Prerequisites: Go 1.22+, optional: golang.org/x/perf/cmd/benchstat
#
#   go install golang.org/x/perf/cmd/benchstat@latest
#
# Usage examples:
#   make bench              # run core benchmarks once (quick sanity check)
#   make bench-quick        # single fast pass (count=1, benchtime=1s)
#   make bench-all          # run every benchmark suite
#   make bench-router       # router micro-benchmarks only
#   make bench-netengine    # reactor + worker-pool benchmarks
#   make bench-middleware   # per-middleware benchmarks
#   make bench-integration  # full-stack integration suite
#   make bench-compare      # record a baseline then diff against current
#   make bench-publish-local# generate HTML comparison report locally

BENCH_COUNT  ?= 5
BENCH_TIME   ?= 2s
BENCH_FLAGS  := -benchmem -count=$(BENCH_COUNT) -benchtime=$(BENCH_TIME)

# ─── Individual suites ───────────────────────────────────────────────────────

.PHONY: bench
bench: ## Core package: routing + middleware chain + context + ServeHTTP
	go test $(BENCH_FLAGS) -bench=. .

.PHONY: bench-quick
bench-quick: ## Single fast pass (count=1, benchtime=1s) for development feedback
	go test -benchmem -count=1 -benchtime=1s -bench=. . ./netengine/ ./middleware/ ./benchmarks/

.PHONY: bench-router
bench-router: ## Router lookup micro-benchmarks (static / param / regex / wildcard / miss)
	go test $(BENCH_FLAGS) -bench=BenchmarkRouter .

.PHONY: bench-middleware-chain
bench-middleware-chain: ## Middleware chain cost (0 → 10 handlers, abort path)
	go test $(BENCH_FLAGS) -bench=BenchmarkMiddlewareChain .

.PHONY: bench-context
bench-context: ## Context response writing (JSON small/medium/large, String, QueryParams)
	go test $(BENCH_FLAGS) -bench=BenchmarkContext .

.PHONY: bench-netengine
bench-netengine: ## Reactor engine: worker pool + HTTP round-trip (keep-alive, new-conn, parallel)
	go test $(BENCH_FLAGS) -bench=. ./netengine/

.PHONY: bench-middleware
bench-middleware: ## Individual middleware: CORS, Recovery, JWT (valid / missing / bad sig)
	go test $(BENCH_FLAGS) -bench=. ./middleware/

.PHONY: bench-integration
bench-integration: ## Full-stack integration suite (static, param, POST+bind, 3MW, 5MW+JWT, parallel)
	go test $(BENCH_FLAGS) -bench=. ./benchmarks/

# ─── Run all suites sequentially ─────────────────────────────────────────────

.PHONY: bench-all
bench-all: ## Run every benchmark suite and print a combined report
	@printf '\n\033[1m=== Core (routing · middleware chain · context · ServeHTTP) ===\033[0m\n\n'
	go test $(BENCH_FLAGS) -bench=. .
	@printf '\n\033[1m=== netengine (worker pool · reactor HTTP round-trip) ===\033[0m\n\n'
	go test $(BENCH_FLAGS) -bench=. ./netengine/
	@printf '\n\033[1m=== middleware (CORS · Recovery · JWT) ===\033[0m\n\n'
	go test $(BENCH_FLAGS) -bench=. ./middleware/
	@printf '\n\033[1m=== Integration (full-stack end-to-end) ===\033[0m\n\n'
	go test $(BENCH_FLAGS) -bench=. ./benchmarks/

# ─── Baseline / comparison workflow ──────────────────────────────────────────
#
# 1. Record baseline on main branch:   make bench-save-baseline
# 2. Apply changes, then:              make bench-compare
#
# Requires: go install golang.org/x/perf/cmd/benchstat@latest

BASELINE_FILE ?= .bench-baseline.txt
CURRENT_FILE  ?= .bench-current.txt

.PHONY: bench-save-baseline
bench-save-baseline: ## Record current numbers as the comparison baseline
	go test $(BENCH_FLAGS) -bench=. . ./netengine/ ./middleware/ ./benchmarks/ \
	  2>/dev/null | tee $(BASELINE_FILE)
	@echo "Baseline saved to $(BASELINE_FILE)"

.PHONY: bench-compare
bench-compare: ## Compare current numbers against the saved baseline (requires benchstat)
	go test $(BENCH_FLAGS) -bench=. . ./netengine/ ./middleware/ ./benchmarks/ \
	  2>/dev/null | tee $(CURRENT_FILE)
	@command -v benchstat >/dev/null 2>&1 || \
	  { echo "benchstat not found — run: go install golang.org/x/perf/cmd/benchstat@latest"; exit 1; }
	benchstat $(BASELINE_FILE) $(CURRENT_FILE)

# ─── CPU / memory profiling ──────────────────────────────────────────────────

.PHONY: bench-profile-cpu
bench-profile-cpu: ## CPU profile for the integration suite (opens pprof in browser)
	go test -bench=BenchmarkIntegration_Parallel_JSON_3MW \
	  -benchtime=5s -count=1 -cpuprofile=cpu.prof ./benchmarks/
	go tool pprof -http=:6060 cpu.prof

.PHONY: bench-profile-mem
bench-profile-mem: ## Memory profile for the integration suite
	go test -bench=BenchmarkIntegration_Parallel_JSON_3MW \
	  -benchtime=5s -count=1 -memprofile=mem.prof ./benchmarks/
	go tool pprof -http=:6061 mem.prof

.PHONY: bench-profile-cpu-core
bench-profile-cpu-core: ## CPU profile for core hot paths: ServeHTTP + router + JSON response
	go test -bench='BenchmarkServeHTTP_Parallel_JSON|BenchmarkRouter_Param|BenchmarkContext_JSON_Medium' \
	  -benchtime=10s -count=1 -cpuprofile=cpu-core.prof .
	go tool pprof -http=:6060 cpu-core.prof

.PHONY: bench-profile-mem-core
bench-profile-mem-core: ## Memory profile for core hot paths (full allocation sampling)
	go test -bench='BenchmarkServeHTTP_Parallel_JSON|BenchmarkContext_JSON_Medium|BenchmarkRouter_Param' \
	  -benchtime=10s -count=1 -memprofile=mem-core.prof -memprofilerate=1 .
	go tool pprof -http=:6061 mem-core.prof

.PHONY: bench-trace
bench-trace: ## Goroutine/GC/sync.Pool trace for core ServeHTTP path
	go test -bench=BenchmarkServeHTTP_Parallel_JSON \
	  -benchtime=5s -count=1 -trace=trace.out .
	go tool trace trace.out

# ─── Local HTML comparison report ────────────────────────────────────────────

.PHONY: bench-publish-local
bench-publish-local: ## Generate HTML comparison report locally → site/benchmarks/index.html
	@mkdir -p site/benchmarks
	go test -run='^$$' -bench='^BenchmarkVs_' -benchmem \
	  -count=6 -benchtime=2s ./benchmarks/ 2>/dev/null | tee bench-vs-local.txt
	python3 scripts/bench-to-html.py \
	  --badges \
	  --platform "$$(uname -m) · $$(uname -s) · local" \
	  bench-vs-local.txt site/benchmarks/index.html
	@echo "Report written to site/benchmarks/index.html"

# ─── Code quality ─────────────────────────────────────────────────────────────

.PHONY: lint
lint: ## Run golangci-lint on core module (new issues only)
	golangci-lint run --new-from-rev=HEAD~1 --timeout=5m ./...

.PHONY: lint-all
lint-all: ## Run golangci-lint on core module (all issues)
	golangci-lint run --timeout=5m ./...

.PHONY: lint-fix
lint-fix: ## Run golangci-lint with auto-fix
	golangci-lint run --fix --timeout=5m ./...

.PHONY: vuln
vuln: ## Run govulncheck on core module
	govulncheck ./...

# ─── Help ─────────────────────────────────────────────────────────────────────

.PHONY: help
help: ## List all available make targets
	@grep -E '^[a-zA-Z_-]+:.*##' $(MAKEFILE_LIST) | \
	  awk 'BEGIN {FS = ":.*##"}; {printf "  \033[36m%-28s\033[0m %s\n", $$1, $$2}'
