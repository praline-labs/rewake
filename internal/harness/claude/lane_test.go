package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
)

// receiver stands in for a Claude Code session: it reads each line on its
// socket and answers the reply address with the receipts its script names.
type receiver struct {
	socket string
	lines  chan map[string]any
}

// startReceiver answers every line with the statuses in script, in order, each
// on a new connection as the harness does.
func startReceiver(t *testing.T, dir string, script ...string) *receiver {
	t.Helper()
	r := &receiver{socket: filepath.Join(dir, "session.sock"), lines: make(chan map[string]any, 8)}
	listener, err := net.Listen("unix", r.socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			scanner := bufio.NewScanner(connection)
			if !scanner.Scan() {
				_ = connection.Close()
				continue
			}
			var line map[string]any
			_ = json.Unmarshal(scanner.Bytes(), &line)
			_ = connection.Close()
			r.lines <- line
			from, _ := line["from"].(string)
			id, _ := line["msg_id"].(string)
			for _, status := range script {
				answer(t, strings.TrimPrefix(from, "uds:"), id, status)
			}
		}
	}()
	return r
}

// answer writes one receipt; "refused" is an expiry with that detail.
func answer(t *testing.T, reply, id, status string) {
	t.Helper()
	receipt := map[string]any{"type": "control", "action": "peer_message_status", "status": status, "orig_msg_id": id, "msgV": 1}
	if status == "refused" {
		receipt["status"], receipt["status_detail"] = "expired", "refused"
	}
	if status == "held" {
		receipt["reason"] = "held for approval"
	}
	encoded, _ := json.Marshal(receipt)
	connection, err := net.Dial("unix", reply)
	if err != nil {
		t.Errorf("could not reach the reply socket: %v", err)
		return
	}
	_, _ = connection.Write(append(encoded, '\n'))
	_ = connection.Close()
}

func startLane(t *testing.T, dir string, drawn <-chan struct{}) *lane {
	t.Helper()
	l := newLane(filepath.Join(dir, "session.reply.sock"), false, drawn, nil)
	l.window, l.limit = 200*time.Millisecond, 100*time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel(); l.Close() })
	if err := l.Start(ctx); err != nil {
		t.Fatal(err)
	}
	return l
}

func deliver(l *lane, r *receiver, id string) inbox.Result {
	return l.Deliver(context.Background(), registry.Session{Socket: r.socket}, inbox.Message{ID: id, Kind: inbox.Task, Text: "work"})
}

func nextReceipt(t *testing.T, l *lane) inbox.Receipt {
	t.Helper()
	select {
	case receipt := <-l.Receipts():
		return receipt
	case <-time.After(2 * time.Second):
		t.Fatal("no receipt reached the server")
		return inbox.Receipt{}
	}
}

func TestLaneSilenceIsDelivered(t *testing.T) {
	dir := t.TempDir()
	r := startReceiver(t, dir)
	l := startLane(t, dir, nil)
	result := deliver(l, r, "m1")
	if result.State != inbox.Delivered {
		t.Fatalf("an accepted line is delivered, got %+v", result)
	}
	line := <-r.lines
	if line["from"] != "uds:"+l.reply || !uuidPattern(line["msg_id"]) {
		t.Fatalf("the line has to ask for receipts, got from %v msg_id %v", line["from"], line["msg_id"])
	}
}

func TestLaneHeldThenReleased(t *testing.T) {
	dir := t.TempDir()
	r := startReceiver(t, dir, "held")
	l := startLane(t, dir, nil)
	result := deliver(l, r, "m1")
	if result.State != inbox.Held || !strings.Contains(result.Detail, "held for approval") {
		t.Fatalf("a held line is held, with the harness's reason, got %+v", result)
	}
	line := <-r.lines
	answer(t, l.reply, line["msg_id"].(string), "delivered")
	if receipt := nextReceipt(t, l); receipt.ID != "m1" || receipt.Result.State != inbox.Delivered {
		t.Fatalf("the release goes to the server under the message id, got %+v", receipt)
	}
}

// Words that follow each other closely end the same way whichever is read
// first: each comes on its own connection, so their order is not kept, and the
// final one wins either way.
func TestLaneCloseWordsEndWithTheFinalOne(t *testing.T) {
	dir := t.TempDir()
	r := startReceiver(t, dir, "held", "expired")
	l := startLane(t, dir, nil)
	switch result := deliver(l, r, "m1"); result.State {
	case inbox.Failed:
	case inbox.Held:
		if receipt := nextReceipt(t, l); receipt.ID != "m1" || receipt.Result.State != inbox.Failed {
			t.Fatalf("the expiry goes to the server, got %+v", receipt)
		}
	default:
		t.Fatalf("got %+v", result)
	}
}

func TestLaneRefusalFails(t *testing.T) {
	dir := t.TempDir()
	r := startReceiver(t, dir, "refused")
	l := startLane(t, dir, nil)
	result := deliver(l, r, "m1")
	if result.State != inbox.Failed || !strings.Contains(result.Detail, "refuses messages from other sessions") {
		t.Fatalf("a refused line fails, got %+v", result)
	}
	if len(l.sent) != 0 {
		t.Fatalf("a settled line is forgotten, %d left", len(l.sent))
	}
}

// A word after the window still reaches the server, which takes the delivery
// back: a hold is never counted as delivered for long.
func TestLaneLateWordReachesTheServer(t *testing.T) {
	dir := t.TempDir()
	r := startReceiver(t, dir)
	l := startLane(t, dir, nil)
	if result := deliver(l, r, "m1"); result.State != inbox.Delivered {
		t.Fatalf("got %+v", result)
	}
	line := <-r.lines
	msgID := line["msg_id"].(string)
	answer(t, l.reply, msgID, "held")
	if receipt := nextReceipt(t, l); receipt.ID != "m1" || receipt.Result.State != inbox.Held {
		t.Fatalf("a late hold goes to the server, got %+v", receipt)
	}
	// Held, the line waits for its end however long that takes.
	l.mu.Lock()
	l.pruneSilent(time.Now().Add(2 * inbox.LateWordWindow))
	l.mu.Unlock()
	answer(t, l.reply, msgID, "expired")
	if receipt := nextReceipt(t, l); receipt.ID != "m1" || receipt.Result.State != inbox.Failed {
		t.Fatalf("the end of a late hold goes to the server, got %+v", receipt)
	}
}

// Past the window the server keeps, a late word is dropped, and so are lines
// beyond the number kept.
func TestLaneForgetsSilentLines(t *testing.T) {
	dir := t.TempDir()
	r := startReceiver(t, dir)
	l := startLane(t, dir, nil)
	if result := deliver(l, r, "m1"); result.State != inbox.Delivered {
		t.Fatalf("got %+v", result)
	}
	line := <-r.lines
	l.mu.Lock()
	l.sent[line["msg_id"].(string)].at = time.Now().Add(-2 * inbox.LateWordWindow)
	l.mu.Unlock()
	answer(t, l.reply, line["msg_id"].(string), "held")
	select {
	case receipt := <-l.Receipts():
		t.Fatalf("a word past the window reached the server: %+v", receipt)
	case <-time.After(300 * time.Millisecond):
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	for index := range inbox.LateWordKeep + 5 {
		id := fmt.Sprint(index)
		l.sent[id] = &sentNotice{id: id, msgID: id, claimed: true, at: now}
		l.silent = append(l.silent, id)
	}
	l.pruneSilent(now)
	if len(l.silent) != inbox.LateWordKeep || len(l.sent) != inbox.LateWordKeep || l.sent["0"] != nil {
		t.Fatalf("kept %d silent lines, %d sent, want the last %d", len(l.silent), len(l.sent), inbox.LateWordKeep)
	}
}

// A reply socket nobody listens on is a killed wrapper's and goes; a live
// one, a young one, and anything that is not a reply socket, stays.
func TestLaneSweepsDeadReplySockets(t *testing.T) {
	dir := t.TempDir()
	dead := filepath.Join(dir, "gone.old.reply.sock")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: dead, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	listener.SetUnlinkOnClose(false)
	_ = listener.Close()
	live := filepath.Join(dir, "here.now.reply.sock")
	alive, err := net.Listen("unix", live)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = alive.Close() }()
	session := filepath.Join(dir, "gone.old.sock")
	other, err := net.ListenUnix("unix", &net.UnixAddr{Name: session, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	other.SetUnlinkOnClose(false)
	_ = other.Close()

	young := filepath.Join(dir, "starting.now.reply.sock")
	bound, err := net.ListenUnix("unix", &net.UnixAddr{Name: young, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	bound.SetUnlinkOnClose(false)
	_ = bound.Close()
	old := time.Now().Add(-time.Minute)
	for _, path := range []string{dead, session} {
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
	}

	sweepReplies(dir)
	if _, err := os.Lstat(dead); !os.IsNotExist(err) {
		t.Fatalf("a dead reply socket stayed: %v", err)
	}
	// A young socket may be bound and not yet listening.
	for _, path := range []string{live, session, young} {
		if _, err := os.Lstat(path); err != nil {
			t.Fatalf("%s went: %v", path, err)
		}
	}
}

func TestLaneWithoutReplySocketWritesPlainly(t *testing.T) {
	dir := t.TempDir()
	r := startReceiver(t, dir)
	l := newLane("", false, nil, nil)
	if err := l.Start(context.Background()); err == nil {
		t.Fatal("a lane with no reply socket has to say it hears nothing back")
	}
	defer l.Close()
	if result := deliver(l, r, "m1"); result.State != inbox.Delivered {
		t.Fatalf("got %+v", result)
	}
	if line := <-r.lines; line["from"] != nil || line["msg_id"] != nil {
		t.Fatalf("a line nobody listens for asks for nothing, got %v", line)
	}
}

func TestLaneOpens(t *testing.T) {
	opened := func(l *lane) bool {
		select {
		case <-l.Opened():
			return true
		case <-time.After(time.Second):
			return false
		}
	}
	t.Run("on the status line", func(t *testing.T) {
		drawn := make(chan struct{})
		l := newLane("", false, drawn, nil)
		l.limit = time.Hour
		_ = l.Start(context.Background())
		defer l.Close()
		close(drawn)
		if !opened(l) {
			t.Fatal("the gate stays shut after the status line")
		}
	})
	t.Run("after the limit", func(t *testing.T) {
		l := newLane("", false, make(chan struct{}), nil)
		l.limit = 10 * time.Millisecond
		_ = l.Start(context.Background())
		defer l.Close()
		if !opened(l) {
			t.Fatal("the gate stays shut past its limit")
		}
	})
}

func TestLaneMakesItsOwnDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sock")
	l := newLane(filepath.Join(dir, "a.reply.sock"), true, nil, nil)
	defer l.Close()
	if err := l.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(l.reply); err != nil || info.Mode()&os.ModeSocket == 0 || info.Mode().Perm() != 0o600 {
		t.Fatalf("the reply socket is missing or open to others: %v %v", info, err)
	}
	l.Close()
	if _, err := os.Lstat(l.reply); !os.IsNotExist(err) {
		t.Fatalf("the reply socket outlived the lane: %v", err)
	}
}

func TestParseReceipt(t *testing.T) {
	cases := []struct {
		line  string
		ids   []string
		state inbox.State
	}{
		{`{"type":"control","action":"peer_message_status","status":"held","orig_msg_id":"a"}`, []string{"a"}, inbox.Held},
		{`{"type":"control","action":"peer_message_status","status":"delivered","orig_msg_id":"a"}`, []string{"a"}, inbox.Delivered},
		{`{"type":"control","action":"peer_message_status","status":"expired","status_detail":"refused","orig_msg_id":"a"}`, []string{"a"}, inbox.Failed},
		{`{"type":"control","action":"peer_message_status","status":"denied","orig_msg_id":"a"}`, []string{"a"}, inbox.Failed},
		{`{"type":"control","action":"peer_message_status","status":"dropped","orig_msg_id":"a","dropped_msg_ids":["b","a"]}`, []string{"a", "b"}, inbox.Failed},
		{`{"type":"control","action":"peer_message_status","status":"pondering","orig_msg_id":"a"}`, nil, ""},
		{`{"type":"user","status":"held","orig_msg_id":"a"}`, nil, ""},
		{`not json`, nil, ""},
	}
	for _, c := range cases {
		words := parseReceipt([]byte(c.line))
		var ids []string
		for _, w := range words {
			ids = append(ids, w.msgID)
			if w.result.State != c.state {
				t.Errorf("%s: state %q, want %q", c.line, w.result.State, c.state)
			}
		}
		if strings.Join(ids, ",") != strings.Join(c.ids, ",") {
			t.Errorf("%s: ids %v, want %v", c.line, ids, c.ids)
		}
	}
}

func uuidPattern(v any) bool {
	s, _ := v.(string)
	parts := strings.Split(s, "-")
	return len(parts) == 5 && len(parts[0]) == 8 && len(parts[4]) == 12 && parts[2][0] == '4'
}
