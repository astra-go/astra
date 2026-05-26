package astra

// Benchmark isTrustedProxy to quantify the gain from pre-compiling CIDRs.
//
// The test calls isTrustedProxy with a list of 5 mixed CIDR / plain-IP proxies
// to reflect a realistic production configuration.
//
// Run with:
//
//	go test -bench=BenchmarkIsTrustedProxy -benchmem -count=5 .

import (
	"net"
	"testing"
)

var benchProxies = []string{
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"127.0.0.1",
	"::1",
}

// probeIP is an IP that is NOT in any of benchProxies, exercising the full
// loop (worst-case: must check every entry before returning false).
var probeIPMiss = net.ParseIP("203.0.113.42")

// probeIPHit matches the last plain-IP entry; exercises an early exit.
var probeIPHit = net.ParseIP("127.0.0.1")

// BenchmarkIsTrustedProxy_Miss measures a full-miss lookup (worst case).
func BenchmarkIsTrustedProxy_Miss(b *testing.B) {
	o := defaultOptions()
	o.TrustedProxies = benchProxies
	o.prepareTrustedNets()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = o.isTrustedProxy(probeIPMiss)
	}
}

// BenchmarkIsTrustedProxy_Hit measures an early-exit lookup (best case).
func BenchmarkIsTrustedProxy_Hit(b *testing.B) {
	o := defaultOptions()
	o.TrustedProxies = benchProxies
	o.prepareTrustedNets()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = o.isTrustedProxy(probeIPHit)
	}
}
