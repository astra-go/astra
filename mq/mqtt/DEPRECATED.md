# ⚠️ DEPRECATED: This sub-module is deprecated

**Status**: Deprecated as of 2026-06-02  
**Transition Period**: 3 months (until 2026-09-02)  
**Alternative**: Use `github.com/astra-go/astra/mq` (unified module)

---

## Migration Notice

This sub-module (`mq/mqtt`) is being phased out in favor of a unified `mq` module.

**Old import** (v1.x - Deprecated):
```go
import "github.com/astra-go/astra/mq/mqtt"

p, err := mqtt.NewProducer(mqtt.ProducerConfig{
    Brokers: []string{"localhost:9092"},
})
```

**New import** (v2.x - Recommended):
```go
import "github.com/astra-go/astra/mq"

// Option 1: Use type-safe constructor (recommended)
p, err := mq.NewKafkaProducer(mq.KafkaProducerConfig{
    Brokers: []string{"localhost:9092"},
})

// Option 2: Use string-based factory (convenient)
p, err := mq.NewProducer("mqtt", mq.ProducerOptions{
    Brokers: []string{"localhost:9092"},
})
```

## Why is this deprecated?

To improve maintainability and reduce complexity:
- **Fewer modules to manage**: 7 MQ sub-modules → 1 unified module
- **Simpler versioning**: 1 Git tag per release instead of 7
- **Faster CI**: Reduced test matrix and build time
- **Better DX**: Unified documentation and examples

## Timeline

| Date | Status |
|------|--------|
| 2026-06-02 | Marked as deprecated |
| 2026-06-02 - 2026-09-02 | Transition period (3 months) |
| 2026-09-02 | End of support |

During the transition period:
- ✅ Security fixes will be backported
- ⚠️ No new features
- ⚠️ Bug fixes on a best-effort basis

## Migration Guide

See [Migration Guide](../../docs/migration-guide-mq-v2.md) for:
- Step-by-step migration instructions
- Automated migration script
- API compatibility matrix
- Troubleshooting common issues

## Questions?

- GitHub Issues: https://github.com/astra-go/astra/issues
- Discussions: https://github.com/astra-go/astra/discussions
- Migration FAQ: [docs/migration-guide-mq-v2.md](../../docs/migration-guide-mq-v2.md)

---

**For current documentation, see**: [github.com/astra-go/astra/mq](https://pkg.go.dev/github.com/astra-go/astra/mq)