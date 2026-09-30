package cli

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/harness/codex/gateway"
)

type integrationWire struct {
	net.Conn
	reader *bufio.Reader
	masked bool
	mu     sync.Mutex
}

func (w *integrationWire) write(value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	header := []byte{0x81}
	maskBit := byte(0)
	if w.masked {
		maskBit = 128
	}
	if len(raw) < 126 {
		header = append(header, maskBit|byte(len(raw)))
	} else {
		header = append(header, maskBit|126, byte(len(raw)>>8), byte(len(raw)))
	}
	if w.masked {
		header = append(header, 0, 0, 0, 0)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	_, err = w.Write(append(header, raw...))
	return err
}

func (w *integrationWire) read() (map[string]json.RawMessage, error) {
	_ = w.SetReadDeadline(time.Now().Add(3 * time.Second))
	head := make([]byte, 2)
	if _, err := io.ReadFull(w.reader, head); err != nil {
		return nil, err
	}
	size := int(head[1] & 127)
	if size == 126 {
		var ext [2]byte
		if _, err := io.ReadFull(w.reader, ext[:]); err != nil {
			return nil, err
		}
		size = int(binary.BigEndian.Uint16(ext[:]))
	}
	if size >= 65536 || head[0] != 0x81 {
		return nil, fmt.Errorf("unexpected fixture frame")
	}
	var mask [4]byte
	if head[1]&128 != 0 {
		if _, err := io.ReadFull(w.reader, mask[:]); err != nil {
			return nil, err
		}
	}
	raw := make([]byte, size)
	if _, err := io.ReadFull(w.reader, raw); err != nil {
		return nil, err
	}
	if head[1]&128 != 0 {
		for i := range raw {
			raw[i] ^= mask[i%4]
		}
	}
	var value map[string]json.RawMessage
	err := json.Unmarshal(raw, &value)
	return value, err
}

func wireServer(t *testing.T, handler http.Handler) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rpc.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: time.Second}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	return path
}

func connectWire(t *testing.T, path string) *integrationWire {
	t.Helper()
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	_, err = fmt.Fprint(conn, "GET / HTTP/1.1\r\nHost: localhost\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n\r\n")
	if err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, &http.Request{Method: http.MethodGet})
	if err != nil || response.StatusCode != 101 {
		t.Fatalf("upgrade: %v", err)
	}
	return &integrationWire{Conn: conn, reader: reader, masked: true}
}

func gatewayWireFixture(t *testing.T, epoch string, complete func(gateway.Completion)) (*gateway.Gateway, *integrationWire, *integrationWire) {
	t.Helper()
	return gatewayWireFixtureConfig(t, gateway.Config{Epoch: epoch, Complete: complete})
}

func gatewayWireFixtureConfig(t *testing.T, cfg gateway.Config) (*gateway.Gateway, *integrationWire, *integrationWire) {
	t.Helper()
	peers := make(chan *integrationWire, 1)
	upstream := wireServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		sum := sha1.Sum([]byte(r.Header.Get("Sec-WebSocket-Key") + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
		_, _ = fmt.Fprintf(rw, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", base64.StdEncoding.EncodeToString(sum[:]))
		_ = rw.Flush()
		peers <- &integrationWire{Conn: conn, reader: rw.Reader}
	}))
	cfg.Upstream = upstream
	g := gateway.New(cfg)
	t.Cleanup(g.Close)
	ui := connectWire(t, wireServer(t, g))
	native := <-peers
	t.Cleanup(func() { _ = native.Close() })
	return g, ui, native
}

func wireExchange(t *testing.T, ui, native *integrationWire, id int, method string, params, result any) {
	t.Helper()
	if err := ui.write(map[string]any{"id": id, "method": method, "params": params}); err != nil {
		t.Fatal(err)
	}
	if _, err := native.read(); err != nil {
		t.Fatal(err)
	}
	if err := native.write(map[string]any{"id": id, "result": result}); err != nil {
		t.Fatal(err)
	}
	if _, err := ui.read(); err != nil {
		t.Fatal(err)
	}
}
