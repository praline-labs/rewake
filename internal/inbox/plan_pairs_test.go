package inbox

import (
	"fmt"
	"slices"
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
	for _, first := range sorted(base.applied) {
		if ofStop(first.rel, mailbox) {
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

// pairCauses are the causes a pair left on record: the effect's and the
// plan's after it.
type pairCauses struct{ effect, plan string }

// pairFault answers whether the scene produced the pair, and the causes it
// recorded when both are on record.
func pairFault(t *testing.T, dir, mailbox string, first, second seamFault, base sceneRun) (bool, *pairCauses) {
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
	stops, read := stopState(dir, mailbox)
	effect := occurrenceOf(stops, causeEffect, first.at.rel)
	planned := occurrenceOf(stops, "", second.at.rel)
	if answer := outcome(err); open(answer) || read != nil || effect == nil {
		t.Errorf("%s: barrier %s, the effect's occurrence %+v %v", label, answer, effect, read)
		return true, nil
	}
	var whole *pairCauses
	if planned == nil {
		t.Errorf("%s: the plan's cause is not on record beside the effect's: %+v", label, stops)
	} else {
		whole = &pairCauses{effect: effect.record.Cause, plan: planned.record.Cause}
	}
	if !strings.Contains(err.Error(), effect.record.Cause) {
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

// occurrenceOf is the open occurrence of a kind — any the plan finds, when
// kind is empty — that names rel.
func occurrenceOf(stops []openStop, kind, rel string) *openStop {
	for index, stop := range stops {
		effect := stop.record.Kind == causeEffect
		if (kind == causeEffect) == effect && slices.Contains(stop.record.Paths, rel) {
			return &stops[index]
		}
	}
	return nil
}
