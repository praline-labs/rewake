package gateway

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"
)

func readWithin(t *testing.T, c *socketClient) []byte {
	t.Helper()
	_ = c.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	b, e := c.readMessage()
	_ = c.conn.SetReadDeadline(time.Time{})
	if e != nil {
		t.Fatal(e)
	}
	return b
}

func write(t *testing.T, c *socketClient, b []byte) {
	t.Helper()
	if e := c.writeFrame(1, b); e != nil {
		t.Fatal(e)
	}
}

func setup(t *testing.T, startupFork ...bool) (*Gateway, *socketClient, chan *socketClient, string) {
	return setupConfig(t, Config{StartupFork: len(startupFork) > 0 && startupFork[0]})
}

func setupConfig(t *testing.T, cfg Config) (*Gateway, *socketClient, chan *socketClient, string) {
	t.Helper()
	d := t.TempDir()
	upPath := filepath.Join(d, "up")
	downPath := filepath.Join(d, "down")
	peers := make(chan *socketClient, 4)
	up := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s, e := upgrade(w, r)
		if e == nil {
			peers <- s
		}
	}), ReadHeaderTimeout: time.Second}
	l, e := net.Listen("unix", upPath)
	if e != nil {
		t.Fatal(e)
	}
	go func() { _ = up.Serve(l) }()
	cfg.Upstream = upPath
	cfg.Epoch = "test"
	g := New(cfg)
	gs := &http.Server{Handler: g, ReadHeaderTimeout: time.Second}
	dl, e := net.Listen("unix", downPath)
	if e != nil {
		t.Fatal(e)
	}
	go func() { _ = gs.Serve(dl) }()
	t.Cleanup(func() { g.Close(); _ = up.Close(); _ = gs.Close(); _ = l.Close(); _ = dl.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	ui, e := dialSocket(ctx, downPath)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = ui.conn.Close() })
	return g, ui, peers, downPath
}

func bindUI(t *testing.T, g *Gateway, ui, server *socketClient) {
	t.Helper()
	write(t, ui, []byte(startA))
	if !bytes.Equal(readWithin(t, server), []byte(startA)) {
		t.Fatal("native request changed")
	}
	reply := []byte(`{"id":1,"result":{"thread":{"id":"A","canAcceptDirectInput":true,"turns":[{"text":"SECRET HISTORY"}]}}}`)
	write(t, server, reply)
	if !bytes.Equal(readWithin(t, ui), reply) {
		t.Fatal("native response changed")
	}
	if !g.Binding().Ready {
		t.Fatal(g.Binding())
	}
}

func TestAdmissionOrderingApprovalReplyAndIDIsolation(t *testing.T) {
	g, ui, peers, _ := setup(t)
	server := <-peers
	defer func() { _ = server.conn.Close() }()
	bindUI(t, g, ui, server)
	got := make(chan error, 1)
	go func() {
		_, e := g.Deliver(context.Background(), g.Binding(), "question-id", "fixture notice")
		got <- e
	}()
	injection := readWithin(t, server)
	m := metadata(t, string(injection))
	if m.method != "turn/start" || m.thread != "A" || m.numeric {
		t.Fatal(m)
	}
	// This ordinary request waits for admission, but a later approval reply must pass it.
	later := []byte(`{"id":2,"method":"thread/start","params":{"threadSource":"user","runtimeWorkspaceRoots":[]}}`)
	write(t, ui, later)
	request := []byte(`{"id":1,"method":"item/commandExecution/requestApproval","params":{"threadId":"A","command":"SECRET"}}`)
	write(t, server, request)
	if !bytes.Equal(readWithin(t, ui), request) {
		t.Fatal("server request changed")
	}
	approval := []byte(`{"id":1,"result":{"decision":"accept"}}`)
	write(t, ui, approval)
	if !bytes.Equal(readWithin(t, server), approval) {
		t.Fatal("approval misrouted or lifecycle overtook injection")
	}
	write(t, server, []byte(fmt.Sprintf(`{"id":%q,"result":{"turn":{"id":"T"}}}`, m.idText)))
	if e := <-got; e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(readWithin(t, server), later) {
		t.Fatal("queued lifecycle not forwarded")
	}
	write(t, server, []byte(`{"id":2,"result":{"thread":{"id":"B","canAcceptDirectInput":true}}}`))
	response := metadata(t, string(readWithin(t, ui)))
	if response.id != "n:2" {
		t.Fatal("injected reply leaked to UI")
	}
}

func TestAuxiliaryConnectionAndReservedIDFailClosed(t *testing.T) {
	g, ui, peers, path := setup(t)
	first := <-peers
	defer func() { _ = first.conn.Close() }()
	bindUI(t, g, ui, first)
	old := g.Binding()
	next, e := dialSocket(context.Background(), path)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = next.conn.Close() }()
	second := <-peers
	defer func() { _ = second.conn.Close() }()
	write(t, next, []byte(`{"method":"initialized"}`))
	_ = readWithin(t, second)
	if g.Binding() != old {
		t.Fatal("auxiliary connection changed primary")
	}
	// Reserved IDs belong to each connection, including an auxiliary one.
	g.mu.Lock()
	serial := g.serial
	g.mu.Unlock()
	write(t, next, []byte(fmt.Sprintf(`{"id":"rewake-inject-test-%d-1","method":"thread/start"}`, serial)))
	_ = second.conn.SetReadDeadline(time.Now().Add(time.Second))
	if _, e = second.readMessage(); e == nil {
		t.Fatal("reserved collision forwarded")
	}
	if g.Binding() != old {
		t.Fatal("auxiliary failure invalidated primary")
	}
}

func TestPreCanceledInjectionSendsNothing(t *testing.T) {
	g, ui, peers, _ := setup(t)
	server := <-peers
	defer func() { _ = server.conn.Close() }()
	bindUI(t, g, ui, server)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := g.Deliver(ctx, g.Binding(), "id", "notice"); e == nil {
		t.Fatal("canceled request accepted")
	}
	write(t, ui, []byte(`{"id":2,"method":"thread/list","params":{}}`))
	m := metadata(t, string(readWithin(t, server)))
	if m.method != "thread/list" {
		t.Fatal("canceled work sent")
	}
}

func TestFragmentedFramesAndControl(t *testing.T) {
	a, b := net.Pipe()
	defer func() { _ = a.Close() }()
	defer func() { _ = b.Close() }()
	s := testSocket(a, false)
	result := make(chan error, 1)
	go func() {
		if _, e := b.Write(append([]byte{1, 5}, []byte(`{"x":`)...)); e != nil {
			result <- e
			return
		}
		if _, e := b.Write([]byte{0x89, 1, 'x'}); e != nil {
			result <- e
			return
		}
		pong := make([]byte, 7)
		if _, e := io.ReadFull(b, pong); e != nil {
			result <- e
			return
		}
		if pong[0] != 0x8a || pong[1] != 0x81 || pong[6]^pong[2] != 'x' {
			result <- fmt.Errorf("invalid pong")
			return
		}
		_, e := b.Write(append([]byte{0x80, 4}, []byte(`"y"}`)...))
		result <- e
	}()
	_ = a.SetDeadline(time.Now().Add(time.Second))
	raw, e := s.readMessage()
	if e != nil || string(raw) != `{"x":"y"}` {
		t.Fatalf("%s %v", raw, e)
	}
	if e = <-result; e != nil {
		t.Fatal(e)
	}
}

func TestMalformedFramesAndBoundedWrites(t *testing.T) {
	for _, frame := range [][]byte{{0x80, 0}, {0x09, 0}, {0x81, 0x80}, {0x82, 0}, {0x81, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}} {
		a, b := net.Pipe()
		s := testSocket(a, false)
		go func() { _, _ = b.Write(frame); _ = b.Close() }()
		if _, e := s.readMessage(); e == nil {
			t.Fatal("bad frame accepted")
		}
		_ = a.Close()
	}
	a, b := net.Pipe()
	defer func() { _ = b.Close() }()
	s := testSocket(a, false)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if e := s.writeFrameContext(ctx, 1, []byte("payload")); e == nil {
		t.Fatal("blocked write succeeded")
	}
	if _, e := a.Write([]byte("later")); e == nil {
		t.Fatal("partial frame connection not closed")
	}
}
