// Package chaos provides fault injection utilities for chaos engineering tests.
// It injects failures (timeout, error, latency, panic) into Astra endpoints
// without depending on external tools — pure Go only.
package chaos

import (
	"context"
	"math/rand/v2"
	"net/http"
	"sync"
	"time"

	"github.com/astra-go/astra"
)

// faultConfig describes the injected fault for a single endpoint.
type faultConfig struct {
	// Timeout configures a context deadline on matching requests.
	Timeout *time.Duration
	// ErrorRate is the probability [0.0, 1.0] of returning 5xx.
	ErrorRate float64
	// Latency adds an artificial delay before the handler runs.
	Latency *time.Duration
	// Panic causes the handler to panic.
	Panic bool
}

// FaultInjector manages fault injection lifecycle for chaos tests.
type FaultInjector struct {
	mu      sync.RWMutex
	faults  map[string]*faultConfig // endpoint pattern → config
	randMu  sync.Mutex
	randSrc *rand.Rand
}

// NewFaultInjector creates a ready-to-use FaultInjector.
func NewFaultInjector() *FaultInjector {
	return &FaultInjector{
		faults:  make(map[string]*faultConfig),
		randSrc: rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), uint64(time.Now().UnixNano()))),
	}
}

// InjectTimeout configures the given endpoint to trigger a context timeout
// after the specified duration.
func (f *FaultInjector) InjectTimeout(endpoint string, duration time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cfg := f.getOrCreate(endpoint)
	cfg.Timeout = &duration
}

// InjectError configures the given endpoint to return 5xx errors at the
// specified rate. errRate must be in [0.0, 1.0].
func (f *FaultInjector) InjectError(endpoint string, errRate float64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cfg := f.getOrCreate(endpoint)
	cfg.ErrorRate = errRate
}

// InjectLatency adds a fixed delay to the given endpoint.
func (f *FaultInjector) InjectLatency(endpoint string, delay time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cfg := f.getOrCreate(endpoint)
	cfg.Latency = &delay
}

// InjectPanic causes the given endpoint to panic.
func (f *FaultInjector) InjectPanic(endpoint string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cfg := f.getOrCreate(endpoint)
	cfg.Panic = true
}

// Reset clears all injected faults.
func (f *FaultInjector) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.faults = make(map[string]*faultConfig)
}

// Middleware returns an Astra middleware that applies configured faults
// based on the request path.
func (f *FaultInjector) Middleware() astra.HandlerFunc {
	return func(c *astra.Ctx) error {
		path := c.Request().URL.Path

		f.mu.RLock()
		cfg, ok := f.faults[path]
		f.mu.RUnlock()

		if !ok || cfg == nil {
			return c.Next()
		}

		// Inject latency
		if cfg.Latency != nil {
			time.Sleep(*cfg.Latency)
		}

		// Inject timeout
		if cfg.Timeout != nil {
			ctx, cancel := context.WithTimeout(c.Request().Context(), *cfg.Timeout)
			defer cancel()
			// Replace request context
			r := c.Request().WithContext(ctx)
			c.SetRequest(r)
			// Wait for the timeout to fire so the handler sees the deadline exceeded
			<-ctx.Done()
			if ctx.Err() == context.DeadlineExceeded {
				return c.JSON(http.StatusGatewayTimeout, map[string]string{
					"error": "chaos: timeout injected",
				})
			}
		}

		// Inject panic
		if cfg.Panic {
			panic("chaos: panic injected")
		}

		// Inject error
		if cfg.ErrorRate > 0 {
			f.randMu.Lock()
			roll := f.randSrc.Float64()
			f.randMu.Unlock()
			if roll < cfg.ErrorRate {
				return c.JSON(http.StatusInternalServerError, map[string]string{
					"error": "chaos: error injected",
				})
			}
		}

		return c.Next()
	}
}

// getOrCreate returns the existing faultConfig for the endpoint or creates a new one.
// Must be called with f.mu held.
func (f *FaultInjector) getOrCreate(endpoint string) *faultConfig {
	cfg, ok := f.faults[endpoint]
	if !ok {
		cfg = &faultConfig{}
		f.faults[endpoint] = cfg
	}
	return cfg
}
