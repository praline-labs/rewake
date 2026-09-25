package claude

import (
	"reflect"
	"strings"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/control"
)

// The module carrying requests out, under the host in plugin_control_host_test.go.

// An idle session compacts, with the focus as the instructions or with no
// argument at all, answers that it started, and tells rewake the counts at
// the end, for main's letter: the summary never leaves the module. Polling
// with nothing waiting asks whether the file exists and reads nothing, so the
// host logs no error.
func TestTheModuleCompactsAnIdleSession(t *testing.T) {
	run := runControl(t, true, controlWorld{}, tick, tick, asked(idA, control.Compact, "keep the plan"), tick, tick, asked(idB, control.Compact, ""), tick)
	if len(run.Errors) != 0 {
		t.Fatalf("host errors: %v", run.Errors)
	}
	if len(run.Compacts) != 2 || run.Compacts[0] == nil || *run.Compacts[0] != "keep the plan" || run.Compacts[1] != nil {
		t.Fatalf("compacted with %v", run.Compacts)
	}
	var ends []map[string]any
	for _, id := range []string{idA, idB} {
		if got, want := answerIn(t, run, id), map[string]any{"id": id, "outcome": "started"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("answer %v, want %v", got, want)
		}
		ends = append(ends, map[string]any{"plugin_event": "compact.ended", "request": id, "by": "lead", "outcome": "done", "tokensBefore": 120000.0, "tokensAfter": 9000.0})
	}
	if got := reported(run, "compact.ended"); !reflect.DeepEqual(got, ends) {
		t.Fatalf("reported %v, want %v", got, ends)
	}
	for name, content := range run.Files {
		if strings.Contains(content, "SECRET") {
			t.Fatalf("%s carries the summary: %s", name, content)
		}
	}
}

// The host's refusals come back as rewake's reasons with the host's text.
func TestTheModulePassesTheHostsRefusalsOn(t *testing.T) {
	busy := runControl(t, true, controlWorld{}, []any{"event", "turn.start", map[string]any{"turnId": "t1"}}, asked(idA, control.Compact, ""), tick)
	answer := answerIn(t, busy, idA)
	if answer["outcome"] != "refused" || answer["reason"] != control.InTurn || !strings.Contains(answer["detail"].(string), "a turn is running (t1)") || len(busy.Aborts) != 0 {
		t.Fatalf("mid-turn: %v, aborts %v", answer, busy.Aborts)
	}
	off := runControl(t, true, controlWorld{CompactOff: true}, asked(idA, control.Compact, ""), tick)
	if answer := answerIn(t, off, idA); answer["reason"] != control.CompactionOff {
		t.Fatalf("switched off: %v", answer)
	}
	short := runControl(t, true, controlWorld{TooShort: true}, asked(idA, control.Compact, ""), tick)
	if answer := answerIn(t, short, idA); answer["outcome"] != "refused" || answer["reason"] != control.NothingToCompact {
		t.Fatalf("too short: %v", answer)
	}
	flight := runControl(t, true, controlWorld{InFlight: true}, asked(idA, control.Compact, ""), tick)
	if answer := answerIn(t, flight, idA); answer["outcome"] != "refused" || answer["reason"] != control.InTurn || !strings.Contains(answer["detail"].(string), "a turn is in flight") {
		t.Fatalf("another compaction in flight: %v", answer)
	}
	for words, reason := range map[string]string{externalTurn: control.InTurn, thinClient: control.RemoteConversation} {
		run := runControl(t, true, controlWorld{Refuse: words}, asked(idA, control.Compact, ""), tick)
		if answer := answerIn(t, run, idA); answer["outcome"] != "refused" || answer["reason"] != reason || !strings.Contains(answer["detail"].(string), words) {
			t.Fatalf("%q: %v", words, answer)
		}
	}
	idle := runControl(t, true, controlWorld{}, asked(idA, control.Interrupt, ""), tick)
	if answer := answerIn(t, idle, idA); answer["reason"] != control.NoTurn || len(idle.Aborts) != 0 {
		t.Fatalf("interrupting an idle session: %v, aborts %v", answer, idle.Aborts)
	}
	ended := runControl(t, true, controlWorld{}, []any{"event", "turn.start", map[string]any{"turnId": "t1"}},
		[]any{"event", "turn.start", map[string]any{"turnId": "s1", "agentId": "a1"}}, asked(idA, control.Interrupt, ""), tick)
	if answer := answerIn(t, ended, idA); answer["outcome"] != "done" || !reflect.DeepEqual(ended.Aborts, []string{"t1"}) {
		t.Fatalf("a subagent's turn is not the one aborted: %v, aborts %v", answer, ended.Aborts)
	}
}

// An interrupt aborts the running turn, and its end is reported as the
// main's; a person's Esc afterwards is the person's.
func TestTheModuleReportsWhoInterrupted(t *testing.T) {
	run := runControl(t, true, controlWorld{},
		[]any{"event", "turn.start", map[string]any{"turnId": "t1"}}, asked(idA, control.Interrupt, ""), tick,
		[]any{"event", "turn.start", map[string]any{"turnId": "t2"}},
		[]any{"event", "turn.complete", map[string]any{"turnId": "t2", "reason": "aborted", "isAborted": true}})
	if answer := answerIn(t, run, idA); answer["outcome"] != "done" || !reflect.DeepEqual(run.Aborts, []string{"t1"}) {
		t.Fatalf("answer %v, aborts %v", answer, run.Aborts)
	}
	var ends []map[string]any
	for _, call := range run.Calls {
		if call["plugin_event"] == "turn.complete" {
			ends = append(ends, call)
		}
	}
	want := []map[string]any{
		{"plugin_event": "turn.complete", "turn_id": "t1", "reason": "aborted", "by": "lead"},
		{"plugin_event": "turn.complete", "turn_id": "t2", "reason": "aborted"},
	}
	if !reflect.DeepEqual(ends, want) {
		t.Fatalf("reported %v, want %v", ends, want)
	}
}

// A request is carried out once: not again at the next tick, not after a
// reload of the module, and not at all with an id of another shape.
func TestTheModuleCarriesARequestOutOnce(t *testing.T) {
	run := runControl(t, true, controlWorld{}, asked(idA, control.Compact, ""), tick, tick, []any{"reload"}, tick, tick)
	if len(run.Compacts) != 1 || run.Timers != 1 {
		t.Fatalf("compacted %d times, %d timers after the reload", len(run.Compacts), run.Timers)
	}
	odd := runControl(t, true, controlWorld{}, []any{"write", "request.json", `{"id":"../x","action":"compact","from":"lead"}`}, tick)
	if len(odd.Compacts) != 0 || len(odd.Files) != 1 {
		t.Fatalf("an odd id: compacted %d, files %v", len(odd.Compacts), odd.Files)
	}
}

// Without a control directory the module polls nothing.
func TestTheModuleWithoutControlPollsNothing(t *testing.T) {
	run := runControl(t, false, controlWorld{}, asked(idA, control.Compact, ""), tick)
	if run.Timers != 0 || len(run.Compacts) != 0 {
		t.Fatalf("%d timers, %d compactions", run.Timers, len(run.Compacts))
	}
}

// A request the asker withdrew as the module marked it taken — removed, or
// replaced by the next asker's — is answered as withdrawn and not carried out:
// the asker, looking for the mark after withdrawing, waits for that answer.
func TestTheModuleDoesNotCarryOutAWithdrawnRequest(t *testing.T) {
	for _, withdraw := range []string{"remove", "replace"} {
		run := runControl(t, true, controlWorld{Withdraw: withdraw},
			[]any{"event", "turn.start", map[string]any{"turnId": "t1"}}, asked(idA, control.Interrupt, ""), tick)
		answer := answerIn(t, run, idA)
		if answer["outcome"] != "refused" || answer["reason"] != control.Withdrawn || len(run.Aborts) != 0 {
			t.Fatalf("%s: %v, aborts %v", withdraw, answer, run.Aborts)
		}
		if len(run.Errors) != 0 {
			t.Fatalf("%s: host errors %v", withdraw, run.Errors)
		}
	}
}
