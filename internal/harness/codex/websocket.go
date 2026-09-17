package codex

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const maxFrame = 4 << 20

type socketClient struct {
	conn    net.Conn
	reader  *bufio.Reader
	writeMu sync.Mutex
}

func dialSocket(ctx context.Context, path string) (*socketClient, error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", path)
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			_ = conn.Close()
		}
	}()
	deadline := time.Now().Add(5 * time.Second)
	if limit, exists := ctx.Deadline(); exists && limit.Before(deadline) {
		deadline = limit
	}
	_ = conn.SetDeadline(deadline)
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, err
	}
	key := base64.StdEncoding.EncodeToString(nonce[:])
	request := "GET / HTTP/1.1\r\nHost: localhost\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: " + key + "\r\n\r\n"
	if _, err := io.WriteString(conn, request); err != nil {
		return nil, err
	}
	reader := bufio.NewReader(conn)
	var header []byte
	for {
		line, err := reader.ReadSlice('\n')
		header = append(header, line...)
		if len(header) > 65536 {
			return nil, errors.New("websocket handshake too large")
		}
		if err != nil && err != bufio.ErrBufferFull {
			return nil, err
		}
		if bytes.HasSuffix(header, []byte("\r\n\r\n")) {
			break
		}
	}
	response, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(header)), nil)
	if err != nil {
		return nil, err
	}
	sum := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	if response.StatusCode != 101 || !strings.EqualFold(response.Header.Get("Upgrade"), "websocket") || !strings.Contains(strings.ToLower(response.Header.Get("Connection")), "upgrade") || response.Header.Get("Sec-WebSocket-Accept") != base64.StdEncoding.EncodeToString(sum[:]) {
		return nil, errors.New("invalid websocket upgrade response")
	}
	_ = conn.SetDeadline(time.Time{})
	ok = true
	return &socketClient{conn: conn, reader: reader}, nil
}

func (s *socketClient) writeFrame(op byte, data []byte) error {
	if len(data) > maxFrame {
		return errors.New("websocket message too large")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_ = s.conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	var mask [4]byte
	if _, err := rand.Read(mask[:]); err != nil {
		return err
	}
	frame := []byte{0x80 | op}
	switch {
	case len(data) < 126:
		frame = append(frame, 0x80|byte(len(data)))
	case len(data) <= 65535:
		frame = append(frame, 0xfe, byte(len(data)>>8), byte(len(data)))
	default:
		frame = append(frame, 0xff)
		frame = binary.BigEndian.AppendUint64(frame, uint64(len(data)))
	}
	frame = append(frame, mask[:]...)
	for i, value := range data {
		frame = append(frame, value^mask[i%4])
	}
	for len(frame) > 0 {
		n, err := s.conn.Write(frame)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		frame = frame[n:]
	}
	return nil
}

func (s *socketClient) readMessage() ([]byte, error) {
	var message []byte
	fragmented := false
	for {
		var head [2]byte
		if _, err := io.ReadFull(s.reader, head[:]); err != nil {
			return nil, err
		}
		final, op := head[0]&128 != 0, head[0]&15
		if head[0]&0x70 != 0 || head[1]&128 != 0 {
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
		if size > maxFrame || uint64(len(message))+size > maxFrame {
			return nil, errors.New("websocket message too large")
		}
		if op >= 8 && (!final || size > 125) {
			return nil, errors.New("invalid websocket control frame")
		}
		data := make([]byte, int(size))
		if _, err := io.ReadFull(s.reader, data); err != nil {
			return nil, err
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
		case 1:
			if fragmented {
				return nil, errors.New("nested websocket message")
			}
			fragmented = true
		case 0:
			if !fragmented {
				return nil, errors.New("unexpected websocket continuation")
			}
		default:
			return nil, fmt.Errorf("unsupported websocket opcode %d", op)
		}
		message = append(message, data...)
		if final {
			if !utf8.Valid(message) {
				return nil, errors.New("invalid websocket text")
			}
			return message, nil
		}
	}
}
