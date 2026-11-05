package mail

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/session"
	"github.com/praline-labs/rewake/internal/infra/boottime"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// stallLab is api after a turn that ended pending eleven minutes ago, its
// mark made a second before the end, with web's task read and owed.
type stallLab struct {
	dir         string
	self        registry.Session
	web         registry.Session
	ended, mark int64
}

func newStallLab(t *testing.T) stallLab {
	t.Helper()
	base := newTwoSessionLab(t)
	lab := stallLab{dir: base.dir, self: base.session(t, "api"), web: base.web}
	lab.ended = boottime.Now() - int64(11*time.Minute)
	lab.mark = lab.ended - int64(time.Second)
	if err := withLock(lab.dir, "api", func() error {
		return markScopedAwaiting(lab.dir, "api", lab.self.Epoch(), Message{ID: "task", From: "web", FromEpoch: lab.web.Epoch()})
	}); err != nil {
		t.Fatal(err)
	}
	if err := markPending(lab.dir, "api", lab.self.Epoch(), "the suite is running\nin the background", lab.mark); err != nil {
		t.Fatal(err)
	}
	outcome, err := CompleteTurn(t.Context(), lab.dir, lab.self, TurnEnd{Text: "started the suite", Started: lab.mark - int64(time.Second), Ended: lab.ended}, "t", TurnEndOptions{})
	if err != nil || !outcome.Interim {
		t.Fatalf("the pending end: %+v %v", outcome, err)
	}
	return lab
}

func (lab stallLab) box() Box { return Open(lab.dir, "api", lab.self.Epoch()) }

func (lab stallLab) remind(t *testing.T) string {
	t.Helper()
	var op string
	err := lab.box().WithLock(context.Background(), func(locked Locked) error {
		var err error
		op, err = locked.RemindStallIfDue(time.Nanosecond, func() session.Snapshot {
			idle := session.ActivityIdle
			return session.Snapshot{Activity: &idle, ActivityFresh: true, Fresh: true}
		}, time.Now())
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return op
}

func (lab stallLab) stalled(t *testing.T) (Stall, bool) {
	t.Helper()
	stall, ok, err := lab.box().Stalled()
	if err != nil {
		t.Fatal(err)
	}
	return stall, ok
}

// A run whose last word is an interim end, with no turn since and its sender
// live, is a stall: the pending line, when it was marked and when the turn
// ended, who waits.
func TestAPendingEndWithNoTurnSinceIsAStall(t *testing.T) {
	lab := newStallLab(t)
	stall, ok := lab.stalled(t)
	if !ok || stall.Text != "the suite is running\nin the background" || stall.Ended != lab.ended || stall.Marked != lab.mark ||
		len(stall.Senders) != 1 || stall.Senders[0] != "web" || stall.Op == "" {
		t.Fatalf("the stall: %+v %v", stall, ok)
	}
	if idle := time.Since(stall.IdleSince(time.Now())); idle < 11*time.Minute || idle > 12*time.Minute {
		t.Errorf("idle for %s, want eleven minutes", idle)
	}
}

// Whatever was heard of a turn after the interim end ends the stall: a timed
// start, any end on record — a stop, which leaves the interim record as it
// was, among them — or a report, which settles it. So does the sender's run
// ending, and a stall is never another run's.
func TestATurnSinceOrNobodyWaitingIsNoStall(t *testing.T) {
	for name, after := range map[string]func(t *testing.T, lab stallLab){
		"a timed start": func(t *testing.T, lab stallLab) {
			if err := recordTurnStart(lab.dir, "api", lab.self.Epoch(), boottime.Now()); err != nil {
				t.Fatal(err)
			}
		},
		"a stopped end": func(t *testing.T, lab stallLab) {
			if _, err := CompleteTurn(t.Context(), lab.dir, lab.self, TurnEnd{Text: "stopped", Stopped: true, Started: lab.ended + 1, Ended: boottime.Now()}, "t", TurnEndOptions{}); err != nil {
				t.Fatal(err)
			}
			if _, interim, _ := lastInterim(lab.dir, "api", lab.self.Epoch()); !interim {
				t.Fatal("the stop changed the interim record")
			}
		},
		"a report": func(t *testing.T, lab stallLab) {
			if _, err := CompleteTurn(t.Context(), lab.dir, lab.self, TurnEnd{Text: "the suite is green", Started: lab.ended + 1, Ended: boottime.Now()}, "t", TurnEndOptions{}); err != nil {
				t.Fatal(err)
			}
		},
		"the sender's run ended": func(t *testing.T, lab stallLab) {
			if err := state.Remove(state.SessionPath(lab.dir, "web")); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			lab := newStallLab(t)
			after(t, lab)
			if stall, ok := lab.stalled(t); ok {
				t.Fatalf("still a stall: %+v", stall)
			}
		})
	}
	lab := newStallLab(t)
	if _, ok, err := Open(lab.dir, "api", "another-run").Stalled(); ok || err != nil {
		t.Fatalf("another run's interim end is a stall of this one: %v %v", ok, err)
	}
}

// The reminder is one notify from the run to itself quoting the pending line,
// once for each interim end: a second call writes nothing, and nor does one
// after the letter has been read and archived. A later interim end is reminded
// of again.
func TestAStallIsRemindedOncePerInterimEnd(t *testing.T) {
	lab := newStallLab(t)
	stall, _ := lab.stalled(t)
	remind := func() {
		t.Helper()
		if op := lab.remind(t); op == "" {
			t.Fatal("the due stall was not reminded")
		}
	}
	remind()
	remind()
	letters := must(List(lab.dir, "api"))
	if len(letters) != 1 {
		t.Fatalf("%d letters after two reminders, want one", len(letters))
	}
	letter := letters[0]
	marked := time.Now().Add(-time.Duration(boottime.Now() - lab.mark)).Local().Format("15:04")
	for _, want := range []string{`Rewake: your pending "the suite is running" since ` + marked, "task of web", "idle 11m", "Go on, or end the turn with the report", "rewake pending"} {
		if !strings.Contains(letter.Text, want) {
			t.Errorf("the reminder lacks %q: %s", want, letter.Text)
		}
	}
	if letter.Kind != Note || letter.From != "api" || letter.To != "api" || letter.FromEpoch != lab.self.Epoch() || letter.ToEpoch != lab.self.Epoch() || Owed(letter) {
		t.Fatalf("the reminder is %+v, want a notify of the run to itself", letter)
	}

	if err := state.Remove(state.InboxPath(lab.dir, "api") + "/" + letter.ID + ".json"); err != nil {
		t.Fatal(err)
	}
	remind()
	if letters := must(List(lab.dir, "api")); len(letters) != 0 {
		t.Fatalf("a stall reminded of already wrote %d letters", len(letters))
	}

	if err := markPending(lab.dir, "api", lab.self.Epoch(), "the second half runs", boottime.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := CompleteTurn(t.Context(), lab.dir, lab.self, TurnEnd{Text: "half done", Started: lab.ended + 1, Ended: boottime.Now()}, "t", TurnEndOptions{}); err != nil {
		t.Fatal(err)
	}
	next, ok := lab.stalled(t)
	if !ok || next.Op == stall.Op || next.Text != "the second half runs" {
		t.Fatalf("the next interim end: %+v %v", next, ok)
	}
	remind()
	if letters := must(List(lab.dir, "api")); len(letters) != 1 || !strings.Contains(letters[0].Text, `"the second half runs"`) {
		t.Fatalf("the next interim end's reminder: %+v", letters)
	}
}

// A letter drawn but not yet written — the wrapper stopped between the two —
// is written under the same id by the next attempt, which a copy already in
// the mailbox makes a no-op.
func TestAReminderDrawnBeforeAStopIsWrittenOnce(t *testing.T) {
	lab := newStallLab(t)
	stall, _ := lab.stalled(t)
	drawn := stallRecord{Epoch: lab.self.Epoch(), Op: stall.Op, ID: NewID()}
	if err := lab.box().writeStallRecord(drawn); err != nil {
		t.Fatal(err)
	}
	if op := lab.remind(t); op != stall.Op {
		t.Fatalf("reminded %s, want %s", op, stall.Op)
	}
	if letters := must(List(lab.dir, "api")); len(letters) != 1 || letters[0].ID != drawn.ID {
		t.Fatalf("letters %+v, want the one drawn, %s", letters, drawn.ID)
	}
}
