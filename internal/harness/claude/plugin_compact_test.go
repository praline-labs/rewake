package claude

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/control"
)

// The module's side of telling rewake which compaction a main asked for
// (docs/remote-control.md), under the host in plugin_control_host_test.go.

// reported picks out the module's reports of one event.
func reported(run controlRun, event string) []map[string]any {
	var found []map[string]any
	for _, call := range run.Calls {
		if call["plugin_event"] == event {
			found = append(found, call)
		}
	}
	return found
}

// Before it asks the host to compact, the module tells rewake who asked, and
// waits until that report is sent: it then reaches the collector ahead of the
// compaction's hooks, and the compaction is counted as the main's. A refusal is
// told before the answer is written, saying whether the host had started;
// a compaction asked mid-turn, which the host refuses, is not announced.
func TestTheModuleTellsWhoAskedBeforeItCompacts(t *testing.T) {
	idle := runControl(t, true, controlWorld{}, asked(idA, control.Compact, ""), tick)
	want := []map[string]any{{"plugin_event": "compact.asked", "request": idA, "by": "lead"}}
	if got := reported(idle, "compact.asked"); !reflect.DeepEqual(got, want) || len(reported(idle, "compact.refused")) != 0 {
		t.Fatalf("reported %v, want %v and no refusal", idle.Calls, want)
	}
	if sent, compacted := indexOf(idle.Order, "sent compact.asked"), indexOf(idle.Order, "compact"); sent < 0 || compacted < 0 || sent > compacted {
		t.Fatalf("the host compacted before the report was sent: %v", idle.Order)
	}
	for name, tc := range map[string]struct {
		world   controlWorld
		started bool
	}{
		"too short":              {controlWorld{TooShort: true}, true},
		"in flight":              {controlWorld{InFlight: true}, false},
		"switched off":           {controlWorld{CompactOff: true}, false},
		"external turn":          {controlWorld{Refuse: externalTurn}, false},
		"thin client":            {controlWorld{Refuse: thinClient}, false},
		"failed after the start": {controlWorld{FailAfter: "the compaction produced no summary"}, true},
	} {
		run := runControl(t, true, tc.world, asked(idA, control.Compact, ""), tick)
		refused := map[string]any{"plugin_event": "compact.refused", "request": idA, "started": tc.started}
		if got := reported(run, "compact.refused"); len(reported(run, "compact.asked")) != 1 || !reflect.DeepEqual(got, []map[string]any{refused}) {
			t.Fatalf("%s: reported %v, want %v", name, run.Calls, refused)
		}
		if sent, answered := indexOf(run.Order, "sent compact.refused"), indexOf(run.Order, "write "+idA+".result"); sent < 0 || answered < 0 || sent > answered {
			t.Fatalf("%s: answered before the refusal was sent: %v", name, run.Order)
		}
	}
	busy := runControl(t, true, controlWorld{}, []any{"event", "turn.start", map[string]any{"turnId": "t1"}}, asked(idA, control.Compact, ""), tick)
	if len(reported(busy, "compact.asked")) != 0 || len(reported(busy, "compact.refused")) != 0 {
		t.Fatalf("a compaction asked mid-turn was announced: %v", busy.Calls)
	}
}

// A compaction the module did not ask for — a person's /compact, an automatic
// one — is not announced, and the module waits out the second after it before
// it asks for one: that compaction's PostCompact may still be on its way to
// rewake, and arriving after the word for the main's it would be taken for
// that one.
func TestTheModuleWaitsOutAnotherCompaction(t *testing.T) {
	other := runControl(t, true, controlWorld{}, []any{"compaction", "manual"})
	if len(other.Calls) != 1 || len(reported(other, "compact.asked")) != 0 {
		t.Fatalf("a compaction nobody asked for was reported: %v", other.Calls)
	}
	run := runControl(t, true, controlWorld{}, []any{"compaction", "manual"}, asked(idA, control.Compact, ""), tick)
	ended, sleep, slept := indexOf(run.Order, "compact manual"), indexOf(run.Order, "sleep 1000"), indexOf(run.Order, "slept 1000")
	if sent := indexOf(run.Order, "sent compact.asked"); ended < 0 || sleep < ended || slept < sleep || sent < slept {
		t.Fatalf("asked for a compaction within the second after another ended: %v", run.Order)
	}
	if answer := answerIn(t, run, idA); answer["outcome"] != "started" {
		t.Fatalf("answer %v", answer)
	}
}

// The host holds a module to the harness's re-entry rule: a compaction the
// module's own call raised does not reach its session.compact handler, while
// one it did not ask for does. A softer host let the module's word ride on
// that handler, which a live session never ran.
func TestTheHostSkipsTheModulesHandlerForItsOwnCompaction(t *testing.T) {
	probe := []byte(`export function register(on) {
  on("session.start", async ($, e, next) => {
    $.clock.every(250, async () => { try { await $.session.compact() } catch {} })
    return next(e)
  })
  on("session.compact", async ($, e, next) => {
    await $.process.run(["/bin/probe"], { stdin: JSON.stringify({ plugin_event: "saw " + e.trigger }) })
    return next(e)
  })
}
`)
	run := runControlModule(t, true, controlWorld{}, probe, tick, []any{"compaction", "manual"})
	want := []map[string]any{{"plugin_event": "saw manual"}}
	if !reflect.DeepEqual(run.Calls, want) || len(run.Compacts) != 1 {
		t.Fatalf("the handler saw %v, want %v; %d compactions of its own", run.Calls, want, len(run.Compacts))
	}
}

// The command is answered as soon as the compaction is seen to begin — the
// collector's mark that its PreCompact arrived — and not at its end, which is
// told to rewake afterwards with the counts. A second request while it runs is
// refused: it would put its asker's word over the first's.
func TestTheModuleAnswersBeforeTheCompactionEnds(t *testing.T) {
	run := runControl(t, true, controlWorld{Hold: 300, Mark: true}, asked(idA, control.Compact, ""), tick, asked(idB, control.Compact, ""), tick)
	if answer := answerIn(t, run, idA); answer["outcome"] != "started" {
		t.Fatalf("answer %v", answer)
	}
	// The last end told is the compaction's: the refused second request is
	// told as soon as it is answered.
	answered, compacted, ended := indexOf(run.Order, "write "+idA+".result"), indexOf(run.Order, "compact"), -1
	for i, step := range run.Order {
		if step == "sent compact.ended" {
			ended = i
		}
	}
	if answered < 0 || compacted < answered || ended < compacted {
		t.Fatalf("answered after the compaction's end, or told its end first: %v", run.Order)
	}
	if answer := answerIn(t, run, idB); answer["outcome"] != "refused" || answer["reason"] != control.InTurn || len(run.Compacts) != 1 {
		t.Fatalf("a second request mid-compaction: %v, %d compactions", answer, len(run.Compacts))
	}
	want := []map[string]any{
		{"plugin_event": "compact.ended", "request": idB, "by": "lead", "outcome": "refused", "reason": control.InTurn, "detail": "the compaction rewake asked for last is still running"},
		{"plugin_event": "compact.ended", "request": idA, "by": "lead", "outcome": "done", "tokensBefore": 120000.0, "tokensAfter": 9000.0},
	}
	if got := reported(run, "compact.ended"); !reflect.DeepEqual(got, want) {
		t.Fatalf("reported %v, want %v", got, want)
	}
}

// A compaction whose start is not seen within the bound is answered as
// requested, not started, and still told at its end; one that fails after
// the answer is told as failed, with the refusal laid aside first.
func TestTheModuleSaysRequestedWhenItSawNoStart(t *testing.T) {
	run := runControl(t, true, controlWorld{Hold: 300}, asked(idA, control.Compact, ""), tick)
	if answer := answerIn(t, run, idA); answer["outcome"] != "requested" || answer["detail"] != "its start was not seen within 3 s" {
		t.Fatalf("answer %v", answer)
	}
	if got := reported(run, "compact.ended"); len(got) != 1 || got[0]["outcome"] != "done" {
		t.Fatalf("reported %v", got)
	}
	failed := runControl(t, true, controlWorld{Hold: 300, Mark: true, FailAfter: "the compaction produced no summary"}, asked(idA, control.Compact, ""), tick)
	if answer := answerIn(t, failed, idA); answer["outcome"] != "started" {
		t.Fatalf("answer %v", answer)
	}
	refused, ended := indexOf(failed.Order, "sent compact.refused"), indexOf(failed.Order, "sent compact.ended")
	got := reported(failed, "compact.ended")
	if refused < 0 || ended < refused || len(got) != 1 || got[0]["outcome"] != "failed" || !strings.Contains(fmt.Sprint(got[0]["detail"]), "the compaction produced no summary") {
		t.Fatalf("reported %v, order %v", failed.Calls, failed.Order)
	}
}

// An answer the module could not write does not strand the compaction: its
// end is still told, and the next request is taken rather than refused as if
// the first were still running.
func TestAnUnwrittenAnswerStillEndsTheCompaction(t *testing.T) {
	run := runControl(t, true, controlWorld{Hold: 100, Mark: true, FailResult: idA}, asked(idA, control.Compact, ""), tick, []any{"wait", 300}, asked(idB, control.Compact, ""), tick)
	if _, answered := run.Files[idA+".result"]; answered {
		t.Fatalf("the host wrote an answer it was to fail: %v", run.Files)
	}
	if got := reported(run, "compact.ended"); len(got) != 2 || got[0]["request"] != idA || got[0]["outcome"] != "done" {
		t.Fatalf("reported %v", run.Calls)
	}
	if answer := answerIn(t, run, idB); answer["outcome"] != "started" || len(run.Compacts) != 2 {
		t.Fatalf("the next request after an unwritten answer: %v, %d compactions", answer, len(run.Compacts))
	}
}

// A host call that failed before the module saw the compaction start is a
// final answer: the call is over, so nothing more can come of it, and the
// answer does not leave the outcome open for a letter.
func TestAFailedHostCallIsAFinalAnswer(t *testing.T) {
	run := runControl(t, true, controlWorld{FailAfter: "the compaction produced no summary"}, asked(idA, control.Compact, ""), tick)
	answer := answerIn(t, run, idA)
	if _, open := answer["open"]; answer["outcome"] != "failed" || open {
		t.Fatalf("answer %v, want a final failure", answer)
	}
}

// A final answer — a refusal, a failure — is told as the compaction's outcome
// too, after the answer is written: a command whose wait was cut short once
// the request was taken holds an open answer, and main's letter then comes
// from this outcome at once rather than at the bound. A command that read the
// answer has closed its record, and the outcome sends nothing.
func TestAFinalAnswerIsToldAsTheOutcome(t *testing.T) {
	for name, tc := range map[string]struct {
		world  controlWorld
		steps  [][]any
		reason string
	}{
		"refused in a turn": {controlWorld{}, [][]any{{"event", "turn.start", map[string]any{"turnId": "t1"}}}, control.InTurn},
		"failed":            {controlWorld{FailAfter: "the compaction produced no summary"}, nil, ""},
	} {
		run := runControl(t, true, tc.world, append(tc.steps, asked(idA, control.Compact, ""), tick)...)
		answer := answerIn(t, run, idA)
		got := reported(run, "compact.ended")
		if len(got) != 1 || got[0]["request"] != idA || got[0]["by"] != "lead" || got[0]["outcome"] != answer["outcome"] {
			t.Fatalf("%s: answered %v, told %v", name, answer, got)
		}
		if tc.reason != "" && got[0]["reason"] != tc.reason {
			t.Fatalf("%s: told %v, want the reason %q", name, got, tc.reason)
		}
		if answered, told := indexOf(run.Order, "write "+idA+".result"), indexOf(run.Order, "sent compact.ended"); answered < 0 || told < answered {
			t.Fatalf("%s: told before the answer was written: %v", name, run.Order)
		}
	}
}
