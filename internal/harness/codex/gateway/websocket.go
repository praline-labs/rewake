package gateway

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Matches the native remote client's frame and assembled-message ceiling.
const maxMessage = 128 << 20

type socketClient struct {
	server    bool
	conn      net.Conn
	reader    *bufio.Reader
	writeOnce sync.Once
	writeGate chan struct{}
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
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return s.writeFrameContext(ctx, op, data)
}

// The gate and the write share the caller's budget. Once a frame write begins,
// failure closes the connection: appending another frame could corrupt the stream.
func (s *socketClient) writeFrameContext(ctx context.Context, op byte, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(data) > maxMessage {
		return &sizeError{Stage: "write-message", Size: uint64(len(data)), Limit: maxMessage}
	}
	if op >= 8 && len(data) > 125 {
		return errors.New("invalid websocket control frame")
	}
	s.writeOnce.Do(func() { s.writeGate = make(chan struct{}, 1) })
	select {
	case <-ctx.Done():
		return ctx.Err()
	case s.writeGate <- struct{}{}:
	}
	defer func() { <-s.writeGate }()
	if err := ctx.Err(); err != nil {
		return err
	}
	deadline, _ := ctx.Deadline()
	if err := s.conn.SetWriteDeadline(deadline); err != nil {
		return err
	}
	interrupted := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { _ = s.conn.SetWriteDeadline(time.Now()); close(interrupted) })
	defer func() {
		// An old cancellation must finish before the next writer sets its deadline.
		if !stop() {
			<-interrupted
		}
		_ = s.conn.SetWriteDeadline(time.Time{})
	}()
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
	if s.server {
		frame[1] &^= 128
	} else {
		frame = append(frame, mask[:]...)
	}
	if err := s.writeBytes(ctx, frame, deadline); err != nil {
		return err
	}
	if s.server {
		return s.writeBytes(ctx, data, deadline)
	}
	// Mask in fixed scratch space; never duplicate a maximum-size message.
	var scratch [64 << 10]byte
	for offset := 0; offset < len(data); {
		n := min(len(scratch), len(data)-offset)
		for i, value := range data[offset : offset+n] {
			scratch[i] = value ^ mask[(offset+i)%4]
		}
		if err := s.writeBytes(ctx, scratch[:n], deadline); err != nil {
			return err
		}
		offset += n
	}
	return nil
}
