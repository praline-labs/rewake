package mail

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// copies says which of a letter's copies stand: waiting, readable, archived.
func copies(t *testing.T, dir string, letter Message) (waiting, unread, done bool) {
	t.Helper()
	stands := func(stage string) bool {
		_, err := os.Lstat(filepath.Join(stage, letter.ID+".json"))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		return err == nil
	}
	return stands(state.InboxPath(dir, letter.To)), stands(state.UnreadPath(dir, letter.To)), stands(state.DonePath(dir, letter.To))
}

// announcedLetter writes a task of api's run to web whose notice is on its
// way: it waits in the mailbox and is readable in unread/.
func announcedLetter(t *testing.T, lab twoSessionLab) Message {
	t.Helper()
	letter := lab.report(Task)
	if err := live(lab.dir).put(letter); err != nil {
		t.Fatal(err)
	}
	if err := LinkUnread(lab.dir, letter.To, letter.ID); err != nil {
		t.Fatal(err)
	}
	return letter
}

func statusBytes(t *testing.T, dir string, letter Message) string {
	t.Helper()
	raw, err := os.ReadFile(statusPath(dir, letter.To, letter.ID))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return string(raw)
}

// The first final status wins: what the harness says of a notice after the
// agent read the letter, or its sender took it back, is not written over it,
// and an earlier status that is not final is. A status that cannot be read
// may be final, so nothing is written over it either.
func TestARecordedResultGivesWayToAFinalStatus(t *testing.T) {
	read := Result{State: Read, Via: "inbox"}
	withdrawal := Result{State: Failed, Withdrawn: true, Detail: "withdrawn by web"}
	for _, c := range []struct {
		name     string
		onRecord *Result
		given    Result
		kept     bool
		stands   Result
	}{
		{name: "nothing on record", given: Result{State: Pending, Detail: "queued"}, stands: Result{State: Pending, Detail: "queued"}},
		{name: "over pending", onRecord: &Result{State: Pending}, given: Result{State: Delivered, Via: "socket"}, stands: Result{State: Delivered, Via: "socket"}},
		{name: "over delivered", onRecord: &Result{State: Delivered}, given: Result{State: Failed, Detail: "late"}, stands: Result{State: Failed, Detail: "late"}},
		{name: "read, then failed", onRecord: &read, given: Result{State: Failed, Detail: "said after the read"}, kept: true, stands: read},
		{name: "read, then delivered", onRecord: &read, given: Result{State: Delivered}, kept: true, stands: read},
		{name: "withdrawn, then delivered", onRecord: &withdrawal, given: Result{State: Delivered}, kept: true, stands: withdrawal},
		{name: "withdrawn, then pending", onRecord: &withdrawal, given: Result{State: Pending}, kept: true, stands: withdrawal},
	} {
		t.Run(c.name, func(t *testing.T) {
			lab := newTwoSessionLab(t)
			letter := announcedLetter(t, lab)
			if c.onRecord != nil {
				if err := WriteStatus(lab.dir, letter.To, letter.ID, *c.onRecord); err != nil {
					t.Fatal(err)
				}
			}
			before := statusBytes(t, lab.dir, letter)
			recorded, err := Open(lab.dir, letter.To, "").Record(letter.ID, c.given)
			if err != nil {
				t.Fatal(err)
			}
			if recorded.Kept != c.kept || recorded.Result != c.stands {
				t.Fatalf("recorded %+v, want kept %v standing %+v", recorded, c.kept, c.stands)
			}
			status, known, err := ReadStatus(lab.dir, letter.To, letter.ID)
			if err != nil || !known || status.Result() != c.stands {
				t.Fatalf("the status on record is %+v (%v, %v), want %+v", status, known, err, c.stands)
			}
			if c.kept && statusBytes(t, lab.dir, letter) != before {
				t.Fatal("a final status was written over")
			}
		})
	}
	t.Run("unreadable", func(t *testing.T) {
		lab := newTwoSessionLab(t)
		letter := announcedLetter(t, lab)
		writeRaw(t, statusPath(lab.dir, letter.To, letter.ID), "{")
		if _, err := Open(lab.dir, letter.To, "").Record(letter.ID, Result{State: Delivered}); err == nil {
			t.Fatal("a status that does not read was taken for none")
		}
		if statusBytes(t, lab.dir, letter) != "{" {
			t.Fatal("a status that does not read was written over")
		}
	})
}

// Settling with a result records it and puts the letter where what stands
// says: delivered keeps it readable, refused archives it, a refused report
// kept readable stays like a delivered one, held keeps both copies. A final
// status on record settles it as itself, whatever the harness said: a read
// letter is not archived as refused, and a withdrawal's tombstone stays to be
// read.
func TestSettlingWithAResultPutsTheLetterWhereWhatStandsSays(t *testing.T) {
	for _, c := range []struct {
		name                  string
		onRecord              *Result
		given                 Result
		waiting, unread, done bool
	}{
		{name: "delivered", given: Result{State: Delivered, Via: "socket"}, unread: true},
		{name: "refused", given: Result{State: Failed, Detail: "the session ended"}, done: true},
		{name: "report kept readable", given: Result{State: Failed, ReportAvailable: true, Detail: "the notice failed"}, unread: true},
		{name: "held", given: Result{State: Held, Via: "socket"}, waiting: true, unread: true},
		{name: "read, then refused", onRecord: &Result{State: Read}, given: Result{State: Failed, Detail: "said after the read"}, unread: true},
		{name: "withdrawn, then refused", onRecord: &Result{State: Failed, Withdrawn: true}, given: Result{State: Failed, Detail: "expired"}, unread: true},
	} {
		t.Run(c.name, func(t *testing.T) {
			lab := newTwoSessionLab(t)
			letter := announcedLetter(t, lab)
			if c.onRecord != nil {
				if err := WriteStatus(lab.dir, letter.To, letter.ID, *c.onRecord); err != nil {
					t.Fatal(err)
				}
			}
			var recorded Recorded
			if err := Open(lab.dir, letter.To, "").WithLock(t.Context(), func(l Locked) (err error) {
				recorded, err = l.SettleWith(letter.ID, c.given)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if !recorded.Settled {
				t.Fatalf("the letter was not settled: %+v", recorded)
			}
			if waiting, unread, done := copies(t, lab.dir, letter); waiting != c.waiting || unread != c.unread || done != c.done {
				t.Fatalf("copies waiting %v, unread %v, done %v; want %v, %v, %v", waiting, unread, done, c.waiting, c.unread, c.done)
			}
			if status, _, _ := ReadStatus(lab.dir, letter.To, letter.ID); c.onRecord == nil && status.Result() != c.given || c.onRecord != nil && status.Result() != *c.onRecord {
				t.Fatalf("the status on record is %+v", status)
			}
		})
	}
	t.Run("unreadable", func(t *testing.T) {
		lab := newTwoSessionLab(t)
		letter := announcedLetter(t, lab)
		writeRaw(t, statusPath(lab.dir, letter.To, letter.ID), "{")
		if err := Open(lab.dir, letter.To, "").WithLock(t.Context(), func(l Locked) error {
			recorded, err := l.SettleWith(letter.ID, Result{State: Failed})
			if err == nil || recorded.Settled {
				t.Errorf("a status that does not read settled the letter: %+v, %v", recorded, err)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if waiting, unread, done := copies(t, lab.dir, letter); !waiting || !unread || done {
			t.Fatalf("copies waiting %v, unread %v, done %v; want both kept", waiting, unread, done)
		}
	})
}

// Only a refusal owes the archive. A letter left in unread/ on purpose stays
// there when it is settled by its status, and its status as it was: held,
// delivered, a report made readable after its notice failed, a withdrawal's
// tombstone — and one whose status does not read, or holds no outcome yet,
// which settles nothing.
func TestOnlyARefusalOnRecordIsArchived(t *testing.T) {
	for _, c := range []struct {
		name     string
		onRecord string
		decided  bool
		archives bool
	}{
		{name: "refused", onRecord: `{"state":"failed","detail":"the session ended"}`, decided: true, archives: true},
		{name: "held", onRecord: `{"state":"held"}`},
		{name: "pending", onRecord: `{"state":"pending"}`},
		{name: "delivered", onRecord: `{"state":"delivered"}`, decided: true},
		{name: "report", onRecord: `{"state":"failed","reportAvailable":true}`, decided: true},
		{name: "withdrawn", onRecord: `{"state":"failed","withdrawn":true,"detail":"withdrawn by api"}`, decided: true},
		{name: "none"},
		{name: "unreadable", onRecord: "{"},
	} {
		t.Run(c.name, func(t *testing.T) {
			lab := newTwoSessionLab(t)
			letter := lab.report(Task)
			landUnread(t, lab.dir, letter)
			if c.onRecord != "" {
				writeRaw(t, statusPath(lab.dir, letter.To, letter.ID), c.onRecord)
			}
			if status, _, err := ReadStatus(lab.dir, letter.To, letter.ID); err == nil && status.Archives() != c.archives {
				t.Fatalf("Archives = %v, want %v", status.Archives(), c.archives)
			}
			if err := Open(lab.dir, letter.To, "").WithLock(t.Context(), func(l Locked) error {
				recorded, decided, err := l.SettleOnRecord(letter.ID)
				if decided != c.decided || (err != nil) != (c.name == "unreadable") || decided && !recorded.Settled {
					t.Errorf("settled %+v, decided %v, err %v", recorded, decided, err)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if _, unread, done := copies(t, lab.dir, letter); unread == c.archives || done != c.archives {
				t.Fatalf("copies unread %v, done %v; archived wanted %v", unread, done, c.archives)
			}
			if statusBytes(t, lab.dir, letter) != c.onRecord {
				t.Fatal("settling by the status wrote it again")
			}
		})
	}
}
