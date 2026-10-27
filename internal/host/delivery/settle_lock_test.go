package delivery

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/mail/mailtest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// Settling that waited for the lock is settled by the passes after it, found
// by its status and its copies: a letter of an earlier session, refused at
// the start or found refused on record, and a late refusal of a delivered
// letter whose readable copy is the only one left. The sender hears of the
// refusal once, when it happened; once settled, nothing is taken up again;
// and mail for another session that arrives after the start stays where it
// is, as ever.
func TestADeferredSettlementGetsAnotherPass(t *testing.T) {
	for _, scene := range []string{"foreign waiting", "foreign refused on record", "late refusal unread only"} {
		t.Run(scene, func(t *testing.T) {
			lab := newTwoSessionLab(t)
			letter := lab.report(mail.Task)
			server := &Server{Dir: lab.dir, Name: letter.To, Epoch: letter.ToEpoch, attempts: map[string]time.Time{}, outcomes: map[string]mail.Result{}, arrivals: map[string]*arrival{}, recent: map[string]recentAnnouncement{}, held: map[string][]mail.Message{}, followed: true}
			if scene != "late refusal unread only" {
				letter.ToEpoch = lab.run
			}
			if err := mail.Put(lab.dir, letter); err != nil {
				t.Fatal(err)
			}
			if scene == "foreign refused on record" {
				if err := mail.WriteStatus(lab.dir, letter.To, letter.ID, mail.Result{State: mail.Failed, Detail: "refused by the earlier session"}); err != nil {
					t.Fatal(err)
				}
			}
			waiting := filepath.Join(state.InboxPath(lab.dir, letter.To), letter.ID+".json")
			left := waiting
			if scene == "late refusal unread only" {
				if err := mail.LinkUnread(lab.dir, letter.To, letter.ID); err != nil {
					t.Fatal(err)
				}
				server.finish(letter, mail.Result{State: mail.Delivered})
				if _, err := os.Stat(waiting); !os.IsNotExist(err) {
					t.Fatalf("the delivered letter kept its waiting copy: %v", err)
				}
				server.recent[letter.ID] = recentAnnouncement{members: []mail.Message{letter}, at: time.Now()}
				left = filepath.Join(state.UnreadPath(lab.dir, letter.To), letter.ID+".json")
			}
			lock := filepath.Join(state.InboxPath(lab.dir, letter.To), ".lock")
			writeRaw(t, lock, "")
			if err := os.Chmod(lock, 0); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(lock, 0o600) })
			if scene != "late refusal unread only" {
				server.sweepForeign()
			} else {
				server.receive(context.Background(), mail.Receipt{ID: letter.ID, Result: mail.Result{State: mail.Failed, Detail: "late refusal"}})
			}
			if status, ok, err := mail.ReadStatus(lab.dir, letter.To, letter.ID); err != nil || !ok || status.State != mail.Failed {
				t.Fatalf("the refusal is not on record: %+v, %v, %v", status, ok, err)
			}
			if _, err := os.Stat(left); err != nil {
				t.Fatalf("the letter was settled without the lock: %v", err)
			}
			if err := os.Chmod(lock, 0o600); err != nil {
				t.Fatal(err)
			}
			later := lab.report(mail.Task)
			later.ToEpoch = lab.run
			if err := mail.Put(lab.dir, later); err != nil {
				t.Fatal(err)
			}
			for range 3 {
				server.attempts = map[string]time.Time{}
				server.drain(context.Background())
			}
			if _, err := os.Stat(left); !os.IsNotExist(err) {
				t.Fatalf("a pass holding the lock did not settle the letter: %v", err)
			}
			if _, err := os.Stat(filepath.Join(state.DonePath(lab.dir, letter.To), letter.ID+".json")); err != nil {
				t.Fatalf("the refused letter was not archived: %v", err)
			}
			status, err := os.ReadFile(mailtest.StatusPath(lab.dir, letter.To, letter.ID))
			if err != nil {
				t.Fatal(err)
			}
			server.attempts = map[string]time.Time{}
			server.drain(context.Background())
			if again, err := os.ReadFile(mailtest.StatusPath(lab.dir, letter.To, letter.ID)); err != nil || string(again) != string(status) {
				t.Fatalf("a settled outcome was taken up again: %s, %v", again, err)
			}
			if _, known, err := mail.ReadStatus(lab.dir, later.To, later.ID); err != nil || known {
				t.Fatalf("mail for another session that came later was settled: known %v, %v", known, err)
			}
			if _, err := os.Stat(filepath.Join(state.InboxPath(lab.dir, later.To), later.ID+".json")); err != nil {
				t.Fatalf("mail for another session that came later left: %v", err)
			}
			if scene == "late refusal unread only" {
				notes, err := mail.List(lab.dir, letter.From)
				if err != nil {
					t.Fatal(err)
				}
				told := 0
				for _, note := range notes {
					if note.Undelivered != nil && note.Undelivered.ID == letter.ID {
						told++
					}
				}
				if told != 1 {
					t.Fatalf("the sender was told of the refusal %d times, want once", told)
				}
			}
		})
	}
}
