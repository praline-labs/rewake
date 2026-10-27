package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/mail"
)

// The adapter-facing confirmation reaches the core hold and publishes its
// continuation with the original answer. The hold rules live in core/mail.

// end is a turn end of the interim lab's session, as an adapter reports it:
// its event and its moment on the boot clock, after the lab's turn start.
func (lab interimLab) end(id string, at int64, kind mail.Kind, text string) adapter.TurnEnd {
	return adapter.TurnEnd{Event: id, Reason: reasonReporting(kind), Answer: text, Started: lab.start, Ended: markAt + at}
}

// reasonReporting is the end reason whose report is of the kind.
func reasonReporting(kind mail.Kind) adapter.EndReason {
	switch kind {
	case mail.Error:
		return adapter.Failed
	case mail.Stopped:
		return adapter.Aborted
	}
	return adapter.Answered
}

// confirm confirms an end with the boundary the run's clock gives now.
func (lab interimLab) confirm(t *testing.T, end adapter.TurnEnd) string {
	t.Helper()
	end.Boundary = boundaryNow(t, lab.dir, lab.self)
	reason, err := ConfirmCompletion(context.Background(), lab.dir, lab.self, end)
	if err != nil {
		t.Fatal(err)
	}
	return reason
}

// reportsOf answers the texts of web's reports of one kind.
func (lab interimLab) reportsOf(t *testing.T, kind mail.Kind) []string {
	t.Helper()
	var texts []string
	for _, report := range reportsTo(t, lab.dir, "web") {
		if mail.KindOf(report) == kind {
			texts = append(texts, report.Text)
		}
	}
	return texts
}

func (lab interimLab) kept() bool {
	_, kept, _ := mail.Open(lab.dir, "api", lab.self.Epoch()).KeptAnswer()
	return kept
}

func TestAConfirmedEndAfterAnInterimOneIsHeldOnce(t *testing.T) {
	lab := newInterimLab(t)
	reason := lab.confirm(t, lab.end("turn-2", 1, mail.Finished, "the suite is green: 40 pass"))
	for _, want := range []string{`ended pending ("the suite is running")`, "web still wait", "rewake pending", "do not repeat it"} {
		if !strings.Contains(reason, want) {
			t.Errorf("the reason lacks %q: %s", want, reason)
		}
	}
	if got := kinds(reportsTo(t, lab.dir, "web")); len(got) != 1 || lab.owed() != 1 {
		t.Fatalf("the hold published %v and left %d owed", got, lab.owed())
	}
	if reason := lab.confirm(t, lab.end("turn-2b", 2, mail.Finished, "nothing more to add")); reason != "" {
		t.Fatalf("the continuation was held again: %s", reason)
	}
	if got := lab.reportsOf(t, mail.Finished); len(got) != 1 || got[0] != "the suite is green: 40 pass\n\nnothing more to add" || lab.owed() != 0 {
		t.Fatalf("after the continuation web holds %q, %d owed", got, lab.owed())
	}
	if lab.kept() {
		t.Error("the kept answer outlived its report")
	}
	if _, interim, _ := mail.Open(lab.dir, "api", lab.self.Epoch()).LastInterim(); interim {
		t.Error("the report left the run marked interim")
	}
}
