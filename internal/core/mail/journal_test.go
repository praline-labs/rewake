package mail

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/registry/registrytest"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// journalLab is api owing web one question, with the journal of the turn end
// answering it written and nothing of it done.
func journalLab(t *testing.T) (string, string, Message) {
	t.Helper()
	dir := stateDir(t)
	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	web := registry.Session{Name: "web", ServicePID: os.Getpid(), ServiceStart: start, Boot: registrytest.Boot(t), CWD: dir, StartedAt: time.Now()}
	if err := registry.Publish(dir, web); err != nil {
		t.Fatal(err)
	}
	epoch := "api-epoch"
	if err := markAwaiting(dir, "api", epoch, "web", web.Epoch(), "question"); err != nil {
		t.Fatal(err)
	}
	waiters, err := ReadWaiters(dir, "api", epoch)
	if err != nil || len(waiters) != 1 {
		t.Fatalf("the wait: %v %v", waiters, err)
	}
	report := Message{ID: ReportID("api", epoch, waiters[0]), From: "api", FromEpoch: epoch, To: "web", ToEpoch: web.Epoch(), Kind: Finished, Text: "the answer", InReplyTo: []string{"question"}, CreatedAt: time.Now()}
	if err := WriteJournal(dir, "api", "end", TurnJournal{Epoch: epoch, Reports: []Message{report}, Clear: waiters}); err != nil {
		t.Fatal(err)
	}
	return dir, epoch, report
}

func finishAll(t *testing.T, dir string) error {
	t.Helper()
	return state.WithMailboxLock(context.Background(), dir, "api", func() error {
		return Reconcile(context.Background(), dir, "api")
	})
}

// A journal takes the kept answer it carried and no other: an answer kept
// after it is a later turn end's.
func TestAJournalTakesOnlyTheAnswerItCarried(t *testing.T) {
	dir, epoch, report := journalLab(t)
	if err := KeepAnswer(dir, "api", epoch, "the carried answer", "", ""); err != nil {
		t.Fatal(err)
	}
	taken, _, err := KeptAnswerThrough(dir, "api", epoch, nil)
	version := taken.Version
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteJournal(dir, "api", "end", TurnJournal{Epoch: epoch, Reports: []Message{report}, Kept: &version}); err != nil {
		t.Fatal(err)
	}
	if err := KeepAnswer(dir, "api", epoch, "a later answer", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := finishAll(t, dir); err != nil {
		t.Fatal(err)
	}
	if text, kept, err := KeptAnswer(dir, "api", epoch); err != nil || !kept || text != "a later answer" {
		t.Fatalf("the journal took a later answer: %q %v %v", text, kept, err)
	}
}

// A completed journal is kept while the run that wrote it lives, for a retry
// of its turn end to find however late, and goes once another run holds the
// name. An unfinished one stays, whoever's.
func TestADoneJournalIsKeptWhileItsRunLives(t *testing.T) {
	dir, epoch, _ := journalLab(t)
	if err := finishAll(t, dir); err != nil {
		t.Fatal(err)
	}
	if err := WriteJournal(dir, "api", "unfinished", TurnJournal{Epoch: "ended-epoch", Clear: []Waiter{{Name: "web", Epoch: "gone"}}}); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-keepFinished - time.Hour)
	age := func() {
		for _, file := range []string{"end" + doneSuffix, "unfinished"} {
			if err := os.Chtimes(filepath.Join(JournalPath(dir, "api"), file), old, old); err != nil {
				t.Fatal(err)
			}
		}
	}
	age()
	sweepTurnRecords(dir, "api", epoch, time.Now().Add(-keepFinished))
	if recorded, err := JournalRecorded(dir, "api", "end"); err != nil || !recorded {
		t.Fatalf("the live run's done journal went: %v %v", recorded, err)
	}
	age()
	sweepTurnRecords(dir, "api", "next-epoch", time.Now().Add(-keepFinished))
	if recorded, _ := JournalRecorded(dir, "api", "end"); recorded {
		t.Error("an ended run's done journal was kept")
	}
	if recorded, _ := JournalRecorded(dir, "api", "unfinished"); !recorded {
		t.Error("an unfinished journal was swept")
	}
}

// A write of a journal that never finished is not a journal: read as one, it
// would stop every turn end of the mailbox.
func TestAnAbandonedJournalWriteIsNotAJournal(t *testing.T) {
	dir, _, _ := journalLab(t)
	if err := os.WriteFile(filepath.Join(JournalPath(dir, "api"), ".tmp-123"), []byte("{\"Epo"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := finishAll(t, dir); err != nil {
		t.Fatal(err)
	}
}
