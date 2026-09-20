package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func passiveNoticeDisplay(t *testing.T) (*connection, Binding) {
	t.Helper()
	g := New(Config{Complete: func(Completion) { t.Error("cosmetic event created outcome") }, ReadSequence: func() uint64 { t.Error("cosmetic event read accounting"); return 0 }, Record: func(Record) { t.Error("cosmetic event reached observation") }})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	c := &connection{owner: g, ctx: ctx, cancel: cancel, toUI: make(chan []byte, 8), state: newState("epoch", 1), admitted: newAdmittedWork()}
	b := Binding{Epoch: "epoch", Connection: 1, Generation: 3, Thread: "primary", Ready: true}
	c.state.Binding = b
	g.current = c
	g.owners[c] = true
	g.conns[c] = true
	return c, b
}

func noticeDisplayParams(t *testing.T, raw []byte) map[string]json.RawMessage {
	t.Helper()
	var value struct {
		Method string                     `json:"method"`
		Params map[string]json.RawMessage `json:"params"`
	}
	if json.Unmarshal(raw, &value) != nil || value.Method != "item/completed" || len(value.Params) != 4 {
		t.Fatalf("invalid UI envelope: %s", raw)
	}
	var item map[string]json.RawMessage
	if json.Unmarshal(value.Params["item"], &item) != nil || len(item) != 13 {
		t.Fatalf("invalid UI item: %s", raw)
	}
	for key, want := range map[string]string{"type": `"commandExecution"`, "command": `"rewake notice --display-only"`, "cwd": `"/"`, "source": `"agent"`, "status": `"completed"`, "commandActions": `[]`, "exitCode": `0`, "processId": `null`, "pluginId": `null`, "scriptPath": `null`, "durationMs": `null`} {
		if string(item[key]) != want {
			t.Fatalf("bad %s: %s", key, raw)
		}
	}
	var id string
	_ = json.Unmarshal(item["id"], &id)
	if !strings.HasPrefix(id, "rewake-notice-display-") || len(id) != len("rewake-notice-display-")+32 {
		t.Fatalf("unscoped call ID: %s", id)
	}
	return value.Params
}

func TestNoticeDisplayBindingAndPressure(t *testing.T) {
	for _, mode := range []string{"side", "epoch", "generation", "connection", "thread", "not-ready", "other-owner", "conflict", "closed", "canceled", "items", "bytes"} {
		t.Run(mode, func(t *testing.T) {
			c, b := passiveNoticeDisplay(t)
			switch mode {
			case "side":
				c.state.side = "other-thread"
			case "epoch":
				c.state.Epoch = "new"
			case "generation":
				c.state.Generation++
			case "connection":
				c.state.Connection++
			case "thread":
				c.state.Thread = "other"
			case "not-ready":
				c.state.Ready = false
			case "other-owner":
				c.owner.current = &connection{}
			case "conflict":
				c.owner.owners[&connection{}] = true
			case "closed":
				c.owner.closed = true
			case "canceled":
				c.cancel()
			case "items":
				for i := 0; i < 4; i++ {
					c.toUI <- []byte("native")
				}
			case "bytes":
				c.responseBytes.Store(transportQueueBytes - (1 << 20) + 1)
			}
			items, bytes := len(c.toUI), c.responseBytes.Load()
			if got := c.displayNotice(b, "ack-turn", "short notice"); got != (mode == "side") {
				t.Fatalf("publication=%v", got)
			}
			if mode == "side" {
				raw := <-c.toUI
				params := noticeDisplayParams(t, raw)
				if string(params["threadId"]) != `"primary"` || string(params["turnId"]) != `"ack-turn"` {
					t.Fatal("retargeted cosmetic event")
				}
			} else if len(c.toUI) != items || c.responseBytes.Load() != bytes {
				t.Fatal("rejection consumed native queue capacity")
			}
			if len(c.admitted.pending) != 0 || len(c.state.pending) != 0 || len(c.state.events.watches) != 0 {
				t.Fatal("cosmetic event modified accounting")
			}
		})
	}
}

func TestNoticeDisplayUsesACKAndCannotFinishActiveWork(t *testing.T) {
	completed := make(chan Completion, 4)
	g, ui, peers, _ := setupConfig(t, Config{Complete: func(v Completion) { completed <- v }})
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	bindUI(t, g, ui, native)
	// These are real upstream notifications in the local protocol fixture.
	started := []byte(`{"method":"item/started","params":{"threadId":"A","turnId":"work","startedAtMs":1,"item":{"type":"commandExecution","id":"native-call","command":"sleep 1","cwd":"/","source":"agent","status":"inProgress","commandActions":[]}}}`)
	write(t, native, started)
	if string(readWithin(t, ui)) != string(started) {
		t.Fatal("native start changed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	r, err := g.Reserve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	done := make(chan error, 1)
	go func() { _, e := r.Deliver(ctx, "mail", MailboxNotice{Notice: "one notice"}, nil); done <- e }()
	request := metadata(t, string(readWithin(t, native)))
	if request.method != "turn/start" {
		t.Fatal(request)
	}
	select {
	case v := <-completed:
		t.Fatalf("premature outcome: %+v", v)
	default:
	}
	write(t, native, []byte(fmt.Sprintf(`{"id":%q,"result":{"turn":{"id":"work"}}}`, request.idText)))
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	params := noticeDisplayParams(t, readWithin(t, ui))
	if string(params["threadId"]) != `"A"` || string(params["turnId"]) != `"work"` {
		t.Fatal("invented scope")
	}
	select {
	case v := <-completed:
		t.Fatalf("UI item completed native work: %+v", v)
	default:
	}
	if _, err := r.Deliver(ctx, "mail", MailboxNotice{Notice: "again"}, nil); err == nil {
		t.Fatal("replayed delivery")
	}
	r.Close()
	// A marker traverses upstream next: no UI frame or duplicate admission can hide.
	exchange(t, ui, native, `{"id":88,"method":"thread/goal/get","params":{"threadId":"A"}}`, `{"id":88,"result":{}}`)
	terminal := []byte(`{"method":"turn/completed","params":{"threadId":"A","turn":{"id":"work","status":"completed"}}}`)
	write(t, native, terminal)
	if string(readWithin(t, ui)) != string(terminal) {
		t.Fatal("native terminal replaced by cosmetic event")
	}
	select {
	case v := <-completed:
		if v.Kind != "finished" || v.ID != "A/work" {
			t.Fatal(v)
		}
	case <-time.After(time.Second):
		t.Fatal("native report lost")
	}
	select {
	case v := <-completed:
		t.Fatalf("duplicate report: %+v", v)
	default:
	}
}

func TestNoticeDisplayPostACKScopeLossKeepsDeliveryAccepted(t *testing.T) {
	g, ui, peers, _ := setupConfig(t, Config{})
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	bindUI(t, g, ui, native)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	r, err := g.Reserve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	done := make(chan error, 1)
	go func() { _, e := r.Deliver(ctx, "mail", MailboxNotice{Notice: "notice"}, nil); done <- e }()
	request := metadata(t, string(readWithin(t, native)))
	closed := []byte(`{"method":"thread/closed","params":{"threadId":"A"}}`)
	write(t, native, closed)
	_ = readWithin(t, ui)
	write(t, native, []byte(fmt.Sprintf(`{"id":%q,"result":{"turn":{"id":"accepted"}}}`, request.idText)))
	if err := <-done; err != nil {
		t.Fatalf("cosmetic refusal changed accepted delivery: %v", err)
	}
	marker := []byte(`{"method":"fixture/marker"}`)
	write(t, native, marker)
	if string(readWithin(t, ui)) != string(marker) {
		t.Fatal("stale cosmetic event leaked")
	}
}

func TestNoticeDisplayQueueFailureKeepsACKAccepted(t *testing.T) {
	g, ui, peers, _ := setupConfig(t, Config{})
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	bindUI(t, g, ui, native)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	r, err := g.Reserve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	done := make(chan error, 1)
	go func() { _, e := r.Deliver(ctx, "mail", MailboxNotice{Notice: "notice"}, nil); done <- e }()
	request := metadata(t, string(readWithin(t, native)))
	c := g.currentConnection()
	c.responseBytes.Store(transportQueueBytes)
	write(t, native, []byte(fmt.Sprintf(`{"id":%q,"result":{"turn":{"id":"accepted"}}}`, request.idText)))
	if err := <-done; err != nil {
		t.Fatalf("cosmetic queue failure rejected delivered mail: %v", err)
	}
	if !g.Binding().Ready {
		t.Fatal("cosmetic pressure closed transport")
	}
	c.responseBytes.Store(0)
	r.Close()
	exchange(t, ui, native, `{"id":89,"method":"thread/goal/get","params":{"threadId":"A"}}`, `{"id":89,"result":{}}`)
}

func TestNoticeDisplayCallIDsAndBounds(t *testing.T) {
	c, b := passiveNoticeDisplay(t)
	seen := map[string]bool{}
	for i := 0; i < 64; i++ {
		if !c.displayNotice(b, "turn", "notice") {
			t.Fatal("valid probe dropped")
		}
		raw := <-c.toUI
		c.responseBytes.Add(-int64(cap(raw)))
		p := noticeDisplayParams(t, raw)
		id := string(field(p["item"], "id"))
		if seen[id] {
			t.Fatal("reused call ID")
		}
		seen[id] = true
	}
	if c.displayNotice(b, "", "notice") || c.displayNotice(b, "turn", strings.Repeat("x", (64<<10)+1)) {
		t.Fatal("invalid cosmetic payload accepted")
	}
	if len(c.toUI) != 0 || c.responseBytes.Load() != 0 {
		t.Fatal("discarded payload retained")
	}
}
