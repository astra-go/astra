# Performance Benchmark Results

This directory contains performance benchmark scripts and results for the Astra Showcase application.

## Quick Start

### Prerequisites

```bash
# macOS
brew install wrk

# Ubuntu/Debian
apt-get install wrk

# Install ghz for gRPC benchmarks (optional)
go install github.com/bojand/ghz/cmd/ghz@latest
```

### Running Benchmarks

```bash
# 1. Start services
cd ../
docker-compose up -d
go run ./cmd/api &
go run ./cmd/grpc &

# 2. Generate JWT token (for authenticated endpoints)
export JWT_TOKEN=$(go run ./tools/gentoken/main.go)

# 3. Run benchmarks
cd perf/
chmod +x benchmark.sh
./benchmark.sh
```

### Results

Results are saved to `perf/results/` with timestamps:
- `01_health_<timestamp>.txt` - Health check (empty route)
- `02_products_<timestamp>.txt` - Product list (cached)
- `03_orders_<timestamp>.txt` - Order creation (write)
- `04_grpc_stock_<timestamp>.txt` - gRPC stock query
- `05_grpc_batch_<timestamp>.txt` - gRPC batch query
- `BENCHMARK_<timestamp>.md` - Summary report

### Parse Results

```bash
go run parse_results.go results/
```

## Benchmark Targets

| Scenario | Target QPS | Target P99 Latency | Description |
|----------|-----------|-------------------|-------------|
| **Health Check** | 10,000+ | < 1ms | Empty route baseline |
| **Product List** | 5,000+ | < 5ms | Redis cache hit |
| **Order Creation** | 1,000+ | < 20ms | DB write + stock decrement |
| **gRPC Stock** | 3,000+ | < 5ms | gRPC unary call |
| **gRPC Batch** | 2,000+ | < 10ms | gRPC batch query |

## k6 Load Test

For more comprehensive load testing scenarios:

```bash
# Install k6
brew install k6  # macOS
# or download from https://k6.io/docs/getting-started/installation/

# Smoke test (sanity check)
k6 run --env SCENARIO=smoke --env JWT=$JWT_TOKEN load_test.js

# Load test (default - ramp to 50 VUs)
k6 run --env JWT=$JWT_TOKEN load_test.js

# Stress test (ramp to 200 VUs)
k6 run --env SCENARIO=stress --env JWT=$JWT_TOKEN load_test.js
```

### k6 Thresholds

- HTTP request duration P95 < 500ms
- Error rate < 1%
- Order creation P95 < 1000ms

## Continuous Integration

Add to `.github/workflows/performance.yml`:

```yaml
name: Performance Regression Test

on:
  pull_request:
    branches: [main]
  push:
    branches: [main]

jobs:
  benchmark:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.25'
      
      - name: Install wrk
        run: sudo apt-get install -y wrk
      
      - name: Start services
        run: |
          cd examples/showcase
          docker-compose up -d
          go run ./cmd/api &
          sleep 10
      
      - name: Run benchmarks
        run: |
          cd examples/showcase/perf
          export JWT_TOKEN=$(go run ../tools/gentoken/main.go)
          ./benchmark.sh
      
      - name: Parse results
        run: |
          cd examples/showcase/perf
          go run parse_results.go results/
      
      - name: Upload results
        uses: actions/upload-artifact@v3
        with:
          name: benchmark-results
          path: examples/showcase/perf/results/
```

## Interpreting Results

### wrk Output

```
Running 30s test @ http://localhost:8080/health
  4 threads and 100 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency     1.23ms    2.45ms  50.12ms   89.45%
    Req/Sec     2.50k   450.00    3.10k    75.00%
  Latency Distribution
     50%    0.95ms
     75%    1.20ms
     90%    2.10ms
     99%    5.80ms
  299456 requests in 30.01s, 45.67MB read
Requests/sec:   9978.52
Transfer/sec:      1.52MB
```

**Key Metrics**:
- **Requests/sec**: Total throughput (QPS)
- **Latency P99**: 99% of requests completed within this time
- **Latency Avg**: Average response time
- **Transfer/sec**: Network bandwidth used

### ghz Output (gRPC)

```
Summary:
  Count:        100000
  Total:        33.12 s
  Slowest:      25.34 ms
  Fastest:      0.15 ms
  Average:      1.65 ms
  Requests/sec: 3019.32

Latency distribution:
  10% in 0.89 ms
  25% in 1.12 ms
  50% in 1.45 ms
  75% in 1.89 ms
  90% in 2.45 ms
  95% in 3.12 ms
  99% in 4.89 ms
```

**Key Metrics**:
- **Requests/sec**: gRPC QPS
- **Latency P99**: 99th percentile
- **Error rate**: Success ratio

## Troubleshooting

### Low QPS

1. **Check CPU usage**: `htop` or `Activity Monitor`
2. **Check DB connections**: `SELECT count(*) FROM pg_stat_activity;`
3. **Check Redis**: `redis-cli INFO stats`
4. **Enable pprof**: `go tool pprof http://localhost:8080/debug/pprof/profile`

### High P99 Latency

1. **GC pressure**: Check `GODEBUG=gctrace=1`
2. **DB slow queries**: Enable PostgreSQL slow query log
3. **Network latency**: Use local Docker instead of remote
4. **Lock contention**: Check for mutex hotspots with pprof

### Common Issues

**"Service not available"**
```bash
# Check if services are running
docker-compose ps
curl http://localhost:8080/health

# Check logs
docker-compose logs postgres
go run ./cmd/api
```

**"ghz: command not found"**
```bash
go install github.com/bojand/ghz/cmd/ghz@latest
export PATH=$PATH:$(go env GOPATH)/bin
```

**"JWT token invalid"**
```bash
# Regenerate token
export JWT_TOKEN=$(go run ./tools/gentoken/main.go)
echo $JWT_TOKEN  # Verify it's not empty
```

## Best Practices

1. **Warm-up**: Run a few requests before benchmarking to warm up caches
2. **Stable environment**: Close unnecessary applications during benchmarks
3. **Multiple runs**: Run benchmarks 3-5 times and take the median
4. **Realistic data**: Seed database with production-like data volume
5. **Monitor resources**: Watch CPU, memory, and network during tests

## References

- [wrk Documentation](https://github.com/wg/wrk)
- [ghz Documentation](https://ghz.sh/)
- [k6 Documentation](https://k6.io/docs/)
- [Grafana k6 Cloud](https://k6.io/cloud/)
