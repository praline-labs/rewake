package claude

import (
	"reflect"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/control"
)

// The module's side of telling rewake which compaction a main asked for
// (docs/remote-control.md), under the host in plugin_control_test.go.

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

// As the host starts the compaction, the module tells rewake who asked, and
// holds the compaction until that report is sent: it then reaches the
// collector ahead of the compaction's hooks, and the compaction is counted as
// the main's. A refusal after that is told before the answer is written; one
// the host gives before it starts — mid-turn, say — needs no word at all.
func TestTheModuleTellsWhoAskedBeforeItCompacts(t *testing.T) {
	idle := runControl(t, true, controlWorld{}, asked(idA, control.Compact, ""), tick)
	want := []map[string]any{{"plugin_event": "compact.asked", "request": idA, "by": "lead"}}
	if got := reported(idle, "compact.asked"); !reflect.DeepEqual(got, want) || len(reported(idle, "compact.refused")) != 0 {
		t.Fatalf("reported %v, want %v and no refusal", idle.Calls, want)
	}
	if sent, compacted := indexOf(idle.Order, "sent compact.asked"), indexOf(idle.Order, "compact"); sent < 0 || compacted < 0 || sent > compacted {
		t.Fatalf("the host compacted before the report was sent: %v", idle.Order)
	}
	short := runControl(t, true, controlWorld{TooShort: true}, asked(idA, control.Compact, ""), tick)
	refused := []map[string]any{{"plugin_event": "compact.refused", "request": idA}}
	if got := reported(short, "compact.refused"); len(reported(short, "compact.asked")) != 1 || !reflect.DeepEqual(got, refused) {
		t.Fatalf("a refused compaction reported %v", short.Calls)
	}
	if sent, answered := indexOf(short.Order, "sent compact.refused"), indexOf(short.Order, "write "+idA+".result"); sent < 0 || answered < 0 || sent > answered {
		t.Fatalf("answered before the refusal was sent: %v", short.Order)
	}
	for name, world := range map[string]controlWorld{"mid-turn": {}, "in flight": {InFlight: true}, "switched off": {CompactOff: true}} {
		steps := [][]any{asked(idA, control.Compact, ""), tick}
		if name == "mid-turn" {
			steps = append([][]any{{"event", "turn.start", map[string]any{"turnId": "t1"}}}, steps...)
		}
		run := runControl(t, true, world, steps...)
		if len(reported(run, "compact.asked")) != 0 || len(reported(run, "compact.refused")) != 0 {
			t.Fatalf("%s: a compaction the host never started was announced: %v", name, run.Calls)
		}
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
	if answer := answerIn(t, run, idA); answer["outcome"] != "done" {
		t.Fatalf("answer %v", answer)
	}
}
