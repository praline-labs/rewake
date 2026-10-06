package inbox

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/state"
)

// The barrier's plan reads what every effect will decide by, so nothing an
// effect meets is news: no list of those reads is kept to check this by, since
// such a list is what three review rounds found short. The test takes the
// reads from the seam instead (access.go). Each scene runs once to learn what
// its plan read, then once per read and fault — a path that cannot be
// followed, one closed to reading, a directory where a file belongs, and,
// for a kind that reads its content, content it does not read as — with the
// fault in both passes, where nothing may change and the mailbox stops; and
// once per read the effects make with the fault in the effects alone, where
// the stop is the effect's and outlives a retry that meets it again, and a
// barrier past it ends where the scene ends without one; and once per pair of
// such a read and a read of the plan after the failure (plan_pairs_test.go),
// where the effect's stop outlives the plan's cause, and once more with no
// stop written, where the answer names both; and once per write the barrier
// makes, broken once (plan_writes_test.go). A read added to an
// effect later is on the list the next run. One that goes around the seam is
// caught by the last leg: the room's mailboxes move aside, the seam follows
// them, and a read around it meets a file where they were.
func TestEveryReadOfThePlanStopsBeforeTheFirstEffect(t *testing.T) {
	kinds := map[string]bool{}
	var plans []map[seamRead]bool
	for _, scene := range planScenes {
		lab := newTwoSessionLab(t)
		mailbox := scene.build(t, lab)
		template := lab.dir
		t.Run(scene.name, func(t *testing.T) {
			work := filepath.Join(probeDir(t), "state")
			base := runScene(t, fresh(t, template, work), mailbox, nil, nil, true)
			if strings.HasPrefix(base.barrier, "unknown") {
				t.Fatalf("the scene does not run through: %s", base.barrier)
			}
			plan, applied := base.planned, base.applied
			plans = append(plans, plan)
			stop := filepath.Join("inbox", mailbox, stopFile)
			for read := range applied {
				if read.rel != stop && !plan[read] {
					t.Errorf("an effect read %s %s, which the plan did not", read.op, read.rel)
				}
			}
			tried, inEffects, again := 0, 0, 0
			for _, read := range sorted(plan) {
				for _, fault := range faultsOf(t, read) {
					planFault(t, fresh(t, template, work), mailbox, fault)
					tried++
					if applied[read] {
						inEffects++
						if effectFault(t, fresh(t, template, work), mailbox, fault, base) {
							again++
						}
					}
				}
			}
			pairs, unrecorded := causePairs(t, template, work, mailbox, base)
			broken := writeFaults(t, template, work, mailbox, base)
			t.Logf("%d reads in the plan, %d of them by the effects; %d faults in both passes, %d in the effects alone, %d of those met again by the retry; %d pairs of an effect's cause and the plan's after it, %d of them with no stop recorded; %d writes broken once", len(plan), len(applied), tried, inEffects, again, pairs, unrecorded, broken)
			aside(t, fresh(t, template, work), mailbox, base)
		})
	}
	for _, plan := range plans {
		for read := range plan {
			if kind, ok := kindAt(read.rel); ok && read.op == "read" {
				kinds[kind.pattern] = true
			}
		}
	}
	for _, kind := range recordKinds {
		if kind.opened && !kinds[kind.pattern] {
			t.Errorf("no scene's plan reads %s (%s): its faults are not tried", kind.pattern, kind.what)
		}
	}
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
	delete(run.tree, filepath.Join("inbox", mailbox, stopFile))
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

// aside runs the scene with the room's mailboxes moved where only the seam
// finds them: it must end as it does in place.
func aside(t *testing.T, dir, mailbox string, base sceneRun) {
	t.Helper()
	inbox := state.InboxesPath(dir)
	moved := remapped{from: inbox, to: inbox + ".aside"}
	if err := os.Rename(inbox, moved.to); err != nil {
		t.Fatal(err)
	}
	writeRaw(t, inbox, "the mailboxes are aside")
	run := sceneRun{plan: newProbe(dir, moved, nil), live: newProbe(dir, moved, nil)}
	testAccess.Store(dir, passAccess{live: run.live, plan: run.plan})
	run.gateBefore = outcome(MailboxStopped(dir, mailbox))
	run.barrier = outcome(barrier(dir, mailbox, false))
	run.gateAfter = outcome(MailboxStopped(dir, mailbox))
	testAccess.Delete(dir)
	if err := os.Remove(inbox); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(moved.to, inbox); err != nil {
		t.Fatal(err)
	}
	if run.gateBefore != base.gateBefore || run.barrier != base.barrier || run.gateAfter != base.gateAfter {
		t.Errorf("around the seam: gate %s, barrier %s, gate after %s; in place %s, %s, %s", run.gateBefore, run.barrier, run.gateAfter, base.gateBefore, base.barrier, base.gateAfter)
	}
	if changed := differ(base.tree, snapshot(t, dir), true); len(changed) > 0 {
		t.Errorf("around the seam the mailboxes differ: %v", changed)
	}
}

func effectStop(dir, mailbox string) bool {
	recorded, err := recordedStop(dir, mailbox)
	return err == nil && recorded != nil && recorded.Effect
}

// faultsOf is every fault that applies to one read.
func faultsOf(t *testing.T, read seamRead) []seamFault {
	t.Helper()
	var faults []seamFault
	for _, kind := range map[string][]string{
		"read": {"not a directory", "no access", "a directory"},
		"stat": {"not a directory", "no access", "a directory"},
		"list": {"not a directory", "no access"},
	}[read.op] {
		faults = append(faults, seamFault{at: read, kind: kind})
	}
	if record, ok := kindAt(read.rel); ok && read.op == "read" && record.parse != nil {
		for _, garbage := range []string{"{", ""} {
			if record.parse(read.rel, []byte(garbage)) != nil {
				faults = append(faults, seamFault{at: read, kind: "unparseable", garbage: []byte(garbage)})
				return faults
			}
		}
		t.Errorf("no content tried fails to read as %s", record.what)
	}
	return faults
}

func (f seamFault) label() string { return fmt.Sprintf("%s %s: %s", f.at.op, f.at.rel, f.kind) }

// kindAt is the kind of a path under the state directory, in whichever
// mailbox.
func kindAt(rel string) (recordKind, bool) {
	parts := strings.SplitN(rel, string(filepath.Separator), 3)
	if len(parts) < 3 || parts[0] != "inbox" {
		return recordKind{}, false
	}
	return kindOf(parts[2])
}

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
		if content != want[rel] && (!letters || !isLetter(rel)) {
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

func isLetter(rel string) bool {
	kind, ok := kindAt(rel)
	return ok && strings.HasSuffix(kind.pattern, "*.json") && !strings.HasPrefix(kind.pattern, "receipts")
}
