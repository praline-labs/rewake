package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/state"
)

func TestMixedGroupPeekThenSelectedReadAndReadAll(t *testing.T) {
	dir, self, peer := stateCaller(t, "write")
	clock, err := inbox.OpenReadClock(context.Background(), dir, self.Name, self.Epoch())
	if err != nil {
		t.Fatal(err)
	}
	defer clock.Close()
	var messages []inbox.Message
	for _, kind := range []inbox.Kind{inbox.Task, inbox.Question, inbox.Note, inbox.Finished} {
		m := inbox.Message{ID: inbox.NewID(), From: peer.Name, FromEpoch: peer.Epoch(), To: self.Name, ToEpoch: self.Epoch(), Kind: kind, CreatedAt: time.Now(), Text: strings.Repeat("preview", 40) + "\nPRIVATE FULL BODY " + string(kind)}
		if err := inbox.Put(dir, m); err != nil {
			t.Fatal(err)
		}
		messages = append(messages, m)
	}
	delivered := make(chan inbox.Message, 4)
	server := &inbox.Server{Dir: dir, Name: self.Name, Epoch: self.Epoch(), Thread: func() (string, error) { return "selected-thread", nil }, Deliver: func(_ context.Context, m inbox.Message) inbox.Result {
		delivered <- m
		return inbox.Result{State: inbox.Delivered}
	}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); server.Serve(ctx) }()
	t.Cleanup(func() { cancel(); <-done })
	select {
	case group := <-delivered:
		if len(group.Batch) != 4 || group.ID == messages[0].ID {
			t.Fatal("mixed mail was not one independent group")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no grouped announcement")
	}
	for range 2 {
		for _, asJSON := range []bool{false, true} {
			args := []string{"inbox", "--peek"}
			if asJSON {
				args = append(args, "--json")
			}
			code, out, stderr := run(args...)
			if code != ExitOK || strings.Contains(out, "PRIVATE FULL BODY") || strings.Contains(out, `"text"`) || strings.Contains(out, "telemetry") {
				t.Fatalf("unsafe peek: %d %s %s", code, out, stderr)
			}
			if asJSON {
				var model inboxPeekModel
				if err := json.Unmarshal([]byte(out), &model); err != nil {
					t.Fatal(err)
				}
				if len(model.Messages) != 4 {
					t.Fatal("peek lost members")
				}
				for i, p := range model.Messages {
					if p.ID != messages[i].ID || p.From != peer.Name || p.Kind != messages[i].Kind || len([]rune(p.Preview)) > 96 {
						t.Fatal("overview identity or preview bound changed")
					}
				}
			}
		}
	}
	if len(inbox.Waiters(dir, self.Name, self.Epoch())) != 0 || clock.Snapshot().Through != 0 {
		t.Fatal("peek created a waiter or advanced the read boundary")
	}
	if unread, err := inbox.PeekUnread(dir, self.Name, self.Epoch()); err != nil || len(unread) != 4 {
		t.Fatal("peek consumed mail")
	}
	code, out, stderr := run("inbox", "--message", messages[0].ID)
	if code != ExitOK || !strings.Contains(out, messages[0].Text) || strings.Contains(out, "PRIVATE FULL BODY question") {
		t.Fatalf("selected view: %d %s %s", code, out, stderr)
	}
	if unread, err := inbox.PeekUnread(dir, self.Name, self.Epoch()); err != nil || len(unread) != 3 {
		t.Fatal("selected read consumed neighbors")
	}
	if len(inbox.Waiters(dir, self.Name, self.Epoch())) != 1 || clock.Snapshot().Through != 1 {
		t.Fatal("selected task did not create exactly one obligation")
	}
	code, out, stderr = run("inbox", "--json")
	var all inboxModel
	if code != ExitOK || json.Unmarshal([]byte(out), &all) != nil || len(all.Messages) != 3 {
		t.Fatalf("old read-all: %d %s %s", code, out, stderr)
	}
	if unread, _ := inbox.PeekUnread(dir, self.Name, self.Epoch()); len(unread) != 0 {
		t.Fatal("read-all left available mail")
	}
	select {
	case <-delivered:
		t.Fatal("group was announced twice")
	default:
	}
}

func TestInboxSelectionRefusesReservedOldAndInvalidIDs(t *testing.T) {
	dir, self, peer := stateCaller(t, "general")
	id := rawUnread(t, dir, self.Name, map[string]any{"from": peer.Name, "fromEpoch": peer.Epoch(), "toEpoch": self.Epoch(), "kind": "finished", "inReplyTo": []string{"question"}, "text": "reserved body"})
	release, err := inbox.ReserveAnswer(dir, self.Name, "question")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	old := rawUnread(t, dir, self.Name, map[string]any{"toEpoch": "old-epoch", "kind": "task", "text": "old body"})
	for _, args := range [][]string{{"inbox", "--message", id}, {"inbox", "--message", old}, {"inbox", "--message", "unknown"}, {"inbox", "--message", "../escape"}, {"inbox", "--message", ""}, {"inbox", "--peek", "--message", id}, {"inbox", "--peek=false"}} {
		code, out, stderr := run(args...)
		if code == ExitOK || strings.Contains(out, "body") || !strings.Contains(stderr, "inbox") {
			t.Fatalf("selection accepted: %v %d %s %s", args, code, out, stderr)
		}
	}
	_, out, _ := run("inbox", "--peek", "--json")
	if strings.Contains(out, id) || strings.Contains(out, "body") {
		t.Fatal("peek stole reserved answer")
	}
	if _, err := os.Stat(filepath.Join(state.UnreadPath(dir, self.Name), id+".json")); err != nil {
		t.Fatal("refusal consumed answer")
	}
}

func TestPeekTelemetryIsMainOnlyAndBodyFree(t *testing.T) {
	for _, role := range []string{"main", "write", "general"} {
		t.Run(role, func(t *testing.T) {
			dir, self, peer := stateCaller(t, role)
			leaveUnread(t, dir, inbox.Message{From: peer.Name, FromEpoch: peer.Epoch(), To: self.Name, ToEpoch: self.Epoch(), Text: "preview\nFULL SECRET", Kind: inbox.Note})
			for _, asJSON := range []bool{false, true} {
				args := []string{"inbox", "--peek"}
				if asJSON {
					args = append(args, "--json")
				}
				code, out, stderr := run(args...)
				visible := strings.Contains(out, `"telemetry"`) || strings.Contains(out, " | context")
				if code != ExitOK || visible != (role == "main") || strings.Contains(out, "FULL SECRET") {
					t.Fatalf("peek visibility: %d %s %s", code, out, stderr)
				}
			}
		})
	}
}

func TestSelectedReadPrintsBeforeMarkingAndKeepsNormalJSON(t *testing.T) {
	dir, self, peer := stateCaller(t, "main")
	id := rawUnread(t, dir, self.Name, map[string]any{"from": peer.Name, "fromEpoch": peer.Epoch(), "toEpoch": self.Epoch(), "kind": "task", "text": "selected body"})
	if code := Run([]string{"inbox", "--message", id}, brokenWriter{}, brokenWriter{}); code == ExitOK {
		t.Fatal("selected read ignored output failure")
	}
	if status, ok := inbox.ReadStatus(dir, self.Name, id); ok && status.State == inbox.Read {
		t.Fatal("failed output consumed selected message")
	}
	code, out, stderr := run("inbox", "--message", id, "--json")
	var model inboxModel
	if code != ExitOK || json.Unmarshal([]byte(out), &model) != nil || len(model.Messages) != 1 || model.Messages[0].Text != "selected body" || model.Messages[0].Telemetry == nil {
		t.Fatalf("selected normal JSON lost: %d %s %s", code, out, stderr)
	}
	if code, _, _ := run("inbox", "--message", id); code == ExitOK {
		t.Fatal("selected message read twice")
	}
}
