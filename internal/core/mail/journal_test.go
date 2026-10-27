package mail

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/registry/registrytest"
	"github.com/praline-labs/rewake/internal/infra/proc"
)

// journalLab is api owing web one question, with the report that answers it.
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
	waiters, err := readWaiters(dir, "api", epoch)
	if err != nil || len(waiters) != 1 {
		t.Fatalf("the wait: %v %v", waiters, err)
	}
	report := Message{ID: ReportID("api", epoch, waiters[0]), From: "api", FromEpoch: epoch, To: "web", ToEpoch: web.Epoch(), Kind: Finished, Text: "the answer", InReplyTo: []string{"question"}, CreatedAt: time.Now()}
	return dir, epoch, report
}

// A done journal is kept while the run that wrote it lives, for a retry of its
// turn end to find however late, and goes once another run holds the name. A
// journal an earlier build left unfinished goes by its run the same way.
func TestADoneJournalIsKeptWhileItsRunLives(t *testing.T) {
	dir, epoch, _ := journalLab(t)
	if err := writeDoneJournal(dir, "api", "end", TurnJournal{Epoch: epoch, Op: "end", Done: true}); err != nil {
		t.Fatal(err)
	}
	if err := writeJournalFile(filepath.Join(JournalPath(dir, "api"), "unfinished"), TurnJournal{Epoch: epoch}); err != nil {
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
	for _, op := range []string{"end", "unfinished"} {
		if recorded, _ := JournalRecorded(dir, "api", op); recorded {
			t.Errorf("an ended run's journal %s was kept", op)
		}
	}
}

// A write of a journal that never finished is not a journal: read as one, it
// would fail every turn end of the mailbox.
func TestAnAbandonedJournalWriteIsNotAJournal(t *testing.T) {
	dir, epoch, _ := journalLab(t)
	if err := os.MkdirAll(JournalPath(dir, "api"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(JournalPath(dir, "api"), ".tmp-123"), []byte("{\"Epo"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := turnWindowStart(dir, "api", epoch, 0, 100); err != nil {
		t.Fatal(err)
	}
}
