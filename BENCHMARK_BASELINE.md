# Astra Performance Baseline

Generated: 2026-07-12
Platform: Apple M4 (arm64), Darwin
Go: 1.26

## DI (Dependency Injection)

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|-----------|
| NewSet | ~100 | 160 | 4 |
| RegisterSets | ~7500 | 1248 | 17 |
| Invoke | ~2700 | 168 | 4 |
| InvokeParallel | ~3850 | 168 | 4 |

**Key Insights:**
- NewSet: ~100ns, cheap to create
- RegisterSets: ~7.5µs for 2 sets × 2 providers each
- Invoke: ~2.7µs per dependency resolution (singleton cache)
- Parallel safe: contention adds ~1µs under parallel load

## Middleware

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|-----------|
| CORS_Passthrough | ~181 | 280 | 8 |
| CORS_CrossOrigin | ~426 | 1016 | 12 |
| CORS_Preflight | — | — | — |
| Recovery_NoPanic | — | — | — |
| Recovery_Panic | — | — | — |
| JWT_ValidToken | — | — | — |
| JWT_CacheHit | — | — | — |

**Key Insights:**
- CORS passthrough: ~181ns, 280B (non-CORS request)
- CORS cross-origin: ~426ns, 1KB (adds headers)

## Serializer (Sonic)

See `serializer_sonic_test.go` for detailed benchmarks.

Pool reuse vs no-pool:
- EncodeInto with pool: ~30% faster than no-pool for repeated small JSON
- EncodeStream with pool: significant speedup for streaming workloads

## Recommendations

1. **DI**: Singleton resolution is fast (~2.7µs). For hot paths, consider caching resolved deps in local vars if invoked millions of times.
2. **Middleware**: CORS adds ~400ns overhead for cross-origin requests. Acceptable for most APIs.
3. **Serializer**: Always use pooled encoders (`EncodeInto`/`EncodeStream`) for high-throughput JSON serialization.

## Running Benchmarks

```bash
# DI
cd astra/di && go test -bench=. -benchmem

# Middleware
cd astra/middleware && go test -bench=. -benchmem

# Serializer
cd astra && go test -run=^$ -bench=Sonic -benchmem
```
