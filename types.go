package astra

import "github.com/astra-go/astra/contract"

// HandlerFunc is the core handler signature in Astra.
// Takes the concrete *Ctx directly — no interface boxing, all method calls
// are inlinable by the compiler.
type HandlerFunc func(*Ctx) error

// MiddlewareFunc is an alias for HandlerFunc — middleware is just a handler.
type MiddlewareFunc = HandlerFunc

// HandlersChain is a slice of HandlerFunc.
type HandlersChain []HandlerFunc

// Map is a shorthand for map[string]any (inspired by gin.H).
type Map map[string]any

// H is an alias for Map.
type H = Map

// ErrorHandler is a function that handles errors returned from handlers.
type ErrorHandler func(*Ctx, error)

// Option is a functional option for configuring the App.
type Option func(*Options)

// Streaming RPC types — re-exported from contract for convenience.

// ServerStream is the stream interface for server-side streaming RPCs (SSE).
type ServerStream = contract.ServerStream

// ClientStream is the stream interface for client-side streaming RPCs (WebSocket).
type ClientStream = contract.ClientStream

// BidiStream is the stream interface for bidirectional streaming RPCs (WebSocket).
type BidiStream = contract.BidiStream

// ServerStreamHandler is a handler for server-side streaming RPCs.
type ServerStreamHandler = contract.ServerStreamHandler

// ClientStreamHandler is a handler for client-side streaming RPCs.
type ClientStreamHandler = contract.ClientStreamHandler

// BidiStreamHandler is a handler for bidirectional streaming RPCs.
type BidiStreamHandler = contract.BidiStreamHandler

// Mode represents the application run mode.
// The mode affects default error response detail (dev returns full messages;
// prod hides internal errors from clients).
type Mode string

const (
	// ModeDev enables detailed error responses and is the default when Mode is unset.
	ModeDev Mode = "dev"
	// ModeProd suppresses internal error details in HTTP responses.
	ModeProd Mode = "prod"
	// ModeStaging behaves like prod but may include additional diagnostics.
	ModeStaging Mode = "staging"
	// ModeTest is intended for test harnesses; suppresses signal handling.
	ModeTest Mode = "test"
)
