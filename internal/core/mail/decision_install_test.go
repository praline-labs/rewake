package mail

import (
	"path/filepath"
	"testing"
)

// The installation is the barrier's before its plan, stopped or not, and a
// note that cannot be told stops nothing, so the scenes of
// TestEveryReadOfThePlanStopsBeforeTheFirstEffect take a decision installed.
// Here the installation itself is cut: every write a barrier makes for an
// uninstalled decision is broken once, and every read of its effects fails
// once in the effects alone. Whatever failed, the barriers after it end where
// the scene ends without it: the resend linked once and run once, the note
// told once, the report never published, nothing left open.
func TestDecisionInstallationSurvivesEveryBrokenWrite(t *testing.T) {
	lab := newDecisionLab(t)
	d := lab.decision(t, ChoiceNotArrived)
	writeDecision(t, lab.dir, d)
	template := lab.dir
	work := filepath.Join(probeDir(t), "state")
	base := runScene(t, fresh(t, template, work), "api", nil, nil, true)
	if !open(base.barrier) || !open(base.gateAfter) {
		t.Fatalf("the scene does not run through: %s, %s", base.barrier, base.gateAfter)
	}
	resent, _ := d.resendJournal()
	converges := func(t *testing.T, dir, label string) {
		t.Helper()
		var clean sceneRun
		for range 2 {
			clean = runScene(t, dir, "api", nil, nil, true)
		}
		if !open(clean.barrier) || !open(clean.gateAfter) {
			t.Errorf("%s: past it the barrier answers %s and the gate %s", label, clean.barrier, clean.gateAfter)
		}
		if changed := differ(base.tree, clean.tree, true); len(changed) > 0 {
			t.Errorf("%s: past it the mailboxes differ from the scene's: %v", label, changed)
		}
		letters := lettersOf(t, dir, "web")
		if letters[lab.report.ID] != 0 || letters[resent.Reports[0].ID] != 1 || letters[d.Notes[0].Note.ID] != 1 {
			t.Errorf("%s: web holds %v", label, letters)
		}
	}
	for _, write := range base.written {
		dir := fresh(t, template, work)
		label := "write " + write.String() + " fails once"
		if run := writeRun(dir, "api", &write, nil, false); !run.broke {
			t.Errorf("%s: the barrier did not make it again", label)
			continue
		}
		converges(t, dir, label)
	}
	for _, read := range sorted(base.applied) {
		if ofStop(read.rel, "api") {
			continue
		}
		fault := seamFault{at: read, kind: "no access"}
		dir := fresh(t, template, work)
		writeRun(dir, "api", nil, &fault, false)
		converges(t, dir, "in the effects, "+fault.label())
	}
	t.Logf("%d writes broken once, %d reads failed in the effects", len(base.written), len(base.applied))
}
