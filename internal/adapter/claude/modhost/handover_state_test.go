package modhost

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/control"
	"github.com/praline-labs/rewake/internal/core/session"
)

// adopted is the next image serving what before handed over, before's
// listener closed.
func adopted(t *testing.T, before *Server, socket string, handed Handed, file *os.File, c *core) *Server {
	t.Helper()
	_ = before.listener.UnixListener.Close()
	after := New(Config{Socket: socket, Role: "main", Wait: 2 * time.Second, Hold: 20 * time.Millisecond, Record: filepath.Join(filepath.Dir(socket), "after.json")})
	if c != nil {
		after.ReportTurns(c.handler())
	}
	if err := after.Adopt(handed, file); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(after.Close)
	return after
}

// Review of b832ce0: a state is the cache observation the handover carries,
// so one between the snapshot and the exec is refused like any effect, and
// the mod sends it again to the next image; acknowledged by the image going
// away, a step and the placeholder after it would leave the next image the
// older observation.
func TestAStateAfterTheHandoversSnapshotGoesToTheNextImage(t *testing.T) {
	before, socket := servingWith(t, func(s *Server) { s.cfg.Hold = 20 * time.Millisecond })
	postValue(t, socket, PathHello, HelloRequest{Protocol: Protocol, LoadID: "load", SessionID: "conversation-1", HarnessVersion: "test"})
	postValue(t, socket, PathModState, step(1, usage(100, 50, 50), nil))
	handed, file, err := before.HandOver()
	if err != nil {
		t.Fatal(err)
	}
	later, _ := json.Marshal(step(2, usage(900, 50, 50), nil))
	if status, answer, err := post(socket, PathModState, later); err != nil || status != http.StatusServiceUnavailable {
		t.Fatalf("a state after the snapshot: %d %s %v", status, answer, err)
	}
	cacheIs(t, before, &session.Cache{HitPercent: 50, Read: 100, Write: 50, Input: 50})
	after := adopted(t, before, socket, handed, file, nil)
	postValue(t, socket, PathModState, step(2, usage(900, 50, 50), nil))
	postValue(t, socket, PathModState, step(3, usage(0, 0, 0), nil))
	cacheIs(t, after, &session.Cache{HitPercent: 90, Read: 900, Write: 50, Input: 50})
}

// Review of b832ce0: the conversation the last state named crosses the
// handover, so a late interrupted end in the conversation the session moved
// to still has its trace appended there.
func TestALateTraceAfterAConversationChangeCrossesTheHandover(t *testing.T) {
	before, socket, _ := turning(t)
	moved := step(1, nil, nil)
	moved.LoadID, moved.Conversation, moved.Activity, moved.Turn = "load-1", "conversation-2", "working", ptr("turn-1")
	postValue(t, socket, PathModState, moved)
	answered := make(chan control.Answer, 1)
	go func() {
		answered <- before.carry(context.Background(), control.Request{ID: "r", Action: control.Interrupt, From: "lead", Conversation: "conversation-2"})
	}()
	if command := commandOf(t, socket); command.Turn == nil || *command.Turn != "turn-1" || command.Conversation != "conversation-2" {
		t.Fatalf("the interrupt: %+v", command)
	} else {
		replyTo(t, socket, command, control.Requested)
	}
	if answer := answerOf(t, answered); answer.Outcome != control.Failed || !answer.Open {
		t.Fatalf("no end in time: %+v", answer)
	}
	handed, file, err := before.HandOver()
	if err != nil {
		t.Fatal(err)
	}
	c := &core{}
	adopted(t, before, socket, handed, file, c)
	end, _ := json.Marshal(TurnEndRequest{LoadID: "load-1", Conversation: "conversation-2", TurnID: "turn-1", EndID: "end-1", Reason: "aborted", Answer: ptr("")})
	if status, _ := sendEnd(t, socket, end); status != http.StatusOK {
		t.Fatalf("the end: %d", status)
	}
	if ends := c.published(); len(ends) != 1 || ends[0].Answer != control.Actor("lead") {
		t.Fatalf("the late report: %+v", ends)
	}
	if trace := commandOf(t, socket); trace.Action != actionTrace || trace.Conversation != "conversation-2" {
		t.Fatalf("the late trace: %+v", trace)
	}
}
