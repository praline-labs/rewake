package modhost

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/control"
)

// commandOf polls as load-1 until the host gives a command.
func commandOf(t *testing.T, socket string) *ControlCommand {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var answer AliveAnswer
		if err := json.Unmarshal([]byte(postValue(t, socket, PathModAlive, AliveRequest{LoadID: "load-1", Generation: 1})), &answer); err != nil {
			t.Fatal(err)
		}
		if answer.Control != nil {
			return answer.Control
		}
	}
	t.Fatal("the host gave no command")
	return nil
}

func replyTo(t *testing.T, socket string, command *ControlCommand, outcome string) {
	t.Helper()
	postValue(t, socket, PathModControl, ControlReply{LoadID: "load-1", Generation: 1, ID: command.ID, Outcome: outcome})
}

// interrupting starts main's interrupt of the turn the mod started.
func interrupting(t *testing.T, server *Server, socket string) chan control.Answer {
	t.Helper()
	postValue(t, socket, PathTurnStart, TurnStartRequest{LoadID: "load-1", Conversation: "conversation-1", TurnID: "turn-1"})
	answered := make(chan control.Answer, 1)
	go func() {
		answered <- server.carry(context.Background(), control.Request{ID: "r", Action: control.Interrupt, From: "lead"})
	}()
	return answered
}

func answerOf(t *testing.T, answered chan control.Answer) control.Answer {
	t.Helper()
	select {
	case answer := <-answered:
		return answer
	case <-time.After(5 * time.Second):
		t.Fatal("the interrupt was not answered")
	}
	return control.Answer{}
}

// The host names the running turn, the mod aborts it and says requested,
// and the turn's aborted end on the outbox settles it: the report names who
// stopped it, and the mod is asked to trace the same words.
func TestAnInterruptSettlesOnTheAbortedEndOfTheTurnItNamed(t *testing.T) {
	server, socket, c := turning(t)
	answered := interrupting(t, server, socket)
	command := commandOf(t, socket)
	if command.Action != control.Interrupt || command.Turn == nil || *command.Turn != "turn-1" {
		t.Fatalf("the interrupt: %+v", command)
	}
	replyTo(t, socket, command, control.Requested)
	if status, _ := sendEnd(t, socket, endBody("end-1", "aborted", "half an answer")); status != 200 {
		t.Fatalf("the end: %d", status)
	}
	if ends := c.published(); len(ends) != 1 || ends[0].Answer != "lead interrupted this turn with rewake interrupt" {
		t.Fatalf("the report: %+v", ends)
	}
	trace := commandOf(t, socket)
	if trace.Action != actionTrace || trace.Text == nil || *trace.Text != "lead interrupted this turn with rewake interrupt" || trace.Conversation != "conversation-1" {
		t.Fatalf("the trace: %+v", trace)
	}
	replyTo(t, socket, trace, control.Done)
	if answer := answerOf(t, answered); answer.Outcome != control.Done {
		t.Fatalf("the interrupt's answer: %+v", answer)
	}
	if why := server.Busy(); why != "" {
		t.Fatalf("busy after the interrupt settled: %s", why)
	}
}

// A turn that ended otherwise was not stopped by the interrupt: its report
// is its own. A trace the mod could not append says so.
func TestAnInterruptIsAnsweredByHowItsTurnEnded(t *testing.T) {
	server, socket, c := turning(t)
	answered := interrupting(t, server, socket)
	replyTo(t, socket, commandOf(t, socket), control.Requested)
	sendEnd(t, socket, endBody("end-1", "answered", "done anyway"))
	if answer := answerOf(t, answered); answer.Outcome != control.Refused || answer.Reason != control.NoTurn {
		t.Fatalf("an answered turn: %+v", answer)
	}
	if ends := c.published(); ends[0].Answer != "done anyway" {
		t.Fatalf("the report: %+v", ends)
	}

	answered = interrupting(t, server, socket)
	replyTo(t, socket, commandOf(t, socket), control.Requested)
	sendEnd(t, socket, endBody("end-2", "aborted", ""))
	postValue(t, socket, PathModControl, ControlReply{LoadID: "load-1", Generation: 1, ID: commandOf(t, socket).ID, Outcome: control.Failed, Native: json.RawMessage(`{"error":"denied"}`)})
	if answer := answerOf(t, answered); answer.Outcome != control.Failed || answer.Detail != control.TraceLost {
		t.Fatalf("a trace that failed: %+v", answer)
	}
}

// With no turn known, nothing goes to the mod; a refusal of the mod is
// worded here by its code.
func TestAnInterruptIsRefusedByTheHostOrWordedFromTheModsCode(t *testing.T) {
	server, socket, _ := turning(t)
	if answer := server.carry(context.Background(), control.Request{ID: "idle", Action: control.Interrupt, From: "lead"}); answer.Outcome != control.Refused || answer.Reason != control.NoTurn {
		t.Fatalf("an idle session: %+v", answer)
	}
	answered := interrupting(t, server, socket)
	code := ControlCodeTurn
	command := commandOf(t, socket)
	postValue(t, socket, PathModControl, ControlReply{LoadID: "load-1", Generation: 1, ID: command.ID, Outcome: control.Refused, Code: &code})
	if answer := answerOf(t, answered); answer.Outcome != control.Refused || answer.Reason != control.NoTurn {
		t.Fatalf("another turn running: %+v", answer)
	}
	postValue(t, socket, PathModState, StateRequest{LoadID: "load-1", Generation: 1, Sequence: 1, Conversation: "conversation-1", Activity: "idle"})
	if answer := server.carry(context.Background(), control.Request{ID: "after", Action: control.Interrupt, From: "lead"}); answer.Reason != control.NoTurn {
		t.Fatalf("a state naming no turn: %+v", answer)
	}
}

// No end within the settle wait leaves the outcome open; the end heard
// later, by the next image too, still carries the actor and its trace.
func TestALateEndInTheNextImageStillCarriesTheActor(t *testing.T) {
	before, socket, _ := turning(t)
	answered := interrupting(t, before, socket)
	replyTo(t, socket, commandOf(t, socket), control.Requested)
	answer := answerOf(t, answered)
	if answer.Outcome != control.Failed || !answer.Open || answer.Detail != control.NotSettled {
		t.Fatalf("no end in time: %+v", answer)
	}
	handed, file, err := before.HandOver()
	if err != nil {
		t.Fatal(err)
	}
	_ = before.listener.UnixListener.Close()
	after := New(Config{Socket: socket, Role: "main", Wait: 2 * time.Second, Hold: 20 * time.Millisecond, Record: filepath.Join(filepath.Dir(socket), "after.json")})
	c := &core{}
	after.ReportTurns(c.handler())
	if err := after.Adopt(handed, file); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(after.Close)
	sendEnd(t, socket, endBody("end-1", "aborted", ""))
	if ends := c.published(); len(ends) != 1 || ends[0].Answer != "lead interrupted this turn with rewake interrupt" {
		t.Fatalf("the late report: %+v", ends)
	}
	if trace := commandOf(t, socket); trace.Action != actionTrace {
		t.Fatalf("the late trace: %+v", trace)
	}
}

// A compaction's native result is worded here: the engine's skip is a
// refusal, a throw a failure, and the counts are the outcome's.
func TestACompactionsNativeResultIsWordedByTheHost(t *testing.T) {
	cases := []struct {
		reply ControlReply
		want  control.Answer
	}{
		{ControlReply{Outcome: control.Done, Native: json.RawMessage(`{"skip":"nothing to compact"}`)}, control.Answer{Outcome: control.Refused, Detail: "nothing to compact"}},
		{ControlReply{Outcome: control.Failed, Native: json.RawMessage(`{"error":"boom"}`)}, control.Answer{Outcome: control.Failed, Reason: control.CutShort, Detail: "native compaction failed or was interrupted"}},
		{ControlReply{Outcome: control.Done, Native: json.RawMessage(`"another shape"`)}, control.Answer{Outcome: control.Done}},
	}
	for _, c := range cases {
		if got := worded(control.Compact, c.reply); got != c.want {
			t.Errorf("%s: %+v, want %+v", c.reply.Native, got, c.want)
		}
	}
	got := worded(control.Compact, ControlReply{Outcome: control.Done, Native: json.RawMessage(`{"tokensBefore":25000,"tokensAfter":1000}`)})
	if got.TokensBefore == nil || *got.TokensBefore != 25000 || got.TokensAfter == nil || *got.TokensAfter != 1000 {
		t.Errorf("the counts: %+v", got)
	}
}
