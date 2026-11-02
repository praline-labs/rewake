package modhost

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/control"
	"github.com/praline-labs/rewake/internal/core/registry"
)

// A compaction main asked for fails at the mod while a switch waits for the
// run (the review of the live switch's fixes, November 2, 2026): its final
// reply settles the request, and the outcome is only in this image's
// observer, which the handover does not carry. The run stays busy until
// main's wrapper has read the outcome and forgotten the compaction; then it
// hands over. A compaction asked for past main's own bound holds nothing.
func TestAnOutcomeMainHasNotReadKeepsTheRunBusy(t *testing.T) {
	before, socket := servingWith(t, func(s *Server) {
		s.cfg.Hold = time.Millisecond
		s.cfg.RoomDir = filepath.Dir(filepath.Dir(s.cfg.Socket))
	})
	room := before.cfg.RoomDir
	id := control.NewID()
	release, err := control.Remember(room, "main", control.Pending{ID: id, Worker: registry.Session{Name: "worker"}, AskerEpoch: "1.1", AskedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	release()
	postValue(t, socket, PathHello, HelloRequest{LoadID: "load", SessionID: "conversation-1", HarnessVersion: "test"})
	postValue(t, socket, PathModState, testState())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	returned := make(chan control.Answer, 1)
	go func() { returned <- before.carry(ctx, control.Request{ID: id, Action: control.Compact, From: "main"}) }()
	var command *ControlCommand
	for deadline := time.Now().Add(time.Second); command == nil && time.Now().Before(deadline); {
		var answer AliveAnswer
		if err := json.Unmarshal([]byte(postValue(t, socket, PathModAlive, AliveRequest{LoadID: "load", Generation: 1})), &answer); err != nil {
			t.Fatal(err)
		}
		command = answer.Control
	}
	if command == nil {
		t.Fatal("the mod was never given the compaction")
	}
	reply := ControlReply{LoadID: "load", Generation: 1, ID: id, Outcome: control.Requested}
	postValue(t, socket, PathModControl, reply)
	if got := <-returned; got.Outcome != control.Requested {
		t.Fatalf("the acknowledgement: %+v", got)
	}
	reply.Outcome, reply.Detail = control.Failed, "native compaction failed"
	postValue(t, socket, PathModControl, reply)

	if why := before.Busy(); why == "" {
		t.Fatal("ready with the failure main has not read: the next image would publish no outcome over it")
	}
	if _, _, err := before.HandOver(); err == nil {
		t.Fatal("handed over with the failure main has not read")
	}

	// main's wrapper sends its letter, then forgets the compaction.
	if err := control.Forget(room, "main", id); err != nil {
		t.Fatal(err)
	}
	if why := before.Busy(); why != "" {
		t.Fatalf("busy once main has read the outcome: %s", why)
	}

	stale, err := control.Remember(room, "main", control.Pending{ID: id, Worker: registry.Session{Name: "worker"}, AskerEpoch: "1.1", AskedAt: time.Now().Add(-unreadBound)})
	if err != nil {
		t.Fatal(err)
	}
	stale()
	if why := before.Busy(); why != "" {
		t.Fatalf("busy for a compaction asked for past main's bound: %s", why)
	}
	if len(before.SessionState().CompactionOutcomes) != 1 {
		t.Fatalf("the outcome: %+v", before.SessionState().CompactionOutcomes)
	}
	if _, _, err := before.HandOver(); err != nil {
		t.Fatalf("no handover once main has read the outcome: %v", err)
	}
}
