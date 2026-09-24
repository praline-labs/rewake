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
	if answer := answerIn(t, run, idA); answer["outcome"] != "done" {
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
