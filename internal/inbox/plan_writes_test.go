package inbox

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// writeFaults breaks, once each, every write the barrier makes in a scene: a
// write of its own records — the stop, a journal, a note owed, a mark — fails
// as a read may, and the next attempt goes through. The writes are taken from
// the seam, as the reads are, in three runs of the scene: as it is; with a
// read of the effects failing in the effects alone, so the barrier records an
// effect's stop; and with that read failing the plan after them as well, so
// the stop is recorded with a cause beside it. Whatever write failed, nothing
// may be lost or done twice — a barrier past it ends where the scene ends —
// and no stop may be weaker than what the barrier knew: once an effect met
// its unknown, the stop on record is that effect's, and the gate stays closed
// until a barrier runs the effects through. It answers how many writes it
// broke.
func writeFaults(t *testing.T, template, work, mailbox string, base sceneRun) int {
	t.Helper()
	broken := 0
	for _, write := range base.written {
		breakWrite(t, fresh(t, template, work), mailbox, write, nil, false, base)
		broken++
	}
	stop := filepath.Join("inbox", mailbox, stopFile)
	for _, read := range sorted(base.applied) {
		if read.rel == stop {
			continue
		}
		fault := seamFault{at: read, kind: "no access"}
		for _, second := range []bool{false, true} {
			learned := writeRun(fresh(t, template, work), mailbox, nil, &fault, second)
			if !learned.met {
				continue
			}
			for _, write := range learned.written {
				breakWrite(t, fresh(t, template, work), mailbox, write, &fault, second, base)
				broken++
			}
		}
	}
	return broken
}

// brokenRun is one barrier with write broken, fault failing a read of the
// effects, and, when second, the same read failing the plan after them.
type brokenRun struct {
	barrier    string
	written    []seamWrite
	met, broke bool
	plan, live *seamProbe
}

func writeRun(dir, mailbox string, write *seamWrite, fault *seamFault, second bool) brokenRun {
	run := brokenRun{plan: newProbe(dir, osAccess{}, nil), live: newProbe(dir, osAccess{}, fault)}
	run.live.broken = write
	testAccess.Store(dir, passAccess{live: run.live, plan: run.plan})
	defer testAccess.Delete(dir)
	if second {
		afterEffect = func() { run.plan.arm(fault) }
		defer func() { afterEffect = func() {} }()
	}
	run.barrier = outcome(barrier(dir, mailbox, true))
	run.written, run.broke = run.live.written(), run.live.broke
	run.met = fault != nil && run.live.hits > 0 && (!second || run.plan.hits > 0)
	return run
}

func breakWrite(t *testing.T, dir, mailbox string, write seamWrite, fault *seamFault, second bool, base sceneRun) {
	t.Helper()
	label := "write " + write.String() + " fails once"
	if fault != nil {
		label = fmt.Sprintf("%s, with the effects failing on %s (the plan after them too: %v)", label, fault.label(), second)
	}
	run := writeRun(dir, mailbox, &write, fault, second)
	if !run.broke {
		t.Errorf("%s: the barrier did not make it again", label)
		return
	}
	if run.met {
		recorded, err := recordedStop(dir, mailbox)
		if open(run.barrier) || err != nil || recorded == nil || !recorded.Effect || !strings.Contains(recorded.Met, fault.at.rel) {
			t.Errorf("%s: barrier %s, a stop weaker than the effect's on record: %+v %v", label, run.barrier, recorded, err)
		}
	}
	// The gate asks with the effects' fault still in place and the plan's
	// gone: an effect's stop answers from its record, and anything else must
	// be what a plan finds in what the failed write left.
	gate := newProbe(dir, osAccess{}, fault)
	testAccess.Store(dir, passAccess{live: gate, plan: osAccess{}})
	answer := outcome(MailboxStopped(dir, mailbox))
	testAccess.Delete(dir)
	if run.met == open(answer) {
		t.Errorf("%s: the gate answers %s; the effect met its unknown: %v", label, answer, run.met)
	}
	clean := runScene(t, dir, mailbox, nil, nil, true)
	if clean.barrier != base.barrier || clean.gateAfter != base.gateAfter {
		t.Errorf("%s: past it the barrier answers %s and the gate %s, without it %s and %s", label, clean.barrier, clean.gateAfter, base.barrier, base.gateAfter)
	}
	if changed := differ(base.tree, clean.tree, true); len(changed) > 0 {
		t.Errorf("%s: past it the mailboxes differ from the scene's: %v", label, changed)
	}
}

// unrecordedPair runs a pair of causes again with every write of the stop
// failing: no stop is on record, so the barrier's answer is the only place
// left that holds what it met, and it must name the effect's cause beside the
// plan's, as recorded holds them when the writes go through, and both failed
// writes.
func unrecordedPair(t *testing.T, dir, mailbox string, faults [2]seamFault, recorded stopRecord) {
	t.Helper()
	label := fmt.Sprintf("both writes of the stop fail, with the effects failing on %s and the plan after them on %s", faults[0].label(), faults[1].label())
	writes := &stopWrites{fileAccess: osAccess{}, path: stopPath(dir, mailbox)}
	plan, live := newProbe(dir, osAccess{}, nil), newProbe(dir, writes, &faults[0])
	testAccess.Store(dir, passAccess{live: live, plan: plan})
	defer testAccess.Delete(dir)
	afterEffect = func() { plan.arm(&faults[1]) }
	defer func() { afterEffect = func() {} }()
	err := barrier(dir, mailbox, true)
	if writes.failed != 2 || err == nil {
		t.Errorf("%s: %d writes failed, the barrier answers %v", label, writes.failed, err)
		return
	}
	answer := err.Error()
	if !strings.Contains(answer, recorded.Met) || !strings.Contains(answer, recorded.Cause) || strings.Count(answer, "could not record the stop") != 2 {
		t.Errorf("%s: the barrier answers %q, which does not name the effect's cause %q, the plan's %q and both failed writes", label, answer, recorded.Met, recorded.Cause)
	}
}
