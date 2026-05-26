package grpcserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/astra-go/astra"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// GatewayRegistrar is called during server startup to register gRPC service
// handlers into the gateway mux. Receive the mux and an in-process connection
// to the local gRPC server, then call pb.RegisterXxxHandler(ctx, mux, conn)
// for each service to expose over HTTP.
//
// Example:
//
//	grpcserver.WithGateway("/api", func(ctx context.Context, mux *runtime.ServeMux, conn *grpc.ClientConn) error {
//	    return pb.RegisterInventoryServiceHandler(ctx, mux, conn)
//	})
type GatewayRegistrar func(ctx context.Context, mux *runtime.ServeMux, conn *grpc.ClientConn) error

// WithGateway mounts a grpc-gateway reverse proxy on the HTTP server.
//
// prefix scopes which URL prefix is routed to the gateway. Use "" to catch all
// unmatched requests. The prefix is stripped before the request reaches the
// gateway mux, so proto http.rules paths should NOT include the prefix.
//
// The registrar receives:
//   - ctx: a background context valid until server shutdown
//   - mux: the gateway ServeMux to register handlers into
//   - conn: a live in-process gRPC ClientConn to the local server
//
// Call pb.RegisterXxxHandler(ctx, mux, conn) inside the registrar.
// Returns an error immediately if the registrar fails, preventing server start.
//
//	srv := grpcserver.New(app,
//	    grpcserver.WithGateway("/", func(ctx context.Context, mux *runtime.ServeMux, conn *grpc.ClientConn) error {
//	        return pb.RegisterInventoryServiceHandler(ctx, mux, conn)
//	    }),
//	)
func WithGateway(prefix string, registrar GatewayRegistrar) Option {
	return func(o *ServerOptions) {
		o.gatewayPrefix = strings.TrimRight(prefix, "/")
		o.gatewayRegistrar = registrar
	}
}

// mountGateway initialises the grpc-gateway mux and mounts it on the Astra app.
// Called from Run() after the gRPC listener is bound but before HTTP ListenAndServe.
func (s *Server) mountGateway(ctx context.Context) error {
	return s.doMountGateway(ctx, s.opts.gatewayPrefix, s.opts.gatewayRegistrar)
}

// MountGateway mounts a grpc-gateway reverse proxy on the HTTP server.
// Call it after New() and before Run() to add a gateway alongside the gRPC
// services you have already registered with srv.GRPC.
//
// prefix scopes which URL prefix is routed to the gateway (use "" to catch all
// unmatched requests). The prefix is stripped so proto http.rule paths do NOT
// need to include it.
//
// Example:
//
//	srv := grpcserver.New(app, ...)
//	pb.RegisterInventoryServiceServer(srv.GRPC, impl)
//	if err := srv.MountGateway("/api", func(ctx context.Context, mux *runtime.ServeMux, conn *grpc.ClientConn) error {
//	    return pb.RegisterInventoryServiceHandler(ctx, mux, conn)
//	}); err != nil { log.Fatal(err) }
//	srv.Run()
func (s *Server) MountGateway(prefix string, registrar GatewayRegistrar) error {
	return s.doMountGateway(context.Background(), strings.TrimRight(prefix, "/"), registrar)
}

// doMountGateway is the shared implementation used by mountGateway and MountGateway.
func (s *Server) doMountGateway(ctx context.Context, prefix string, registrar GatewayRegistrar) error {
	conn, err := grpc.NewClient(
		s.opts.grpcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return err
	}

	mux := runtime.NewServeMux(
		runtime.WithErrorHandler(gatewayErrorHandler),
		runtime.WithRoutingErrorHandler(gatewayRoutingErrorHandler),
	)
	if err := registrar(ctx, mux, conn); err != nil {
		_ = conn.Close()
		return err
	}

	// Close the connection when the server shuts down.
	s.gatewayConn = conn

	// Mount the mux on the Astra HTTP app.
	// Requests matching the prefix are stripped of it before forwarding so that
	// proto http.rule paths (e.g. "/v1/items/{id}") work without a prefix.
	handler := func(c *astra.Ctx) error {
		r := c.Request()
		if prefix != "" {
			// Strip prefix so the gateway mux sees the path it registered.
			stripped := strings.TrimPrefix(r.URL.Path, prefix)
			if stripped == "" {
				stripped = "/"
			}
			r2 := r.Clone(r.Context())
			r2.URL.Path = stripped
			r2.URL.RawPath = ""
			mux.ServeHTTP(c.Writer(), r2)
		} else {
			mux.ServeHTTP(c.Writer(), r)
		}
		return nil
	}

	pattern := prefix + "/*"
	if prefix == "" {
		pattern = "/*"
	}
	s.HTTP.Any(pattern, handler)
	return nil
}

// gatewayErrorHandler aligns grpc-gateway error responses with the project's
// HTTP error format: {"code": <http_code>, "message": "<text>"}.
func gatewayErrorHandler(
	ctx context.Context,
	_ *runtime.ServeMux,
	_ runtime.Marshaler,
	w http.ResponseWriter,
	_ *http.Request,
	err error,
) {
	s, _ := status.FromError(err)
	httpCode := grpcCodeToHTTPStatus(s.Code())
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpCode)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"code":    httpCode,
		"message": s.Message(),
	})
}

// gatewayRoutingErrorHandler handles cases where the gateway mux itself cannot
// find a matching route (e.g. the path exists in proto but with a different HTTP
// method). Returns 404 for unknown routes and 405 for method mismatches.
func gatewayRoutingErrorHandler(
	ctx context.Context,
	mux *runtime.ServeMux,
	marshaler runtime.Marshaler,
	w http.ResponseWriter,
	r *http.Request,
	httpStatus int,
) {
	var code codes.Code
	switch httpStatus {
	case http.StatusNotFound:
		code = codes.NotFound
	case http.StatusMethodNotAllowed:
		code = codes.Unimplemented
	default:
		code = codes.Internal
	}
	gatewayErrorHandler(ctx, mux, marshaler, w, r,
		status.Error(code, http.StatusText(httpStatus)))
}
