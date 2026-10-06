package inbox

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/state"
)

// stopAfterListing runs after once, right after the stops of a mailbox are
// listed: where a holder of the lock records an occurrence while a settling
// that does not hold it has decided and not yet removed.
type stopAfterListing struct {
	fileAccess
	root  string
	after func()
}

func (p *stopAfterListing) ReadDir(path string) ([]fs.DirEntry, error) {
	entries, err := p.fileAccess.ReadDir(path)
	if path == p.root && p.after != nil {
		after := p.after
		p.after = nil
		after()
	}
	return entries, err
}

// A lock whose file the server cannot open may still be held: a descriptor
// opened before the file's mode changed keeps its lock. So the server writes
// the status alone and settles nothing until it holds the lock itself — not
// the delivered letter's waiting copy, which the holder names in an
// occurrence recorded after a settling would have read the stops. With the
// lock its own again, a pass after the retry interval settles the letter, and
// none before it. Both ways the server settles go through it: publishing an
// outcome, and finding one on record.
func TestSettlingWaitsForALockTheServerHolds(t *testing.T) {
	for _, via := range []string{"publish", "already settled"} {
		t.Run(via, func(t *testing.T) {
			lab := newTwoSessionLab(t)
			report := lab.report(Finished)
			if err := live(lab.dir).put(report); err != nil {
				t.Fatal(err)
			}
			if err := linkUnread(lab.dir, report.To, report.ID); err != nil {
				t.Fatal(err)
			}
			waiting := filepath.Join(state.InboxPath(lab.dir, report.To), report.ID+".json")
			probe := &stopAfterListing{fileAccess: osAccess{}, root: stopsPath(lab.dir, report.To)}
			testAccess.Store(lab.dir, passAccess{live: probe, plan: osAccess{}})
			t.Cleanup(func() { testAccess.Delete(lab.dir) })
			server := &Server{Dir: lab.dir, Name: report.To, attempts: map[string]time.Time{}, outcomes: map[string]Result{}}
			delivered := Result{State: Delivered, Via: "socket"}
			settled := false
			if err := withLock(lab.dir, report.To, func() error {
				lock := filepath.Join(state.InboxPath(lab.dir, report.To), ".lock")
				if err := os.Chmod(lock, 0); err != nil {
					return err
				}
				defer func() { _ = os.Chmod(lock, 0o600) }()
				probe.after = func() {
					settled = true
					// The holder, whose descriptor stays locked.
					if _, err := (world{dir: lab.dir, files: osAccess{}}).recordOccurrence(report.To, stopCause{Kind: causeUnreadable, Paths: []string{relative(lab.dir, waiting)}, Cause: "the holder names the waiting copy"}); err != nil {
						t.Error(err)
					}
				}
				if via == "publish" {
					server.publish(report.ID, delivered)
				} else {
					if err := writeStatus(lab.dir, report.To, report.ID, delivered); err != nil {
						return err
					}
					server.alreadySettled(report)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			probe.after = nil
			if settled {
				t.Error("the server settled without the lock")
			}
			if _, err := os.Stat(waiting); err != nil {
				t.Fatalf("the waiting copy went without the lock: %v", err)
			}
			if status, ok, err := ReadStatus(lab.dir, report.To, report.ID); err != nil || !ok || status.State != Delivered {
				t.Fatalf("the status was not written alone: %+v, %v, %v", status, ok, err)
			}
			settle := func() {
				if via == "publish" {
					server.publish(report.ID, delivered)
				} else {
					server.alreadySettled(report)
				}
			}
			settle()
			if _, err := os.Stat(waiting); err != nil {
				t.Fatalf("the letter was tried again before the retry interval: %v", err)
			}
			delete(server.attempts, report.ID)
			settle()
			if _, err := os.Stat(waiting); !os.IsNotExist(err) {
				t.Fatalf("a pass holding the lock did not settle the letter: %v", err)
			}
		})
	}
}

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
			letter := lab.report(Task)
			server := &Server{Dir: lab.dir, Name: letter.To, Epoch: letter.ToEpoch, attempts: map[string]time.Time{}, outcomes: map[string]Result{}, arrivals: map[string]*arrival{}, recent: map[string]recentAnnouncement{}, held: map[string][]Message{}, followed: true}
			if scene != "late refusal unread only" {
				letter.ToEpoch = lab.run
			}
			if err := Put(lab.dir, letter); err != nil {
				t.Fatal(err)
			}
			if scene == "foreign refused on record" {
				if err := writeStatus(lab.dir, letter.To, letter.ID, Result{State: Failed, Detail: "refused by the earlier session"}); err != nil {
					t.Fatal(err)
				}
			}
			waiting := filepath.Join(state.InboxPath(lab.dir, letter.To), letter.ID+".json")
			left := waiting
			if scene == "late refusal unread only" {
				if err := linkUnread(lab.dir, letter.To, letter.ID); err != nil {
					t.Fatal(err)
				}
				server.finish(letter, Result{State: Delivered})
				if _, err := os.Stat(waiting); !os.IsNotExist(err) {
					t.Fatalf("the delivered letter kept its waiting copy: %v", err)
				}
				server.recent[letter.ID] = recentAnnouncement{members: []Message{letter}, at: time.Now()}
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
				server.receive(Receipt{ID: letter.ID, Result: Result{State: Failed, Detail: "late refusal"}})
			}
			if status, ok, err := ReadStatus(lab.dir, letter.To, letter.ID); err != nil || !ok || status.State != Failed {
				t.Fatalf("the refusal is not on record: %+v, %v, %v", status, ok, err)
			}
			if _, err := os.Stat(left); err != nil {
				t.Fatalf("the letter was settled without the lock: %v", err)
			}
			if err := os.Chmod(lock, 0o600); err != nil {
				t.Fatal(err)
			}
			later := lab.report(Task)
			later.ToEpoch = lab.run
			if err := Put(lab.dir, later); err != nil {
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
			status, err := os.ReadFile(statusPath(lab.dir, letter.To, letter.ID))
			if err != nil {
				t.Fatal(err)
			}
			server.attempts = map[string]time.Time{}
			server.drain(context.Background())
			if again, err := os.ReadFile(statusPath(lab.dir, letter.To, letter.ID)); err != nil || string(again) != string(status) {
				t.Fatalf("a settled outcome was taken up again: %s, %v", again, err)
			}
			if _, known, err := ReadStatus(lab.dir, later.To, later.ID); err != nil || known {
				t.Fatalf("mail for another session that came later was settled: known %v, %v", known, err)
			}
			if _, err := os.Stat(filepath.Join(state.InboxPath(lab.dir, later.To), later.ID+".json")); err != nil {
				t.Fatalf("mail for another session that came later left: %v", err)
			}
			if scene == "late refusal unread only" {
				notes, err := list(lab.dir, letter.From)
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
