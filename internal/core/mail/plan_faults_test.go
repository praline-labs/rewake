package mail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// One journal exercises each read-failure kind before effects and during
// effects. The retry must converge on the clean result without publishing
// its report twice; the exhaustive scene and fault-pair matrix is retired.
func TestPlanFaultSmoke(t *testing.T) {
	lab := newTwoSessionLab(t)
	lab.owe(t, "task")
	waiters, err := ReadWaiters(lab.dir, "api", lab.run)
	if err != nil {
		t.Fatal(err)
	}
	report := lab.report(Finished, "task")
	if err := WriteJournal(lab.dir, "api", "end", TurnJournal{
		Epoch: lab.run, Op: "end", Reports: []Message{report}, Clear: waiters,
	}); err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(probeDir(t), "state")
	base := runScene(t, fresh(t, lab.dir, work), "api", nil, nil, true)
	if !open(base.barrier) || !open(base.gateAfter) {
		t.Fatalf("the clean journal did not finish: %s, %s", base.barrier, base.gateAfter)
	}
	read := seamRead{op: "read", rel: filepath.Join("inbox", "api", "journal", "end")}
	if !base.planned[read] || !base.applied[read] {
		t.Fatal("the journal was not read by both passes")
	}
	for _, kind := range []string{"not a directory", "no access", "a directory", "unparseable"} {
		t.Run(kind, func(t *testing.T) {
			fault := seamFault{at: read, kind: kind, garbage: []byte("{")}
			planFault(t, fresh(t, lab.dir, work), "api", fault)
			effectFault(t, fresh(t, lab.dir, work), "api", fault, base)
		})
	}
	t.Run("failed journal completion", func(t *testing.T) {
		write := seamWrite{op: "rename", rel: read.rel, n: 1}
		if !slices.Contains(base.written, write) {
			t.Fatal("the clean journal did not reach its completion")
		}
		dir := fresh(t, lab.dir, work)
		run := writeRun(dir, "api", &write, nil)
		if !run.broke || open(run.barrier) {
			t.Fatalf("the failed completion did not stop: broke %v, barrier %s", run.broke, run.barrier)
		}
		clean := runScene(t, dir, "api", nil, nil, true)
		if clean.barrier != base.barrier || clean.gateAfter != base.gateAfter {
			t.Errorf("retry: barrier %s, gate %s", clean.barrier, clean.gateAfter)
		}
		if changed := differ(base.tree, clean.tree, true); len(changed) > 0 {
			t.Errorf("retry differs from the clean journal: %v", changed)
		}
		if copies := lettersOf(t, dir, "web")[report.ID]; copies != 1 {
			t.Errorf("retry left %d copies of the report", copies)
		}
	})
}

// sceneRun is what one run of a scene answered and left.
type sceneRun struct {
	gateBefore, barrier, gateAfter string
	tree                           map[string]string
	plan, live                     *seamProbe
	// planned and applied are what the plans and the effects read up to the
	// barrier's end: a gate after it plans on what the effects left.
	planned, applied map[seamRead]bool
	// written is what the barrier wrote, in order.
	written []seamWrite
}

// runScene asks the gate, runs the barrier, and asks the gate again, with
// the passes reading through probes that fail as faults say.
func runScene(t *testing.T, dir, mailbox string, planFault, liveFault *seamFault, locked bool) sceneRun {
	t.Helper()
	run := sceneRun{plan: newProbe(dir, osAccess{}, planFault), live: newProbe(dir, osAccess{}, liveFault)}
	testAccess.Store(dir, passAccess{live: run.live, plan: run.plan})
	defer testAccess.Delete(dir)
	run.gateBefore = outcome(MailboxStopped(dir, mailbox))
	before := len(run.live.written())
	run.barrier = outcome(barrier(dir, mailbox, locked))
	run.planned, run.applied = run.plan.taken(), run.live.taken()
	run.written = run.live.written()[before:]
	run.gateAfter = outcome(MailboxStopped(dir, mailbox))
	run.tree = snapshot(t, dir)
	return run
}

func barrier(dir, mailbox string, locked bool) error {
	reconcile := func() error { return Reconcile(context.Background(), dir, mailbox) }
	if !locked {
		return reconcile()
	}
	return withLock(dir, mailbox, reconcile)
}

// outcome is what a call answered, as far as a test compares two runs.
func outcome(err error) string {
	var unknown *UnknownRecordError
	switch {
	case err == nil:
		return "open"
	case errors.As(err, &unknown):
		return "unknown: " + err.Error()
	}
	return "stopped: " + err.Error()
}

func open(outcome string) bool { return outcome == "open" }

// planFault is the fault in both passes: the plan meets it before any
// effect, so nothing changes but the stop on record, and every call says so.
func planFault(t *testing.T, dir, mailbox string, fault seamFault) {
	t.Helper()
	before := snapshot(t, dir)
	run := runScene(t, dir, mailbox, &fault, &fault, true)
	label := fault.label()
	if run.plan.hits == 0 {
		t.Errorf("%s: the plan did not read it again", label)
		return
	}
	if open(run.gateBefore) || open(run.barrier) || open(run.gateAfter) {
		t.Errorf("%s: gate %s, barrier %s, gate after %s", label, run.gateBefore, run.barrier, run.gateAfter)
	}
	if changed := differ(before, run.tree, false); len(changed) > 0 {
		t.Errorf("%s: an effect ran past it: %v", label, changed)
	}
}

// effectFault is the fault in the effects alone: the barrier stops as the
// effect's, a retry that meets it again keeps the stop, and a barrier without
// it ends as the scene does. It answers whether the retry met the fault.
func effectFault(t *testing.T, dir, mailbox string, fault seamFault, base sceneRun) bool {
	t.Helper()
	label := "in the effects, " + fault.label()
	run := runScene(t, dir, mailbox, nil, &fault, true)
	if run.live.hits == 0 {
		t.Errorf("%s: the effects did not read it again", label)
		return false
	}
	if open(run.barrier) || open(run.gateAfter) || !effectStop(dir, mailbox) {
		t.Errorf("%s: barrier %s, gate after %s, stop of an effect %v", label, run.barrier, run.gateAfter, effectStop(dir, mailbox))
		return false
	}
	retry := runScene(t, dir, mailbox, nil, &fault, true)
	if retry.live.hits > 0 && (open(retry.barrier) || !effectStop(dir, mailbox)) {
		t.Errorf("%s: a retry that met it again lifted the stop: %s", label, retry.barrier)
	}
	clean := runScene(t, dir, mailbox, nil, nil, true)
	if clean.barrier != base.barrier || clean.gateAfter != base.gateAfter {
		t.Errorf("%s: past it the barrier answers %s and the gate %s, without it %s and %s", label, clean.barrier, clean.gateAfter, base.barrier, base.gateAfter)
	}
	if changed := differ(base.tree, clean.tree, true); len(changed) > 0 {
		t.Errorf("%s: past it the mailboxes differ from the scene's: %v", label, changed)
	}
	return retry.live.hits > 0
}

// effectStop says an occurrence an effect met is open.
func effectStop(dir, mailbox string) bool {
	open, err := stopState(dir, mailbox)
	return err == nil && slices.ContainsFunc(open, func(stop openStop) bool { return stop.record.Kind == causeEffect })
}

// ofStop says rel is the stop's own: its records, which the barrier reads
// itself, not as a pass does.
func ofStop(rel, mailbox string) bool {
	stops := filepath.Join("inbox", mailbox, stopsDir)
	return rel == stops || strings.HasPrefix(rel, stops+string(filepath.Separator))
}

func (f seamFault) label() string { return fmt.Sprintf("%s %s: %s", f.at.op, f.at.rel, f.kind) }

func sorted(reads map[seamRead]bool) []seamRead {
	return slices.SortedFunc(maps.Keys(reads), func(a, b seamRead) int {
		return strings.Compare(a.op+" "+a.rel, b.op+" "+b.rel)
	})
}

// differ names what two trees do not share. Two runs of a scene write the
// same letters at the same paths, but a letter written at run time carries
// its time; so letters may compare by their paths alone.
func differ(want, got map[string]string, letters bool) []string {
	var changed []string
	for _, rel := range slices.Sorted(maps.Keys(want)) {
		content, ok := got[rel]
		if !ok {
			changed = append(changed, "gone "+rel)
			continue
		}
		if content != want[rel] && (!letters || !isLetter(rel) && !sameStamped(rel, content, want[rel])) {
			changed = append(changed, "changed "+rel)
		}
	}
	for rel := range got {
		if _, ok := want[rel]; !ok {
			changed = append(changed, "new "+rel)
		}
	}
	return changed
}

// sameStamped says two versions of a decision's closure, durable marker or
// told record are the same but for when they were written, which every run
// writes anew, and for which attempt put a note told: written now and written
// before both say it is in the run's mailbox.
func sameStamped(rel, a, b string) bool {
	if !strings.HasSuffix(rel, closedSuffix) && !strings.HasSuffix(rel, durableSuffix) && !strings.Contains(filepath.Base(rel), toldSuffix) {
		return false
	}
	var x, y map[string]any
	if json.Unmarshal([]byte(a), &x) != nil || json.Unmarshal([]byte(b), &y) != nil {
		return false
	}
	for _, record := range []map[string]any{x, y} {
		delete(record, "at")
		if record["answer"] == ToldBefore {
			record["answer"] = ToldWritten
		}
	}
	return reflect.DeepEqual(x, y)
}

func isLetter(rel string) bool {
	kind, ok := kindAt(rel)
	return ok && strings.HasSuffix(kind.pattern, "*.json") && !strings.HasPrefix(kind.pattern, "receipts")
}
