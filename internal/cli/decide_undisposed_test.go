package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// An occurrence about a decided report that its decision does not dispose
// still stops the mailbox, and every view that says so derives it from the
// listing: the session's stop in list, decide's own listing, and the
// answer of the same words again, which keeps the decision and names the
// occurrence instead of calling the mailbox unstopped.
func TestEveryStopViewNamesAnOccurrenceTheDecisionDoesNotDispose(t *testing.T) {
	lab := newDecideLab(t)
	(&route{}).install(t)
	if code, out, errOut := run("decide", "api", lab.report.ID, "--arrived"); code != ExitOK {
		t.Fatalf("decide: exit %d\n%s%s", code, out, errOut)
	}
	if view := stoppedOf(lab.dir, "api"); view != nil {
		t.Fatalf("the decision disposes its occurrence, yet the stop is %+v", view)
	}
	again := recordAgain(t, lab.dir, "api")
	if view := stoppedOf(lab.dir, "api"); view == nil || view.Undecidable != 1 || len(view.Decidable) != 0 {
		t.Fatalf("the session's stop: %+v", view)
	}
	var listing decideListModel
	if _, out, _ := run("decide", "api", "--json"); json.Unmarshal([]byte(out), &listing) != nil || len(listing.Undecidable) != 1 || listing.Undecidable[0].Path != again {
		t.Fatalf("decide's listing:\n%s", out)
	}
	if len(listing.Subjects) != 1 || listing.Subjects[0].Decided == nil {
		t.Fatalf("the decided subject left the listing: %+v", listing.Subjects)
	}
	_, out, errOut := run("decide", "api", lab.report.ID, "--arrived", "--json")
	var answer decideModel
	if err := json.Unmarshal([]byte(out), &answer); err != nil {
		t.Fatalf("%v:\n%s%s", err, out, errOut)
	}
	if !answer.Before || !answer.Stopped || len(answer.Undecidable) != 1 || answer.Undecidable[0].Path != again {
		t.Fatalf("the same words again: %+v", answer)
	}
}

// recordAgain records a second occurrence of the one cause name's mailbox has
// open, under a new id, as a later look that found it again would, and
// answers its path.
func recordAgain(t *testing.T, dir, name string) string {
	t.Helper()
	records, _ := filepath.Glob(filepath.Join(state.InboxPath(dir, name), "stops", "*", "*"))
	var first string
	for _, record := range records {
		if base := filepath.Base(record); !strings.Contains(base, ".") {
			if first != "" {
				t.Fatalf("more than one occurrence: %v", records)
			}
			first = record
		}
	}
	if first == "" {
		t.Fatalf("no occurrence: %v", records)
	}
	raw, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	again := filepath.Join(filepath.Dir(first), mail.NewID())
	if err := os.WriteFile(again, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return again
}
