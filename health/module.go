package health

import "github.com/astra-go/astra"

// NewModule returns an astra.Module that registers the standard Kubernetes
// health-check endpoints (/live, /ready, /health) when installed on an *App.
//
//	app.Register(
//	    health.NewModule(
//	        health.WithProbe("db", func(ctx context.Context) error {
//	            return db.PingContext(ctx)
//	        }),
//	        health.WithPrefix("/internal"),
//	    ),
//	)
func NewModule(opts ...Option) astra.Module {
	return astra.NewModuleFunc("health", func(app *astra.App) error {
		Register(app, opts...)
		return nil
	})
}

// NewIstioModule returns an astra.Module that registers both the standard
// Kubernetes probe paths (/live, /ready, /health) AND the Istio-compatible
// paths (/healthz/live, /healthz/ready) on the same App.
//
// Use this instead of NewModule when deploying behind an Istio sidecar.
//
//	app.Register(
//	    health.NewIstioModule(
//	        health.WithProbe("db", dbProbe),
//	        health.WithIstioHeaders(),
//	    ),
//	)
func NewIstioModule(opts ...Option) astra.Module {
	return astra.NewModuleFunc("health.istio", func(app *astra.App) error {
		Register(app, opts...)
		RegisterIstioProbes(app, opts...)
		return nil
	})
}
