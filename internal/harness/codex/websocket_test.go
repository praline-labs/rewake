package codex

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func socketFixture(t *testing.T, serve func(net.Conn, *bufio.Reader)) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rpc.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		reader := bufio.NewReader(conn)
		req, err := http.ReadRequest(reader)
		if err != nil {
			return
		}
		sum := sha1.Sum([]byte(req.Header.Get("Sec-WebSocket-Key") + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
		_, _ = io.WriteString(conn, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: "+base64.StdEncoding.EncodeToString(sum[:])+"\r\n\r\n")
		serve(conn, reader)
	}()
	return path
}

func readClientFrame(r io.Reader) (byte, []byte, bool, error) {
	header := make([]byte, 2)
	if _, err := io.ReadFull(r, header); err != nil {
		return 0, nil, false, err
	}
	size := uint64(header[1] & 127)
	if size == 126 {
		var raw [2]byte
		if _, err := io.ReadFull(r, raw[:]); err != nil {
			return 0, nil, false, err
		}
		size = uint64(binary.BigEndian.Uint16(raw[:]))
	}
	if size == 127 {
		var raw [8]byte
		if _, err := io.ReadFull(r, raw[:]); err != nil {
			return 0, nil, false, err
		}
		size = binary.BigEndian.Uint64(raw[:])
	}
	if size > maxFrame {
		return 0, nil, false, io.ErrShortBuffer
	}
	var mask [4]byte
	masked := header[1]&128 != 0
	if masked {
		if _, err := io.ReadFull(r, mask[:]); err != nil {
			return 0, nil, false, err
		}
	}
	data := make([]byte, int(size))
	_, err := io.ReadFull(r, data)
	if masked {
		for i := range data {
			data[i] ^= mask[i%4]
		}
	}
	return header[0] & 15, data, masked, err
}

func TestSocketMasksRequestsAndHandlesControlFrames(t *testing.T) {
	got := make(chan bool, 1)
	path := socketFixture(t, func(c net.Conn, r *bufio.Reader) {
		op, data, masked, err := readClientFrame(r)
		if err != nil || op != 1 || string(data) != "request" || !masked {
			got <- false
			return
		}
		_, _ = c.Write([]byte{0x89, 1, 'p'})
		op, data, masked, err = readClientFrame(r)
		got <- err == nil && op == 10 && string(data) == "p" && masked
		_, _ = c.Write([]byte{0x01, 2, 'o', 'k', 0x80, 1, '!'})
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	client, err := dialSocket(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.conn.Close() }()
	if err := client.writeFrame(1, []byte("request")); err != nil {
		t.Fatal(err)
	}
	data, err := client.readMessage()
	if err != nil || string(data) != "ok!" {
		t.Fatalf("message=%q err=%v", data, err)
	}
	if !<-got {
		t.Fatal("client frames were not masked or pong was lost")
	}
}

func TestSocketRejectsOversizedFrames(t *testing.T) {
	path := socketFixture(t, func(c net.Conn, _ *bufio.Reader) {
		frame := make([]byte, 10)
		frame[0] = 0x81
		frame[1] = 127
		binary.BigEndian.PutUint64(frame[2:], maxFrame+1)
		_, _ = c.Write(frame)
	})
	client, err := dialSocket(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.conn.Close() }()
	if _, err := client.readMessage(); err == nil || !strings.Contains(err.Error(), "large") {
		t.Fatalf("unbounded frame: %v", err)
	}
}

func TestSocketRejectsUnexpectedUpgrade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_ = conn.SetDeadline(time.Now().Add(time.Second))
		_, _ = http.ReadRequest(bufio.NewReader(conn))
		_, _ = io.WriteString(conn, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: wrong\r\n\r\n")
	}()
	if client, err := dialSocket(context.Background(), path); err == nil {
		_ = client.conn.Close()
		t.Fatal("accepted an invalid WebSocket handshake")
	}
}
