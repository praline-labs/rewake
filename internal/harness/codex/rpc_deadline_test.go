package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

func TestCanceledRPCNeverStartsATurn(t *testing.T) {
	received := make(chan string, 1)
	path := socketFixture(t, func(c net.Conn, r *bufio.Reader) {
		_, _, _, _ = readClientFrame(r)
		serverMessage(c, map[string]any{"id": 1, "result": map[string]any{}})
		_, _, _, _ = readClientFrame(r)
		_, raw, _, err := readClientFrame(r)
		if err != nil {
			return
		}
		var request struct {
			Method string `json:"method"`
		}
		_ = json.Unmarshal(raw, &request)
		received <- request.Method
	})
	client, err := connectRPC(context.Background(), path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = client.call(ctx, "turn/start", map[string]string{"threadId": "thread"}, nil)
	t.Logf("call error=%v", err)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled call returned %v", err)
	}
	select {
	case method := <-received:
		t.Fatalf("already canceled operation reached server: %s", method)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestRPCDeadlineIncludesWritingTheFrame(t *testing.T) {
	clientSide, serverSide := net.Pipe()
	defer func() { _ = clientSide.Close() }()
	defer func() { _ = serverSide.Close() }()
	c := &rpcClient{socket: &socketClient{conn: clientSide, reader: bufio.NewReader(clientSide)}, pending: make(map[uint64]chan rpcReply), done: make(chan struct{})}
	timer := time.AfterFunc(250*time.Millisecond, func() { _ = serverSide.Close() })
	defer timer.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := c.call(ctx, "turn/start", map[string]string{"threadId": "thread"}, nil)
	elapsed := time.Since(start)
	t.Logf("deadline=20ms actual=%s error=%v", elapsed, err)
	if elapsed > 150*time.Millisecond || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("RPC deadline did not bound the write")
	}
}

type observedWriter struct {
	net.Conn
	started chan struct{}
	writes  atomic.Int32
}

func (w *observedWriter) Write(data []byte) (int, error) {
	if w.writes.Add(1) == 1 {
		close(w.started)
	}
	return w.Conn.Write(data)
}

func TestRPCDeadlineIncludesWaitingForAnotherWriter(t *testing.T) {
	clientSide, serverSide := net.Pipe()
	defer func() { _ = clientSide.Close() }()
	defer func() { _ = serverSide.Close() }()
	conn := &observedWriter{Conn: clientSide, started: make(chan struct{})}
	socket := &socketClient{conn: conn}
	first := make(chan error, 1)
	initial, cancelInitial := context.WithTimeout(context.Background(), time.Second)
	defer cancelInitial()
	go func() { first <- socket.writeFrameContext(initial, 1, []byte("first")) }()
	<-conn.started
	client := &rpcClient{socket: socket, pending: make(map[uint64]chan rpcReply), done: make(chan struct{})}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := client.call(ctx, "turn/start", struct{}{}, nil)
	if time.Since(start) > 150*time.Millisecond || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("writer contention exceeded deadline: %v", err)
	}
	_, body, _, err := readClientFrame(serverSide)
	if err != nil || string(body) != "first" {
		t.Fatalf("canceling the waiter disturbed another frame: %q %v", body, err)
	}
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if conn.writes.Load() != 1 {
		t.Fatal("expired waiter wrote a frame")
	}
}

func TestRPCCancellationInterruptsAWriteWithoutADeadline(t *testing.T) {
	clientSide, serverSide := net.Pipe()
	defer func() { _ = clientSide.Close() }()
	defer func() { _ = serverSide.Close() }()
	conn := &observedWriter{Conn: clientSide, started: make(chan struct{})}
	client := &rpcClient{socket: &socketClient{conn: conn}, pending: make(map[uint64]chan rpcReply), done: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- client.call(ctx, "turn/start", struct{}{}, nil) }()
	<-conn.started
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(150 * time.Millisecond):
		t.Fatal("canceled write remained blocked")
	}
	var data [1]byte
	if _, err := serverSide.Read(data[:]); err != io.EOF {
		t.Fatalf("failed write left a reusable connection: %v", err)
	}
}
