package gateway

import (
	"encoding/binary"
	"errors"
	"io"
	"unicode/utf8"
)

func (s *socketClient) readMessage() ([]byte, error) {
	var message []byte
	fragmented := false
	for {
		var head [2]byte
		if _, err := io.ReadFull(s.reader, head[:]); err != nil {
			return nil, err
		}
		final, op := head[0]&128 != 0, head[0]&15
		if head[0]&0x70 != 0 || (head[1]&128 != 0) != s.server {
			return nil, errors.New("invalid server websocket frame")
		}
		size := uint64(head[1] & 127)
		switch size {
		case 126:
			var raw [2]byte
			if _, err := io.ReadFull(s.reader, raw[:]); err != nil {
				return nil, err
			}
			size = uint64(binary.BigEndian.Uint16(raw[:]))
		case 127:
			var raw [8]byte
			if _, err := io.ReadFull(s.reader, raw[:]); err != nil {
				return nil, err
			}
			size = binary.BigEndian.Uint64(raw[:])
		}
		if size > maxMessage {
			return nil, &sizeError{Stage: "read-frame", Size: size, Limit: maxMessage}
		}
		if op >= 8 && (!final || size > 125) {
			return nil, errors.New("invalid websocket control frame")
		}
		switch op {
		case 0:
			if !fragmented {
				return nil, errors.New("unexpected websocket continuation")
			}
		case 1:
			if fragmented {
				return nil, errors.New("nested websocket message")
			}
			fragmented = true
		case 8, 9, 10:
		default:
			return nil, errors.New("unsupported websocket opcode")
		}
		if op < 8 && uint64(len(message))+size > maxMessage {
			return nil, &sizeError{Stage: "read-message", Size: uint64(len(message)) + size, Limit: maxMessage}
		}
		var mask [4]byte
		if s.server {
			if _, err := io.ReadFull(s.reader, mask[:]); err != nil {
				return nil, err
			}
		}
		var data []byte
		if op < 8 {
			start := len(message)
			message = growMessage(message, int(size))
			data = message[start:]
		} else {
			data = make([]byte, int(size))
		}
		if _, err := io.ReadFull(s.reader, data); err != nil {
			return nil, err
		}
		if s.server {
			for i := range data {
				data[i] ^= mask[i%4]
			}
		}
		switch op {
		case 8:
			_ = s.writeFrame(8, data)
			return nil, io.EOF
		case 9:
			if err := s.writeFrame(10, data); err != nil {
				return nil, err
			}
			continue
		case 10:
			continue
		}
		if final {
			if !utf8.Valid(message) {
				return nil, errors.New("invalid websocket text")
			}
			return message, nil
		}
	}
}

// Read payloads directly into their final buffer. Geometric growth avoids copying
// the full prefix for every small fragment; capacity never exceeds the wire limit.
func growMessage(message []byte, extra int) []byte {
	needed := len(message) + extra
	if needed <= cap(message) {
		return message[:needed]
	}
	next := make([]byte, needed, min(maxMessage, max(needed, 2*cap(message))))
	copy(next, message)
	return next
}
