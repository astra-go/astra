package astra

import (
	"fmt"
)

// RunQUIC starts an HTTP/3 (QUIC) server on addr and simultaneously starts a
// TLS server that advertises HTTP/3 via the Alt-Svc response header. Clients
// that support HTTP/3 will upgrade automatically on their next request.
//
// This is a convenience wrapper around the astra/quic sub-module. Import
// github.com/astra-go/astra/quic for advanced configuration options like
// 0-RTT early data, custom idle timeouts, or QUIC-only mode.
//
//	app := astra.New()
//	// ... register routes ...
//	app.RunQUIC(":443", "cert.pem", "key.pem")
//
// Note: This method requires the astra/quic sub-module to be imported in your
// application. If the quic package is not imported, this method will return an
// error at runtime.
func (a *App) RunQUIC(addr, certFile, keyFile string) error {
	// This method serves as a bridge to the quic sub-module. The actual
	// implementation is in github.com/astra-go/astra/quic to keep the ~40
	// transitive dependencies of quic-go out of the core module.
	//
	// The quic package registers itself via init() and sets quicRunner when
	// imported. If quicRunner is nil, the user forgot to import the quic package.
	if quicRunner == nil {
		return fmt.Errorf("astra: RunQUIC requires importing github.com/astra-go/astra/quic")
	}
	return quicRunner(a, addr, certFile, keyFile)
}

// quicRunner is set by the quic sub-module's init() function when imported.
// This indirection keeps quic-go's dependencies out of the core module while
// still allowing App.RunQUIC() to exist as a convenience method.
var quicRunner func(*App, string, string, string) error

// RegisterQUICRunner is called by the quic sub-module's init() function to
// register its implementation. This allows the quic package to wire itself
// into the core App without creating a circular dependency.
func RegisterQUICRunner(fn func(*App, string, string, string) error) {
	quicRunner = fn
}
