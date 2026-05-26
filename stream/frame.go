package stream

import (
	"encoding/binary"
	"errors"
	"sync"
)

// Frame type constants for the binary framing protocol.
//
// Wire format per frame (WebSocket binary message):
//
//	[1B type][4B payload_len big-endian][N bytes payload]
//
// Total header: 5 bytes.
const (
	frameTypeData  byte = 0x01 // data frame — payload is serialized message
	frameTypeEnd   byte = 0x02 // end-of-stream — payload empty
	frameTypeError byte = 0x03 // error frame — payload is JSON {"message":"..."}
	frameTypePing  byte = 0x04 // ping — payload empty
	frameTypePong  byte = 0x05 // pong — payload empty

	frameHeaderSize = 5
)

// frameBufPool recycles byte slices used to build outgoing frames, keeping
// per-message allocations off the heap on the hot path.
var frameBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, 512)
		return &b
	},
}

// encodeFrame appends a complete frame to dst and returns the extended slice.
// Callers that borrow dst from frameBufPool must return it after the write.
func encodeFrame(dst []byte, typ byte, payload []byte) []byte {
	dst = append(dst, typ)
	l := uint32(len(payload))
	dst = append(dst, byte(l>>24), byte(l>>16), byte(l>>8), byte(l))
	return append(dst, payload...)
}

type frame struct {
	typ     byte
	payload []byte
}

// decodeFrame parses a single frame from a WebSocket binary message.
func decodeFrame(data []byte) (frame, error) {
	if len(data) < frameHeaderSize {
		return frame{}, errors.New("stream: frame too short")
	}
	typ := data[0]
	l := binary.BigEndian.Uint32(data[1:5])
	end := frameHeaderSize + int(l)
	if len(data) < end {
		return frame{}, errors.New("stream: frame payload truncated")
	}
	return frame{typ: typ, payload: data[frameHeaderSize:end]}, nil
}

// encodeError builds an error frame whose payload is a JSON object.
func encodeError(dst []byte, msg string) []byte {
	payload := append([]byte(`{"message":`), '"')
	for i := 0; i < len(msg); i++ {
		c := msg[i]
		switch c {
		case '"':
			payload = append(payload, '\\', '"')
		case '\\':
			payload = append(payload, '\\', '\\')
		case '\n':
			payload = append(payload, '\\', 'n')
		default:
			payload = append(payload, c)
		}
	}
	payload = append(payload, '"', '}')
	return encodeFrame(dst, frameTypeError, payload)
}
