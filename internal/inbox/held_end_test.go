package inbox

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// The journal that took a held end's kept answer names that end in every form
// it takes, and a retention sweep while its run lives keeps it: the held end
// confirmed again learns it was published with its continuation, however late.
func TestAJournalThatTookAHeldEndNamesItInEveryForm(t *testing.T) {
	dir, epoch, report := journalLab(t)
	if err := KeepAnswer(dir, "api", epoch, "the held answer", "held-op", "the reason"); err != nil {
		t.Fatal(err)
	}
	taken, ok, err := KeptAnswerThrough(dir, "api", epoch, nil)
	if err != nil || !ok || taken.Held != "held-op" {
		t.Fatalf("the kept answer: %+v %v %v", taken, ok, err)
	}
	if err := WriteJournal(dir, "api", "end", TurnJournal{Epoch: epoch, Reports: []Message{report}, Kept: &taken.Version, Held: taken.Held}); err != nil {
		t.Fatal(err)
	}
	held := func(form string) {
		t.Helper()
		if got, err := HeldEndTaken(dir, "api", epoch, "held-op"); err != nil || !got {
			t.Fatalf("%s: the journal does not name the held end: %v %v", form, got, err)
		}
		if got, _ := HeldEndTaken(dir, "api", "another-epoch", "held-op"); got {
			t.Fatalf("%s: another run's end was taken for this one", form)
		}
	}
	held("open")
	if err := finishAll(t, dir); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(JournalPath(dir, "api"), "end"+doneSuffix))
	if err != nil {
		t.Fatal(err)
	}
	if journal, err := parseJournal("end", raw); err != nil || !journal.Done || journal.Held != "held-op" {
		t.Fatalf("the done journal: %+v %v", journal, err)
	}
	held("done")
	old := time.Now().Add(-keepFinished - time.Hour)
	if err := os.Chtimes(filepath.Join(JournalPath(dir, "api"), "end"+doneSuffix), old, old); err != nil {
		t.Fatal(err)
	}
	sweepTurnRecords(dir, "api", epoch, time.Now().Add(-keepFinished))
	held("swept while its run lives")
}

// A hold whose clock commit was lost leaves its position above the word; the
// held end confirmed again raises the word to it, and allocates nothing.
func TestAHeldEndRaisesTheClockToItsPosition(t *testing.T) {
	dir, epoch, _ := journalLab(t)
	clock, err := openReadClock(dir, "api", epoch)
	if err != nil {
		t.Fatal(err)
	}
	defer clock.Close()
	before := clock.Snapshot().Through
	if err := KeepAnswer(dir, "api", epoch, "the held answer", "held-op", "the reason"); err != nil {
		t.Fatal(err)
	}
	record, _, _ := readKept(dir, "api", epoch)
	*clock.word() = before
	if reason, ok, err := HeldEnd(dir, "api", epoch, "another-op"); err != nil || ok || reason != "" {
		t.Fatalf("another end was answered as the held one: %q %v %v", reason, ok, err)
	}
	if got := clock.Snapshot().Through; got != before {
		t.Fatalf("another end moved the clock to %d", got)
	}
	if reason, ok, err := HeldEnd(dir, "api", epoch, "held-op"); err != nil || !ok || reason != "the reason" {
		t.Fatalf("the held end: %q %v %v", reason, ok, err)
	}
	if got := clock.Snapshot().Through; got != record.Seq {
		t.Fatalf("the clock is at %d, want the hold's position %d", got, record.Seq)
	}
	high, err := readHigh(filepath.Join(filepath.Dir(clock.file.Name()), ".read-high"))
	if err != nil || string(high) != strconv.FormatUint(record.Seq, 10) {
		t.Fatalf("the reservation is %q, want the hold's %d alone: %v", high, record.Seq, err)
	}
	// A word above the position stays: no boundary is narrowed.
	*clock.word() = record.Seq + 5
	if _, _, err := HeldEnd(dir, "api", epoch, "held-op"); err != nil || clock.Snapshot().Through != record.Seq+5 {
		t.Fatalf("a committed word was moved: %d %v", clock.Snapshot().Through, err)
	}
}

// A journal that cannot be read may be the one that took the held end, so the
// lookup fails rather than answer that none did.
func TestAnUnreadableJournalIsNotTakenForNone(t *testing.T) {
	dir, epoch, _ := journalLab(t)
	if err := os.MkdirAll(JournalPath(dir, "api"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(JournalPath(dir, "api"), "end"), []byte("{torn"), 0o600); err != nil {
		t.Fatal(err)
	}
	if taken, err := HeldEndTaken(dir, "api", epoch, "held-op"); err == nil || taken {
		t.Fatalf("a torn journal answered %v, %v", taken, err)
	}
}
