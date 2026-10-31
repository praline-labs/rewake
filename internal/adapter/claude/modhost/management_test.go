package modhost

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/control"
	"github.com/praline-labs/rewake/internal/core/session"
)

func postValue(t *testing.T, socket, path string, value any) string {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	status, answer, err := post(socket, path, body)
	if err != nil || status != http.StatusOK {
		t.Fatalf("%s: %d %s %v", path, status, answer, err)
	}
	return answer
}

func testState() StateRequest {
	tokens, window, percent, model := 25000, 100000, 25, "test-model"
	return StateRequest{LoadID: "load", Generation: 1, Sequence: 1, Conversation: "conversation-1", Activity: "idle", Model: &model, Tokens: &tokens, Window: &window, Percent: &percent}
}

func TestStateIsCurrentGenerationOrderedAndUnknownAfterCompaction(t *testing.T) {
	server, socket := serving(t)
	postValue(t, socket, PathHello, HelloRequest{LoadID: "load", SessionID: "conversation-1", HarnessVersion: "test"})
	sample := testState()
	postValue(t, socket, PathModState, sample)
	got := server.SessionState()
	if !got.Fresh || got.ContextUsed == nil || *got.ContextUsed != 25000 || got.Compactions == nil || *got.Compactions != 0 {
		t.Fatalf("first sample: %+v", got)
	}
	sample.Sequence = 2
	sample.Compactions = 1
	sample.Tokens, sample.Percent = nil, nil
	sample.Request, sample.From = "r1", "main"
	postValue(t, socket, PathModState, sample)
	postValue(t, socket, PathModState, testState())
	got = server.SessionState()
	if got.ContextUsed != nil || got.FilledPercent != nil || *got.Compactions != 1 || len(got.CompactionEvents) != 1 || got.CompactionEvents[0].Request != "r1" {
		t.Fatalf("completion or stale sample: %+v", got)
	}
	postValue(t, socket, PathHello, HelloRequest{LoadID: "new", LoadSeq: 1, SessionID: "conversation-1", HarnessVersion: "test"})
	if server.SessionState().Fresh {
		t.Fatal("old load's snapshot stayed fresh")
	}
	body, _ := json.Marshal(sample)
	if status, _, _ := post(socket, PathModState, body); status != http.StatusConflict {
		t.Fatalf("old load state: %d", status)
	}
	sample.LoadID, sample.Generation, sample.Sequence, sample.Compactions = "new", 2, 1, 0
	postValue(t, socket, PathModState, sample)
	got = server.SessionState()
	if !got.Fresh || *got.Compactions != 1 || got.Coverage != "partial" {
		t.Fatalf("reload: %+v", got)
	}
	server.mu.Lock()
	arm := server.current.arm
	server.mu.Unlock()
	server.lapsed(2, arm)
	if server.SessionState().Fresh {
		t.Fatal("lapsed snapshot stayed fresh")
	}
	body, _ = json.Marshal(sample)
	if status, _, _ := post(socket, PathModState, body); status != http.StatusConflict {
		t.Fatalf("lapsed state: %d", status)
	}
}

func TestControlCrossesFilesHeartbeatAndFinalOutcome(t *testing.T) {
	server, socket := servingWith(t, func(server *Server) {
		server.cfg.ControlDir = filepath.Join(filepath.Dir(server.cfg.Record), "control")
		server.cfg.Hold = 10 * time.Millisecond
		if err := control.Prepare(server.cfg.ControlDir); err != nil {
			t.Fatal(err)
		}
	})
	server.StartControl(context.Background())
	postValue(t, socket, PathHello, HelloRequest{LoadID: "load", SessionID: "conversation-1", HarnessVersion: "test"})
	postValue(t, socket, PathModState, testState())
	result := make(chan control.Answer, 1)
	go func() {
		answer, err := control.Ask(context.Background(), server.cfg.ControlDir, control.Request{Action: control.Compact, From: "main", Focus: "keep the plan", Conversation: "conversation-1"}, control.Limits{Pickup: time.Second, Outcome: time.Second, Poll: time.Millisecond})
		if err != nil {
			result <- control.Answer{Detail: err.Error()}
			return
		}
		result <- answer
	}()
	var command *ControlCommand
	deadline := time.Now().Add(time.Second)
	for command == nil && time.Now().Before(deadline) {
		text := postValue(t, socket, PathModAlive, AliveRequest{LoadID: "load", Generation: 1})
		var answer AliveAnswer
		if err := json.Unmarshal([]byte(text), &answer); err != nil {
			t.Fatal(err)
		}
		command = answer.Control
	}
	if command == nil || command.Focus != "keep the plan" || command.From != "main" {
		t.Fatalf("command: %+v", command)
	}
	initial := ControlReply{LoadID: "load", Generation: 1, ID: command.ID, Outcome: control.Requested}
	postValue(t, socket, PathModControl, initial)
	select {
	case answer := <-result:
		if answer.Outcome != control.Requested {
			t.Fatalf("initial: %+v", answer)
		}
	case <-time.After(time.Second):
		t.Fatal("control was not answered")
	}
	before, after := 25000, 1000
	initial.Outcome, initial.TokensBefore, initial.TokensAfter = control.Done, &before, &after
	postValue(t, socket, PathModControl, initial)
	postValue(t, socket, PathModControl, initial)
	outcomes := server.SessionState().CompactionOutcomes
	if len(outcomes) != 1 || outcomes[0].RequestedBy != "main" || outcomes[0].TokensAfter == nil || *outcomes[0].TokensAfter != 1000 {
		t.Fatalf("final: %+v", outcomes)
	}
	if reply := postValue(t, socket, PathModAlive, AliveRequest{LoadID: "load", Generation: 1}); reply != "{}\n" {
		t.Fatalf("command was dispatched twice: %s", reply)
	}
}

func TestControlRefusesUnadmittedAndWrongConversation(t *testing.T) {
	server, socket := serving(t)
	request := control.Request{ID: "r", From: "main", Action: control.Compact}
	if answer := server.carry(context.Background(), request); answer.Reason != control.NotAnswering {
		t.Fatalf("before hello: %+v", answer)
	}
	postValue(t, socket, PathHello, HelloRequest{LoadID: "load", SessionID: "conversation-1", HarnessVersion: "test"})
	request.Conversation = "other"
	if answer := server.carry(context.Background(), request); answer.Reason != control.NotSelected {
		t.Fatalf("wrong conversation: %+v", answer)
	}
	body, _ := json.Marshal(ControlReply{LoadID: "load", Generation: 1, ID: "absent", Outcome: control.Done})
	if status, _, _ := post(socket, PathModControl, body); status != http.StatusConflict {
		t.Fatalf("unsolicited result: %d", status)
	}
}

func TestCanceledUndispatchedCommandCannotRunLater(t *testing.T) {
	server, socket := serving(t)
	postValue(t, socket, PathHello, HelloRequest{LoadID: "load", SessionID: "conversation-1", HarnessVersion: "test"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	answer := server.carry(ctx, control.Request{ID: "r", From: "main", Action: control.Compact})
	if answer.Outcome != control.Refused || answer.Open {
		t.Fatalf("not dispatched: %+v", answer)
	}
	if command := server.takeCommand(AliveRequest{LoadID: "load", Generation: 1}); command != nil {
		t.Fatalf("late command: %+v", command)
	}
}

func TestDispatchedControlWithoutAcknowledgementIsUnknown(t *testing.T) {
	server, socket := servingWith(t, func(server *Server) { server.cfg.Hold = 10 * time.Millisecond })
	postValue(t, socket, PathHello, HelloRequest{LoadID: "load", SessionID: "conversation-1", HarnessVersion: "test"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan control.Answer, 1)
	go func() { result <- server.carry(ctx, control.Request{ID: "r", From: "main", Action: control.Compact}) }()
	var command *ControlCommand
	deadline := time.Now().Add(time.Second)
	for command == nil && time.Now().Before(deadline) {
		text := postValue(t, socket, PathModAlive, AliveRequest{LoadID: "load", Generation: 1})
		var answer AliveAnswer
		if err := json.Unmarshal([]byte(text), &answer); err != nil {
			t.Fatal(err)
		}
		command = answer.Control
	}
	if command == nil {
		t.Fatal("no dispatched command")
	}
	cancel()
	select {
	case answer := <-result:
		if answer.Outcome != control.Failed || !answer.Open {
			t.Fatalf("dispatched without acknowledgement: %+v", answer)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled control did not return")
	}
	postValue(t, socket, PathModControl, ControlReply{LoadID: "load", Generation: 1, ID: "r", Outcome: control.Done})
	if outcomes := server.SessionState().CompactionOutcomes; len(outcomes) != 1 || outcomes[0].Outcome != control.Done {
		t.Fatalf("late outcome: %+v", outcomes)
	}
	postValue(t, socket, PathHello, HelloRequest{LoadID: "new", LoadSeq: 1, SessionID: "conversation-1", HarnessVersion: "test"})
	body, _ := json.Marshal(ControlReply{LoadID: "load", Generation: 1, ID: "r", Outcome: control.Done})
	if status, _, _ := post(socket, PathModControl, body); status != http.StatusConflict {
		t.Fatalf("old generation outcome: %d", status)
	}
}

// A dialog the mod names is told to main once while it lasts, again after
// a letter that could not be left, and anew once it cleared and another came.
func TestADialogIsToldToMainOncePerDialog(t *testing.T) {
	var told []string
	fail := true
	server, socket := servingWith(t, func(s *Server) {
		s.cfg.Dialog = func(message string) error {
			told = append(told, message)
			if fail {
				fail = false
				return errors.New("no main")
			}
			return nil
		}
	})
	postValue(t, socket, PathHello, HelloRequest{LoadID: "load", SessionID: "conversation-1", HarnessVersion: "test"})
	sample, waiting := testState(), "Claude needs your permission"
	for sequence, dialog := range []*string{&waiting, &waiting, &waiting, nil, &waiting} {
		sample.Sequence, sample.Dialog = sequence+1, dialog
		postValue(t, socket, PathModState, sample)
		if got := server.SessionState().Dialog; (got == nil) != (dialog == nil) {
			t.Fatalf("sample %d: dialog %v", sequence+1, got)
		}
	}
	if len(told) != 3 {
		t.Fatalf("told %q", told)
	}
}

// What a held dialog asks is the snapshot's while samples name it, and is
// gone with the first sample that does not.
func TestAHeldDialogsKindLastsWhileSamplesNameIt(t *testing.T) {
	server, socket := serving(t)
	postValue(t, socket, PathHello, HelloRequest{LoadID: "load", SessionID: "conversation-1", HarnessVersion: "test"})
	sample := testState()
	permission, question := AskKindPermission, AskKindQuestion
	for sequence, round := range []struct {
		waiting *AskKind
		want    string
	}{{&permission, session.WaitingPermission}, {&question, session.WaitingQuestion}, {nil, ""}} {
		sample.Sequence, sample.Waiting = sequence+1, round.waiting
		postValue(t, socket, PathModState, sample)
		if got := server.SessionState().Waiting; got != round.want {
			t.Fatalf("sample %d: waiting %q", sequence+1, got)
		}
	}
}

// A sample's directory and the account's windows reach the snapshot; a
// window without a kind or past a whole percent is no window, and a newer
// sample without them says the session no longer knows them.
func TestASampleCarriesTheDirectoryAndTheAccountsWindows(t *testing.T) {
	server, socket := serving(t)
	postValue(t, socket, PathHello, HelloRequest{LoadID: "load", SessionID: "conversation-1", HarnessVersion: "test"})
	sample := testState()
	cwd, resets := "/work/wt", 1_790_000_000
	sample.Cwd = &cwd
	sample.RateLimits = []RateLimit{{Kind: "five_hour", Percent: 37, ResetsAt: &resets}, {Kind: "seven_day", Percent: 9}, {Kind: "", Percent: 5}, {Kind: "odd", Percent: 101}}
	postValue(t, socket, PathModState, sample)
	got := server.SessionState()
	if got.Cwd == nil || *got.Cwd != cwd || len(got.RateLimits) != 2 {
		t.Fatalf("the sample: %+v", got)
	}
	if five := got.RateLimits[0]; five.Kind != "five_hour" || five.Percent != 37 || five.ResetsAt == nil || five.ResetsAt.Unix() != int64(resets) {
		t.Fatalf("the five hours: %+v", five)
	}
	if seven := got.RateLimits[1]; seven.Kind != "seven_day" || seven.ResetsAt != nil {
		t.Fatalf("the seven days: %+v", seven)
	}
	sample.Sequence, sample.Cwd, sample.RateLimits = 2, nil, nil
	postValue(t, socket, PathModState, sample)
	if got := server.SessionState(); got.Cwd != nil || got.RateLimits != nil {
		t.Fatalf("a sample without them: %+v", got)
	}
}
