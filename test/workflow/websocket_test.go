package workflow

import (
	"bufio"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// The Codex adapter speaks JSON-RPC over WebSocket, so a shim that plays a
// session has to speak it too — as a server for the wrapper's app-server
// socket, and as a client for the connection the wrapper proxies.
//
// This is a deliberately small implementation: text frames, no fragmentation,
// no extensions, no compression. It is not a WebSocket library and must not
// grow into one; it exists so a fixture can hold one conversation with one
// peer that is known to send whole text frames.

const maxTestFrame = 4 << 20

// wsConn is one framed connection, either side.
type wsConn struct {
	conn   net.Conn
	reader *bufio.Reader
	mask   bool // a client masks what it sends; a server must not

	writeMu sync.Mutex
}

// wsAccept completes the server side of the handshake on an accepted socket.
func wsAccept(conn net.Conn) (*wsConn, error) {
	reader := bufio.NewReader(conn)
	request, err := http.ReadRequest(reader)
	if err != nil {
		return nil, err
	}
	key := request.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		return nil, errors.New("handshake without Sec-WebSocket-Key")
	}
	sum := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	response := "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " +
		base64.StdEncoding.EncodeToString(sum[:]) + "\r\n\r\n"
	if _, err := io.WriteString(conn, response); err != nil {
		return nil, err
	}
	return &wsConn{conn: conn, reader: reader}, nil
}

// wsDial completes the client side against a Unix socket.
func wsDial(path string, deadline time.Time) (*wsConn, error) {
	conn, err := net.DialTimeout("unix", path, time.Until(deadline))
	if err != nil {
		return nil, err
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		_ = conn.Close()
		return nil, err
	}
	key := base64.StdEncoding.EncodeToString(nonce[:])
	request := "GET / HTTP/1.1\r\nHost: localhost\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n" +
		"Sec-WebSocket-Version: 13\r\nSec-WebSocket-Key: " + key + "\r\n\r\n"
	if err := conn.SetDeadline(deadline); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if _, err := io.WriteString(conn, request); err != nil {
		_ = conn.Close()
		return nil, err
	}
	reader := bufio.NewReader(conn)
	status, err := reader.ReadString('\n')
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if !strings.Contains(status, "101") {
		_ = conn.Close()
		return nil, fmt.Errorf("handshake refused: %s", strings.TrimSpace(status))
	}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			_ = conn.Close()
			return nil, err
		}
		if strings.TrimSpace(line) == "" {
			break
		}
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return &wsConn{conn: conn, reader: reader, mask: true}, nil
}

func (w *wsConn) close() { _ = w.conn.Close() }

// readMessage returns the payload of the next text frame. Control frames are
// answered or ignored rather than surfaced: a fixture has no use for them, but
// a peer that pings must still get its pong.
func (w *wsConn) readMessage() ([]byte, error) {
	for {
		opcode, payload, err := w.readFrame()
		if err != nil {
			return nil, err
		}
		switch opcode {
		case 0x1, 0x2:
			return payload, nil
		case 0x8:
			return nil, io.EOF
		case 0x9:
			if err := w.writeFrame(0xA, payload); err != nil {
				return nil, err
			}
		case 0xA:
		default:
			return nil, fmt.Errorf("unsupported opcode %#x", opcode)
		}
	}
}

func (w *wsConn) readFrame() (byte, []byte, error) {
	var header [2]byte
	if _, err := io.ReadFull(w.reader, header[:]); err != nil {
		return 0, nil, err
	}
	if header[0]&0x80 == 0 {
		return 0, nil, errors.New("fragmented frames are not supported here")
	}
	opcode := header[0] & 0x0f
	masked := header[1]&0x80 != 0
	length := uint64(header[1] & 0x7f)
	switch length {
	case 126:
		var extended [2]byte
		if _, err := io.ReadFull(w.reader, extended[:]); err != nil {
			return 0, nil, err
		}
		length = uint64(binary.BigEndian.Uint16(extended[:]))
	case 127:
		var extended [8]byte
		if _, err := io.ReadFull(w.reader, extended[:]); err != nil {
			return 0, nil, err
		}
		length = binary.BigEndian.Uint64(extended[:])
	}
	if length > maxTestFrame {
		return 0, nil, fmt.Errorf("frame of %d bytes is over the fixture limit", length)
	}
	var key [4]byte
	if masked {
		if _, err := io.ReadFull(w.reader, key[:]); err != nil {
			return 0, nil, err
		}
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(w.reader, payload); err != nil {
		return 0, nil, err
	}
	if masked {
		for i := range payload {
			payload[i] ^= key[i%4]
		}
	}
	return opcode, payload, nil
}

// writeJSON sends one value as a text frame.
func (w *wsConn) writeJSON(value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return w.writeFrame(0x1, raw)
}

func (w *wsConn) writeFrame(opcode byte, payload []byte) error {
	w.writeMu.Lock()
	defer w.writeMu.Unlock()
	header := []byte{0x80 | opcode}
	length := len(payload)
	maskBit := byte(0)
	if w.mask {
		maskBit = 0x80
	}
	switch {
	case length < 126:
		header = append(header, maskBit|byte(length))
	case length <= 0xffff:
		header = append(header, maskBit|126, byte(length>>8), byte(length))
	default:
		header = append(header, maskBit|127)
		var extended [8]byte
		binary.BigEndian.PutUint64(extended[:], uint64(length))
		header = append(header, extended[:]...)
	}
	body := payload
	if w.mask {
		var key [4]byte
		if _, err := rand.Read(key[:]); err != nil {
			return err
		}
		header = append(header, key[:]...)
		body = make([]byte, length)
		for i := range payload {
			body[i] = payload[i] ^ key[i%4]
		}
	}
	if _, err := w.conn.Write(append(header, body...)); err != nil {
		return err
	}
	return nil
}
