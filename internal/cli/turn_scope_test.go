package cli

import (
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/inbox"
)

// A retry answers and takes only what its event's read boundary covers,
// whatever its first attempt managed to record (docs/turn-end-recovery.md#the-operation).

// A first attempt whose journal could not be written recorded nothing: its
// retry prepares from the same event, so a question read since is above its
// boundary and not answered with its text.
func TestARetryWhoseFirstAttemptRecordedNothingKeepsItsScope(t *testing.T) {
	dir, self, web := toolSession(t)
	original := readKind(t, dir, web, inbox.Task)
	readOnlyJournals(t, dir)
	event := inbox.TurnEnd{Boundary: boundaryNow(t, dir, self), ID: "first", Text: "FIRST_END_TEXT"}
	if completeTurn(dir, self, event, "") == nil {
		t.Fatal("the journal was not refused")
	}
	writableJournals(t, dir)
	fresh := readKind(t, dir, web, inbox.Question)
	if err := completeTurn(dir, self, event, ""); err != nil {
		t.Fatal(err)
	}
	if answersTo(t, dir, original) != 1 || answersTo(t, dir, fresh) != 0 || !owes(t, dir, self, fresh) {
		t.Fatalf("reports: original %d, fresh %d; fresh owed %v", answersTo(t, dir, original), answersTo(t, dir, fresh), owes(t, dir, self, fresh))
	}
}

// A retry prepared again takes the kept answer its first attempt saw, and no
// answer kept after it.
func TestARetryTakesOnlyTheAnswerItsFirstAttemptSaw(t *testing.T) {
	t.Run("kept since", func(t *testing.T) {
		dir, self, web := toolSession(t)
		readKind(t, dir, web, inbox.Task)
		readOnlyJournals(t, dir)
		event := inbox.TurnEnd{Boundary: boundaryNow(t, dir, self), ID: "first", Text: "FIRST_END_TEXT"}
		if completeTurn(dir, self, event, "") == nil {
			t.Fatal("the journal was not refused")
		}
		writableJournals(t, dir)
		if err := completeTurn(dir, self, inbox.TurnEnd{Boundary: boundaryNow(t, dir, self), ID: "second", Text: "the second is done"}, ""); err != nil {
			t.Fatal(err)
		}
		fresh := readKind(t, dir, web, inbox.Question)
		if err := inbox.KeepAnswer(dir, "api", self.Epoch(), "FRESH_HELD_ANSWER"); err != nil {
			t.Fatal(err)
		}
		if err := completeTurn(dir, self, event, ""); err != nil {
			t.Fatal(err)
		}
		text, kept, err := inbox.KeptAnswer(dir, "api", self.Epoch())
		if err != nil || !kept || text != "FRESH_HELD_ANSWER" || !owes(t, dir, self, fresh) {
			t.Fatalf("kept %q %v (%v); fresh owed %v", text, kept, err, owes(t, dir, self, fresh))
		}
	})
	t.Run("seen", func(t *testing.T) {
		dir, self, web := toolSession(t)
		task := readKind(t, dir, web, inbox.Task)
		if err := inbox.KeepAnswer(dir, "api", self.Epoch(), "HELD_BEFORE_THE_END"); err != nil {
			t.Fatal(err)
		}
		readOnlyJournals(t, dir)
		event := inbox.TurnEnd{Boundary: boundaryNow(t, dir, self), ID: "first", Text: "FIRST_END_TEXT"}
		if completeTurn(dir, self, event, "") == nil {
			t.Fatal("the journal was not refused")
		}
		writableJournals(t, dir)
		if err := completeTurn(dir, self, event, ""); err != nil {
			t.Fatal(err)
		}
		if _, kept, err := inbox.KeptAnswer(dir, "api", self.Epoch()); err != nil || kept {
			t.Fatalf("the answer it saw is still kept: %v %v", kept, err)
		}
		for _, report := range reportsTo(t, dir, "web") {
			if len(report.InReplyTo) == 1 && report.InReplyTo[0] == task && !strings.HasPrefix(report.Text, "HELD_BEFORE_THE_END") {
				t.Fatalf("the report lost the answer it carried: %q", report.Text)
			}
		}
		if answersTo(t, dir, task) != 1 {
			t.Fatalf("reports: %d", answersTo(t, dir, task))
		}
	})
}

// A first attempt with nothing to answer, whose journal could not be
// written, has a boundary below the task read since: its retry does not take
// that task for its own.
func TestAnEndWithNothingOwedKeepsItsScopeOnRetry(t *testing.T) {
	dir, self, web := toolSession(t)
	readOnlyJournals(t, dir)
	event := inbox.TurnEnd{Boundary: boundaryNow(t, dir, self), ID: "first", Text: "FIRST_END_TEXT", Ended: 5}
	if completeTurn(dir, self, event, "") == nil {
		t.Fatal("the journal was not refused")
	}
	writableJournals(t, dir)
	fresh := readKind(t, dir, web, inbox.Task)
	if err := completeTurn(dir, self, event, ""); err != nil {
		t.Fatal(err)
	}
	if answersTo(t, dir, fresh) != 0 || !owes(t, dir, self, fresh) {
		t.Fatalf("the retry answered a task read since: %d", answersTo(t, dir, fresh))
	}
}
