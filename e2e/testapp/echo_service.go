package testapp

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// EchoRequest is the request message for the Echo RPC.
type EchoRequest struct {
	Message string
}

// EchoResponse is the response message for the Echo RPC.
type EchoResponse struct {
	Message string
}

// EchoServiceName is the fully-qualified gRPC service name.
const EchoServiceName = "testapp.EchoService"

// EchoService implements a minimal gRPC service for e2e testing.
// It echoes the request message back and validates the Bearer token in metadata.
type EchoService struct {
	secret string
}

func NewEchoService(jwtSecret string) *EchoService {
	return &EchoService{secret: jwtSecret}
}

// Echo echoes the request message. Requires a valid Bearer token in metadata.
func (s *EchoService) Echo(ctx context.Context, req *EchoRequest) (*EchoResponse, error) {
	if err := s.validateToken(ctx); err != nil {
		return nil, err
	}
	return &EchoResponse{Message: req.Message}, nil
}

func (s *EchoService) validateToken(ctx context.Context) error {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "missing metadata")
	}
	vals := md.Get("authorization")
	if len(vals) == 0 {
		return status.Error(codes.Unauthenticated, "missing authorization")
	}
	// Expect "Bearer <token>"
	auth := vals[0]
	const prefix = "Bearer "
	if len(auth) <= len(prefix) {
		return status.Error(codes.Unauthenticated, "invalid authorization format")
	}
	tokenStr := auth[len(prefix):]
	if err := validateJWT(tokenStr, s.secret); err != nil {
		return status.Error(codes.Unauthenticated, err.Error())
	}
	return nil
}

// EchoServiceDesc is the grpc.ServiceDesc for EchoService.
// Using ServiceDesc directly avoids any protoc/proto-gen dependency.
var EchoServiceDesc = grpc.ServiceDesc{
	ServiceName: EchoServiceName,
	HandlerType: (*echoServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Echo",
			Handler:    echoHandler,
		},
	},
	Streams: []grpc.StreamDesc{},
}

type echoServiceServer interface {
	Echo(context.Context, *EchoRequest) (*EchoResponse, error)
}

func echoHandler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	req := new(EchoRequest)
	if err := dec(req); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(echoServiceServer).Echo(ctx, req)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/" + EchoServiceName + "/Echo"}
	handler := func(ctx context.Context, req any) (any, error) {
		return srv.(echoServiceServer).Echo(ctx, req.(*EchoRequest))
	}
	return interceptor(ctx, req, info, handler)
}
