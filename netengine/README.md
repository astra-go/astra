# NetEngine — Network Engine

High-performance network engine implementation providing custom transport layer capabilities.

## Features

- **Custom Transport Layer**: Supports replacing standard `net/http` transport layer
- **High-Performance HTTP Server**: High-concurrency processing based on custom network stack
- **Protocol Extension**: Supports custom protocol encoding/decoding

## Module Dependencies

This is an internal implementation module with few direct use cases. Core functionality is encapsulated by Astra main framework.

## Use Cases

Typically configured indirectly via `astra.New()`, no direct import needed:

```go
app := astra.New(
    astra.WithEngine(netengine.New(...)),
)
```
