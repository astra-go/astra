package testapp

import (
	"encoding/json"

	"google.golang.org/grpc/encoding"
)

// jsonCodec is a gRPC codec that uses JSON instead of protobuf.
// Registered as "proto" so it intercepts the default codec slot and lets
// EchoRequest/EchoResponse (plain Go structs) be used without proto.Message.
type jsonCodec struct{}

func (jsonCodec) Marshal(v any) ([]byte, error)   { return json.Marshal(v) }
func (jsonCodec) Unmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }
func (jsonCodec) Name() string                    { return "proto" }

func init() {
	encoding.RegisterCodec(jsonCodec{})
}
