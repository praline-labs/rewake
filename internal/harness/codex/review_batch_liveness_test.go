package codex

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/role"
)

type reviewBatchFixture struct {
	dir      string
	server   *inbox.Server
	backend  *serverSession
	native   net.Conn
	captured <-chan map[string]json.RawMessage
}

func reviewBatches(t *testing.T) (*reviewBatchFixture, []inbox.Message) {
	t.Helper()
	peers := make(chan net.Conn, 1)
	backend, captured := gitDeliveryFixtureTraffic(t, role.Write, "active", nil, func(c net.Conn) { peers <- c })
	f := &reviewBatchFixture{dir: t.TempDir(), backend: backend, native: <-peers, captured: captured}
	// The stock socket fixture expires after five seconds. This regression
	// intentionally spans seven seconds of arrivals plus processing time.
	if err := f.native.SetDeadline(time.Now().Add(15 * time.Second)); err != nil {
		t.Fatal(err)
	}
	f.server = &inbox.Server{Dir: f.dir, Name: "receiver", Epoch: "epoch", Reserve: backend.Reserve}
	first := []inbox.Message{f.put(t, inbox.Note, "old-one"), f.put(t, inbox.Note, "old-two")}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); f.server.Serve(ctx) }()
	t.Cleanup(func() { cancel(); <-done })
	f.notice(t, first)
	f.wait(t, first, inbox.Delivered)
	return f, first
}

func (f *reviewBatchFixture) put(t *testing.T, kind inbox.Kind, text string) inbox.Message {
	t.Helper()
	m := inbox.Message{ID: inbox.NewID(), From: "sender", FromEpoch: "sender-run", To: f.server.Name, ToEpoch: f.server.Epoch, Kind: kind, CreatedAt: time.Now(), Text: text}
	if err := inbox.Put(f.dir, m); err != nil {
		t.Fatal(err)
	}
	return m
}

func (f *reviewBatchFixture) wait(t *testing.T, members []inbox.Message, want inbox.State) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		all := true
		for _, m := range members {
			st, ok := inbox.ReadStatus(f.dir, m.To, m.ID)
			all = all && ok && st.State == want
		}
		if all {
			return
		}
		if time.Now().After(deadline) {
			for _, m := range members {
				st, ok := inbox.ReadStatus(f.dir, m.To, m.ID)
				t.Logf("member %s kind=%s statusKnown=%v status=%+v", m.Text, m.Kind, ok, st)
			}
			t.Logf("binding=%+v", f.backend.gateway.Binding())
			t.Fatalf("members did not become %s", want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (f *reviewBatchFixture) notice(t *testing.T, members []inbox.Message) {
	t.Helper()
	select {
	case params := <-f.captured:
		var id string
		notice := decodedMailbox(t, params)
		if json.Unmarshal(params["clientUserMessageId"], &id) != nil {
			t.Fatal("invalid native admission identity")
		}
		if len(notice.Members) != len(members) {
			t.Fatal("wrong member count")
		}
		for i, member := range members {
			got := notice.Members[i]
			if got.ID != member.ID || got.From != member.From || got.FromEpoch != member.FromEpoch || got.To != member.To || got.ToEpoch != member.ToEpoch {
				t.Fatalf("wrong member: %+v", got)
			}
		}
		want := members[0].ID
		if len(members) > 1 {
			h := sha256.New()
			for _, m := range members {
				_, _ = fmt.Fprintf(h, "%s\x00", m.ID)
			}
			want = fmt.Sprintf("group-%x", h.Sum(nil))
		}
		if id != want || !strings.Contains(notice.Notice, fmt.Sprintf("%d new message", len(members))) || !strings.Contains(notice.Notice, members[len(members)-1].Text) {
			t.Fatalf("wrong fixed notice %s: %s", id, notice.Notice)
		}
		if strings.Contains(notice.Notice, "--peek") {
			t.Fatal("long usage line returned")
		}
	case <-time.After(2500 * time.Millisecond):
		for _, m := range members {
			st, known := inbox.ReadStatus(f.dir, m.To, m.ID)
			t.Logf("unannounced %s: known=%v status=%+v", m.Text, known, st)
		}
		t.Fatal("new queued mail did not wake idle recipient")
	}
}

func (f *reviewBatchFixture) emit(turn, status string) {
	serverMessage(f.native, map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": fixtureRoot, "turn": map[string]string{"id": turn, "status": status}}})
}

func TestReviewSeparatedArrivalsDispatchWhileTurnStaysActive(t *testing.T) {
	f, _ := reviewBatches(t)
	serverMessage(f.native, map[string]any{"method": "turn/started", "params": map[string]any{"threadId": fixtureRoot, "turn": map[string]string{"id": "delivered-turn-1"}}})
	for i, delay := range []time.Duration{0, 2 * time.Second, 3 * time.Second, 2 * time.Second} {
		if delay > 0 {
			timer := time.NewTimer(delay)
			<-timer.C
		}
		next := f.put(t, []inbox.Kind{inbox.Task, inbox.Question, inbox.Error, inbox.Stopped}[i], fmt.Sprintf("new-%d", i))
		f.notice(t, []inbox.Message{next})
		f.wait(t, []inbox.Message{next}, inbox.Delivered)
		unread, err := inbox.PeekUnread(f.dir, f.server.Name, f.server.Epoch)
		if err != nil || len(unread) != 3+i {
			t.Fatal("independent old/new mail lost", err)
		}
	}
	// Every spaced arrival has already been admitted. Future mail cannot extend an
	// immutable accepted notice; no peek or terminal event was needed for dispatch.
	select {
	case <-f.captured:
		t.Fatal("old unread replay")
	default:
	}
}

func TestReviewNewMailAfterFailedTurnMustNotRequireHuman(t *testing.T) {
	f, _ := reviewBatches(t)
	next := f.put(t, inbox.Note, "independent-new-mail")
	f.emit("delivered-turn-1", "failed")
	serverMessage(f.native, map[string]any{"method": "thread/status/changed", "params": map[string]any{"threadId": fixtureRoot, "status": map[string]string{"type": "idle"}}})
	// This is NEW mail, not a replay of the failed turn or its original members.
	f.notice(t, []inbox.Message{next})
}
