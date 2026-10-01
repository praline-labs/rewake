package inbox

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// causePairs composes two causes in a scene: a read of the effects fails in
// the effects alone, and once that effect has failed, a read fails the plan
// after it, the same read or another. The plan's cause is no proof that it is
// the effect's, so the effect's stop must stand beside it, and outlive it once
// it is gone; only a barrier that runs the effects through ends the stop. The
// fault is the same kind for both, since every kind is tried on its own by the
// legs before: what is composed here is the causes. A pair whose second read
// the plan after the failure does not make is not one the scene produces.
// Each pair produced is run again with both writes of the stop failing
// (unrecordedPair). It answers how many pairs the scene produced, and how
// many of them were run so.
func causePairs(t *testing.T, template, work, mailbox string, base sceneRun) (pairs, unrecorded int) {
	t.Helper()
	stop := filepath.Join("inbox", mailbox, stopFile)
	for _, first := range sorted(base.applied) {
		if first.rel == stop {
			continue
		}
		for _, second := range sorted(base.planned) {
			faults := [2]seamFault{{at: first, kind: "no access"}, {at: second, kind: "no access"}}
			produced, recorded := pairFault(t, fresh(t, template, work), mailbox, faults[0], faults[1], base)
			if produced {
				pairs++
			}
			if recorded != nil {
				unrecordedPair(t, fresh(t, template, work), mailbox, faults, *recorded)
				unrecorded++
			}
		}
	}
	return pairs, unrecorded
}

// pairFault answers whether the scene produced the pair, and the stop it
// recorded when that stop holds both causes.
func pairFault(t *testing.T, dir, mailbox string, first, second seamFault, base sceneRun) (bool, *stopRecord) {
	t.Helper()
	label := fmt.Sprintf("in the effects %s, then in the plan after them %s", first.label(), second.label())
	plan, live := newProbe(dir, osAccess{}, nil), newProbe(dir, osAccess{}, &first)
	testAccess.Store(dir, passAccess{live: live, plan: plan})
	defer testAccess.Delete(dir)
	afterEffect = func() { plan.arm(&second) }
	err := barrier(dir, mailbox, true)
	afterEffect = func() {}
	if live.hits == 0 || plan.hits == 0 {
		return false, nil
	}
	recorded, read := recordedStop(dir, mailbox)
	if open(outcome(err)) || read != nil || recorded == nil || !recorded.Effect {
		t.Errorf("%s: barrier %s, stop on record %+v %v", label, outcome(err), recorded, read)
		return true, nil
	}
	whole := recorded
	if !strings.Contains(recorded.Met, first.at.rel) || !strings.Contains(recorded.Cause, second.at.rel) {
		t.Errorf("%s: the record does not keep both causes: %+v", label, recorded)
		whole = nil
	}
	if !strings.Contains(err.Error(), recorded.Met) {
		t.Errorf("%s: the barrier answers %v, which does not name the effect's cause", label, err)
	}
	plan.arm(nil)
	if gate := outcome(MailboxStopped(dir, mailbox)); open(gate) {
		t.Errorf("%s: with the plan's cause gone the gate answers %s before the effect ran through", label, gate)
	}
	testAccess.Delete(dir)
	clean := runScene(t, dir, mailbox, nil, nil, true)
	if clean.barrier != base.barrier || clean.gateAfter != base.gateAfter {
		t.Errorf("%s: past both the barrier answers %s and the gate %s, without them %s and %s", label, clean.barrier, clean.gateAfter, base.barrier, base.gateAfter)
	}
	if changed := differ(base.tree, clean.tree, true); len(changed) > 0 {
		t.Errorf("%s: past both the mailboxes differ from the scene's: %v", label, changed)
	}
	return true, whole
}
