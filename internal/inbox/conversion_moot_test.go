package inbox

import (
	"testing"

	"github.com/praline-labs/rewake/internal/registry"
)

// A report of the earlier build whose recipient is a run of this build that
// was replaced, or whose name is gone, can reach nobody: nothing proves it
// either way, and still it neither stops the mailbox nor goes out
// (docs/turn-end-recovery.md#reconciliation).
func TestAReportForAnEndedRunOfThisBuildIsMoot(t *testing.T) {
	for _, recipient := range []string{"replaced", "gone"} {
		t.Run(recipient, func(t *testing.T) {
			lab := newConversionLab(t)
			lab.owe(t, "t1")
			report := lab.report(Finished, "t1")
			report.ToEpoch = registry.RunEpoch(lab.web.ServicePID, lab.web.ServiceStart+1, lab.web.Boot)
			if recipient == "gone" {
				report.To = "left"
			}
			lab.receipt(t, false, false, report)
			if err := lab.reconcile(t); err != nil {
				t.Fatalf("a report nobody can receive stopped the mailbox: %v", err)
			}
			if MailboxStopped(lab.dir, "api") != nil || lab.copies(t, report.ID) != 0 {
				t.Fatalf("stopped=%v copies=%d", MailboxStopped(lab.dir, "api"), lab.copies(t, report.ID))
			}
		})
	}
}

// What a moot closing report answered is cleared with the conversion, as
// what a published one answered is: no report will answer it, and a wait
// left owed would be taken over by adoption for a run nobody can report to
// (docs/turn-end-recovery.md#reconciliation, step 6).
func TestAMootReportClearsWhatItAnswered(t *testing.T) {
	lab := newConversionLab(t)
	lab.owe(t, "task", "other")
	lab.receipt(t, false, false, lab.report(Finished, "task"))
	if _, err := registry.RemoveOwned(lab.dir, "web", lab.web.Epoch()); err != nil {
		t.Fatal(err)
	}
	if err := lab.reconcile(t); err != nil {
		t.Fatal(err)
	}
	if lab.owes(t, "task") || !lab.owes(t, "other") {
		t.Fatalf("owed after the conversion: task %v, other %v", lab.owes(t, "task"), lab.owes(t, "other"))
	}
}
