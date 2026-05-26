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

# ─── Help ─────────────────────────────────────────────────────────────────────

.PHONY: help
help: ## List all available make targets
	@grep -E '^[a-zA-Z_-]+:.*##' $(MAKEFILE_LIST) | \
	  awk 'BEGIN {FS = ":.*##"}; {printf "  \033[36m%-28s\033[0m %s\n", $$1, $$2}'
