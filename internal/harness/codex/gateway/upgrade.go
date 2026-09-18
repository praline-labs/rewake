package gateway

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

func upgrade(w http.ResponseWriter, r *http.Request) (*socketClient, error) {
	key := r.Header.Get("Sec-WebSocket-Key")
	decoded, err := base64.StdEncoding.DecodeString(key)
	if r.Method != "GET" || r.Header.Get("Sec-WebSocket-Version") != "13" || !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") || !strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade") || err != nil || len(decoded) != 16 {
		http.Error(w, "websocket upgrade required", 400)
		return nil, errors.New("invalid client handshake")
	}
	h, ok := w.(http.Hijacker)
	if !ok {
		return nil, errors.New("hijack unavailable")
	}
	c, b, err := h.Hijack()
	if err != nil {
		return nil, err
	}
	_ = c.SetWriteDeadline(time.Now().Add(3 * time.Second))
	sum := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	_, err = fmt.Fprintf(b, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", base64.StdEncoding.EncodeToString(sum[:]))
	if err == nil {
		err = b.Flush()
	}
	if err != nil {
		_ = c.Close()
		return nil, err
	}
	_ = c.SetWriteDeadline(time.Time{})
	return &socketClient{conn: c, reader: b.Reader, server: true}, nil
}

func testSocket(c net.Conn, server bool) *socketClient {
	return &socketClient{conn: c, reader: bufio.NewReader(c), server: server}
}
