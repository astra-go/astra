package astra

// SealPool exposes the private sealPool method for white-box benchmarks and
// tests that exercise the pool-sizing path without starting a real server.
func (a *App) SealPool() { a.sealPool() }

// JsonBufPool exposes jsonBufPool for white-box tests.
var JsonBufPool = &jsonBufPool

// JsonBufMaxCap exposes jsonBufMaxCap for white-box tests.
const JsonBufMaxCap = jsonBufMaxCap
