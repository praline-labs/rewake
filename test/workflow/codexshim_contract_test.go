package workflow

// What the shim owes the protocol, checked directly rather than through a
// scenario. These came from the stage-1 acceptance reviews, where each round
// found the shim agreeing to something the real server refuses — and a shim
// that is too agreeable makes every scenario built on it worth less.
//
// Shape is checked separately, against the installed schema; this file is
// about behavior: who owns the handshake, what silence means, when an event
// is sent, and which requests must be refused.
//
// These run with the suite switch off, as part of an ordinary `go test ./...`,
// which is why none of them may start work: everything here decides something
// and returns. A test that accepted a turn would leave a goroutine running
// rewake after the test had restored the ambient environment — the one
// belonging to whatever session the owner is working in.

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func shimTestServer(t *testing.T) func() *wsConn {
	t.Helper()
	path := filepath.Join(t.TempDir(), "s")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	s := &shimSession{thread: shimThread}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go s.serve(conn)
		}
	}()
	return func() *wsConn {
		ws, err := wsDial(path, time.Now().Add(time.Second))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(ws.close)
		return ws
	}
}

func shimTestRead(t *testing.T, ws *wsConn) map[string]json.RawMessage {
	t.Helper()
	_ = ws.conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	raw, err := ws.readMessage()
	if err != nil {
		t.Fatal(err)
	}
	var msg map[string]json.RawMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatal(err)
	}
	return msg
}

func reviewInit(t *testing.T, ws *wsConn) {
	t.Helper()
	if err := ws.writeJSON(map[string]any{"id": 1, "method": "initialize", "params": map[string]any{"clientInfo": map[string]string{"name": "review", "version": "1"}, "capabilities": map[string]bool{"experimentalApi": true}}}); err != nil {
		t.Fatal(err)
	}
	if msg := shimTestRead(t, ws); msg["error"] != nil {
		t.Fatal(string(msg["error"]))
	}
}

func reviewStart(t *testing.T, ws *wsConn) {
	t.Helper()
	if err := ws.writeJSON(map[string]any{"id": 2, "method": "thread/start", "params": map[string]any{"threadSource": "user", "runtimeWorkspaceRoots": []string{"/work"}}}); err != nil {
		t.Fatal(err)
	}
}

func TestShimInitializeIsPerConnection(t *testing.T) {
	dial := shimTestServer(t)
	reviewInit(t, dial())
	second := dial()
	reviewStart(t, second)
	if msg := shimTestRead(t, second); msg["error"] == nil {
		t.Fatalf("uninitialized second connection accepted: %s", msg["result"])
	}
}

func TestShimNoCorrelatedReplyHasNoReply(t *testing.T) {
	t.Setenv(shimNoCorrelatedReply, "1")
	ws := shimTestServer(t)()
	reviewInit(t, ws)
	reviewStart(t, ws)
	first := shimTestRead(t, ws)
	if string(first["method"]) != `"thread/started"` {
		t.Fatalf("no event: %v", first)
	}
	_ = ws.conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	raw, err := ws.readMessage()
	if err == nil {
		t.Fatalf("control actually sends a correlated reply: %s", raw)
	}
	if timeout, ok := err.(net.Error); !ok || !timeout.Timeout() {
		t.Fatalf("unexpected close: %v", err)
	}
}

func TestShimNormalStartEmitsEventAfterReply(t *testing.T) {
	ws := shimTestServer(t)()
	reviewInit(t, ws)
	reviewStart(t, ws)
	if first := shimTestRead(t, ws); string(first["id"]) != "2" {
		t.Fatalf("first message is not reply: %v", first)
	}
	next := shimTestRead(t, ws)
	if string(next["method"]) != `"thread/started"` {
		t.Fatalf("missing started event: %v", next)
	}
}

func TestShimInvalidThreadRequestsAreRejected(t *testing.T) {
	cases := []struct {
		name, method, params string
		experimental         bool
	}{
		{"unknown-resume", "thread/resume", `{"threadId":"0199ab12-ffff-7fff-8fff-ffffffffffff"}`, true},
		{"missing-read-id", "thread/read", `{}`, true},
		{"unnegotiated-resume-roots", "thread/resume", `{"threadId":"` + shimThread + `","runtimeWorkspaceRoots":["/work"]}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &shimSession{thread: shimThread}
			peer := &shimPeer{initialized: true, experimental: tc.experimental}
			result, _, err := s.answer(peer, tc.method, json.RawMessage(tc.params))
			if err == nil {
				t.Fatalf("invalid %s accepted: %#v", tc.method, result)
			}
		})
	}
}

// Nothing in the suite may run rewake outside its own case. The guard is on
// every call the shim makes, because a turn accepted inside a test starts work
// in a goroutine that can outlive the environment the test set up for it.
func TestShimRefusesToRunOutsideACase(t *testing.T) {
	t.Setenv(shimEnv, "")
	t.Setenv(stateDirEnv, "/tmp/somewhere")
	if err := insideACase(); err == nil {
		t.Fatal("a process with no shim mark considered itself inside a case")
	}
	// A refusal is not enough on its own: with no rewake on PATH the call
	// would fail anyway and the test would pass while the guard was gone. So
	// PATH gets a rewake that leaves a mark, and the mark must not appear.
	dir := t.TempDir()
	mark := filepath.Join(dir, "touched")
	// Redirection rather than a command: PATH is about to hold only this
	// directory, so anything the script called would not be found and the mark
	// would stay absent for the wrong reason.
	script := "#!/bin/sh\n: > " + strconv.Quote(mark) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "rewake"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	s := &shimSession{thread: shimThread}
	if read, err := s.readMailbox(); err == nil {
		t.Fatalf("the mailbox was read outside a case: %q", read)
	}
	if _, err := os.Stat(mark); err == nil {
		t.Fatal("rewake was run outside a case")
	}
	t.Setenv(shimEnv, "1")
	t.Setenv(stateDirEnv, "")
	if err := insideACase(); err == nil {
		t.Fatal("a shim with no case state directory considered itself inside a case")
	}
}
