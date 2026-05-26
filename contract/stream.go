package contract

// ServerStream is the per-stream interface for server-side streaming RPCs.
// The server sends a sequence of messages; the client receives them.
// Transport: HTTP text/event-stream (SSE).
//
// Usage:
//
//	app.GET("/events", stream.ServerStreamHandler(func(s contract.ServerStream) error {
//	    for _, item := range items {
//	        if err := s.Send(item); err != nil { return err }
//	    }
//	    return nil
//	}))
type ServerStream interface {
	Context
	// Send serializes v and pushes it to the client.
	Send(v any) error
	// Done returns a channel that is closed when the stream ends (client
	// disconnected or handler returned).
	Done() <-chan struct{}
}

// ClientStream is the per-stream interface for client-side streaming RPCs.
// The client sends a sequence of messages; the server reads them and replies once.
// Transport: WebSocket with binary frame protocol.
//
// Usage:
//
//	app.GET("/upload", stream.ClientStreamHandler(func(s contract.ClientStream) error {
//	    var total int
//	    for {
//	        var chunk Chunk
//	        if err := s.Recv(&chunk); errors.Is(err, io.EOF) { break } else if err != nil { return err }
//	        total += len(chunk.Data)
//	    }
//	    return s.SendAndClose(Result{Total: total})
//	}))
type ClientStream interface {
	Context
	// Recv reads the next message from the client into v (blocks until a message
	// arrives or the stream closes). Returns io.EOF when the client has finished
	// sending.
	Recv(v any) error
	// SendAndClose sends a single response to the client and closes the stream.
	// Must be called exactly once, after all Recv calls are done.
	SendAndClose(v any) error
	// Done returns a channel that is closed when the stream ends.
	Done() <-chan struct{}
}

// BidiStream is the per-stream interface for bidirectional streaming RPCs.
// Both the client and the server can send and receive an arbitrary number of
// messages in any order.
// Transport: WebSocket with binary frame protocol.
//
// Usage:
//
//	app.GET("/chat", stream.BidiHandler(func(s contract.BidiStream) error {
//	    for {
//	        var msg Message
//	        if err := s.Recv(&msg); errors.Is(err, io.EOF) { return nil } else if err != nil { return err }
//	        if err := s.Send(Reply{Text: "echo: " + msg.Text}); err != nil { return err }
//	    }
//	}))
type BidiStream interface {
	Context
	// Send serializes v and writes it to the client.
	Send(v any) error
	// Recv reads the next client message into v. Returns io.EOF when the client
	// has closed its send side.
	Recv(v any) error
	// Done returns a channel that is closed when the stream ends.
	Done() <-chan struct{}
}

// ServerStreamHandler is a handler for server-side streaming RPCs.
type ServerStreamHandler func(ServerStream) error

// ClientStreamHandler is a handler for client-side streaming RPCs.
type ClientStreamHandler func(ClientStream) error

// BidiStreamHandler is a handler for bidirectional streaming RPCs.
type BidiStreamHandler func(BidiStream) error
