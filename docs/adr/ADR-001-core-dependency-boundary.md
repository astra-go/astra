# ADR-001: Core Module Dependency Boundary

## Status
Proposed

## Date
2026-05-26

## Context
The core `github.com/astra-go/astra` module currently directly requires several sub-modules with heavy third-party dependencies:
- `middleware/security` → pulls in `golang-jwt/jwt`, `go-redis`
- `middleware/observability` → pulls in OpenTelemetry SDK, Prometheus client
- `discovery` → pulls in service discovery logic
- `quic-go` → HTTP/3 support, ~40 transitive dependencies
- `expr-lang/expr` → rule engine expression evaluator

This contradicts the "lightweight core" promise. Users who only want the router must download JWT, OTel, Redis, and other heavy deps.

## Decision
**Core module MUST NOT directly require any sub-module that has heavy third-party dependencies.** Instead:
1. Core should only depend on lightweight utilities
2. Heavy integrations (OTel, GORM, MQ, Redis, gRPC, etc.) live in their own sub-modules
3. Users install sub-modules explicitly when needed: `go get github.com/astra-go/astra/middleware/security`
4. Core examples may demonstrate sub-module usage but must not make them required for core functionality

## Options Considered

### Option A: Keep Status Quo
- **Pros**: Convenient for testing, unified API
- **Cons**: Dependency pollution, violates lightweight promise

### Option B: Core Depends on Zero Sub-Modules
- **Pros**: Truly lightweight, users pick and choose
- **Cons**: Testing requires restructuring, breaking change

### Option C: Only testutil as test dependency
- **Pros**: Balanced approach
- **Cons**: Requires go.mod to distinguish test vs. main dependencies (not natively supported)

## Decision
**Option B: Core depends on zero heavy sub-modules.**

All sub-modules with third-party dependencies become truly optional. Core examples will demonstrate how to use sub-modules, but they are not bundled with core.

## Consequences
- **Breaking**: Users upgrading may need to update their `go.mod` if they relied on transitively-included deps
- **Documentation**: Must clearly document how to install optional sub-modules
- **CI**: Add depguard rules to prevent new heavy deps from entering core

## References
- ADR-002: Module/Plugin Interface Unification
- ADR-003: Rule Package Ownership
