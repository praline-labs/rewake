package modhost

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/control"
	"github.com/praline-labs/rewake/internal/core/mirror"
)

// A run the face draws says so in its hello answer, takes the current
// load's events into the model and refuses another load's; a run it does
// not draw says nothing and takes none.
func TestMirrorEventsReachTheModelOfTheCurrentLoadOnly(t *testing.T) {
	model := mirror.New()
	_, socket := servingWith(t, func(server *Server) { server.cfg.Mirror = model })
	var hello HelloAnswer
	if err := json.Unmarshal([]byte(postValue(t, socket, PathHello, HelloRequest{LoadID: "load", SessionID: "conversation-1", HarnessVersion: "test"})), &hello); err != nil {
		t.Fatal(err)
	}
	if hello.Mirror == nil || !*hello.Mirror {
		t.Fatalf("the hello answer of a drawn run: %+v", hello)
	}
	id, text, block := "b1", "Hello", MirrorBlockText
	postValue(t, socket, PathMirrorEvents, MirrorBatch{LoadID: "load", Generation: 1, Events: []MirrorEvent{{Kind: MirrorEventKindDelta, ID: &id, Block: &block, Text: &text}}})
	body, _ := json.Marshal(MirrorBatch{LoadID: "other", Generation: 1, Events: []MirrorEvent{{Kind: MirrorEventKindDelta, ID: &id, Text: &text}}})
	if status, _, _ := post(socket, PathMirrorEvents, body); status != http.StatusConflict {
		t.Fatalf("another load's batch: %d", status)
	}
	if got := snapshotTexts(t, model); len(got) != 1 || got[0] != "Hello" {
		t.Fatalf("the model holds %q", got)
	}

	_, plain := serving(t)
	var plainHello HelloAnswer
	if err := json.Unmarshal([]byte(postValue(t, plain, PathHello, HelloRequest{LoadID: "load", SessionID: "conversation-1", HarnessVersion: "test"})), &plainHello); err != nil {
		t.Fatal(err)
	}
	if plainHello.Mirror != nil {
		t.Fatal("a run the face does not draw asks for its conversation")
	}
	if status, _, _ := post(plain, PathMirrorEvents, body); status != http.StatusNotFound {
		t.Fatalf("a batch to a run the face does not draw: %d", status)
	}
}

// A prompt of the face rides the next heartbeat with its text; its item is the
// mod's to send, so the host adds none on the mod's answer.
func TestAFacePromptRidesTheHeartbeat(t *testing.T) {
	model := mirror.New()
	_, socket := servingWith(t, func(server *Server) {
		// As New takes the face's prompts of a run it is given a model.
		server.cfg.Mirror = model
		model.Take(server.Prompt)
		server.cfg.Hold = 10 * time.Millisecond
	})
	postValue(t, socket, PathHello, HelloRequest{LoadID: "load", SessionID: "conversation-1", HarnessVersion: "test"})
	result := make(chan error, 1)
	go func() {
		result <- model.Input(context.Background(), mirror.Input{ID: "i1", Kind: mirror.InputKindPrompt, Text: "go on"})
	}()
	var command *ControlCommand
	for deadline := time.Now().Add(2 * time.Second); command == nil && time.Now().Before(deadline); {
		var answer AliveAnswer
		if err := json.Unmarshal([]byte(postValue(t, socket, PathModAlive, AliveRequest{LoadID: "load", Generation: 1})), &answer); err != nil {
			t.Fatal(err)
		}
		command = answer.Control
	}
	if command == nil || command.Action != actionPrompt || command.Text == nil || *command.Text != "go on" || command.Conversation != "conversation-1" {
		t.Fatalf("command: %+v", command)
	}
	postValue(t, socket, PathModControl, ControlReply{LoadID: "load", Generation: 1, ID: command.ID, Outcome: control.Done})
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if got := snapshotTexts(t, model); len(got) != 0 {
		t.Fatalf("the model holds %q", got)
	}
}

// snapshotTexts reads the model as a face would, by its snapshot.
func snapshotTexts(t *testing.T, model *mirror.Model) []string {
	t.Helper()
	var texts []string
	for _, line := range model.Snapshot() {
		if line.Op == mirror.LineOpItem {
			texts = append(texts, line.Item.Text)
		}
	}
	return texts
}
