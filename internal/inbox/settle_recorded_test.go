package inbox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/state"
)

// servingRun is a server partway through its run, the earlier runs' waits
// already followed.
func servingRun(lab twoSessionLab, epoch string) *Server {
	return &Server{Dir: lab.dir, Name: "web", Epoch: epoch, attempts: map[string]time.Time{}, outcomes: map[string]Result{}, arrivals: map[string]*arrival{}, recent: map[string]recentAnnouncement{}, held: map[string][]Message{}, followed: true}
}

// lateRefusal delivers a task, then has the harness refuse its notice late,
// while the mailbox lock cannot be taken: the refusal is on record and its
// sender told, its waiting copy gone, and its readable copy the only one left.
func lateRefusal(t *testing.T, lab twoSessionLab) (*Server, Message, string) {
	t.Helper()
	letter := lab.report(Task)
	letter.ToEpoch = lab.run
	server := servingRun(lab, lab.run)
	if err := Put(lab.dir, letter); err != nil {
		t.Fatal(err)
	}
	if err := linkUnread(lab.dir, letter.To, letter.ID); err != nil {
		t.Fatal(err)
	}
	server.finish(letter, Result{State: Delivered})
	server.recent[letter.ID] = recentAnnouncement{members: []Message{letter}, at: time.Now()}
	lock := filepath.Join(state.InboxPath(lab.dir, letter.To), ".lock")
	if err := os.Chmod(lock, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(lock, 0o600) })
	server.receive(Receipt{ID: letter.ID, Result: Result{State: Failed, Detail: "late refusal"}})
	if status, ok, err := ReadStatus(lab.dir, letter.To, letter.ID); err != nil || !ok || status.State != Failed {
		t.Fatalf("the refusal is not on record: %+v, %v, %v", status, ok, err)
	}
	if _, err := os.Stat(filepath.Join(state.InboxPath(lab.dir, letter.To), letter.ID+".json")); !os.IsNotExist(err) {
		t.Fatalf("the delivered letter kept its waiting copy: %v", err)
	}
	if _, err := os.Stat(unreadCopy(lab, letter)); err != nil {
		t.Fatalf("the letter was settled without the lock: %v", err)
	}
	return server, letter, lock
}

func unreadCopy(lab twoSessionLab, letter Message) string {
	return filepath.Join(state.UnreadPath(lab.dir, letter.To), letter.ID+".json")
}

// settledOnce says the refused letter is archived and its sender was told of
// the refusal once, when it happened.
func settledOnce(t *testing.T, lab twoSessionLab, letter Message) {
	t.Helper()
	if _, err := os.Stat(unreadCopy(lab, letter)); !os.IsNotExist(err) {
		t.Errorf("the refused letter is still unread: %v", err)
	}
	if _, err := os.Stat(filepath.Join(state.DonePath(lab.dir, letter.To), letter.ID+".json")); err != nil {
		t.Errorf("the refused letter was not archived: %v", err)
	}
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
		t.Errorf("the sender was told of the refusal %d times, want once", told)
	}
}

func drainAfterInterval(server *Server, passes int) {
	for range passes {
		server.attempts = map[string]time.Time{}
		server.drain(context.Background())
	}
}

// A run knows nothing of an earlier run's memory, and a late refusal whose
// settling could not take the lock is in none of the lists a run starts from.
// The status and the readable copy say what is owed: the next run settles it
// at its start when the lock can be taken, or on a pass once it can, without
// telling the sender again.
func TestARestartSettlesALateRefusalLeftUnread(t *testing.T) {
	for _, repaired := range []string{"before the start", "after the start"} {
		t.Run("lock repaired "+repaired, func(t *testing.T) {
			lab := newTwoSessionLab(t)
			_, letter, lock := lateRefusal(t, lab)
			if repaired == "before the start" {
				if err := os.Chmod(lock, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			next := &Server{Dir: lab.dir, Name: letter.To, Epoch: lab.web.Epoch()}
			if !next.begin(context.Background()) {
				t.Fatal("the next run did not begin")
			}
			if repaired == "before the start" {
				settledOnce(t, lab, letter)
				return
			}
			if _, err := os.Stat(unreadCopy(lab, letter)); err != nil {
				t.Fatalf("the letter was settled without the lock: %v", err)
			}
			if err := os.Chmod(lock, 0o600); err != nil {
				t.Fatal(err)
			}
			drainAfterInterval(next, 3)
			settledOnce(t, lab, letter)
		})
	}
}

// A settling under the lock that leaves the letter — an open stop names it,
// or its archive cannot be written — has not settled it. The passes find it
// again by its status and its copy, at the retry interval, without writing
// the status again, and settle it once the stop is resolved or the archive
// works.
func TestASettlingCutShortIsSettledOnceItCan(t *testing.T) {
	for _, blocker := range []string{"an open stop names the letter", "the archive cannot be written"} {
		t.Run(blocker, func(t *testing.T) {
			lab := newTwoSessionLab(t)
			server, letter, lock := lateRefusal(t, lab)
			if err := os.Chmod(lock, 0o600); err != nil {
				t.Fatal(err)
			}
			var release func() error
			if blocker == "an open stop names the letter" {
				if err := withLock(lab.dir, letter.To, func() error {
					stop, err := live(lab.dir).recordOccurrence(letter.To, stopCause{Kind: causeUnreadable, Paths: []string{relative(lab.dir, unreadCopy(lab, letter))}, Cause: "the letter does not read"})
					release = func() error {
						return withLock(lab.dir, letter.To, func() error {
							return live(lab.dir).resolveStop(letter.To, stop, []string{"the letter reads again"})
						})
					}
					return err
				}); err != nil {
					t.Fatal(err)
				}
			} else {
				done := state.DonePath(lab.dir, letter.To)
				writeRaw(t, done, "not a directory")
				release = func() error { return os.Remove(done) }
			}
			status, err := os.ReadFile(statusPath(lab.dir, letter.To, letter.ID))
			if err != nil {
				t.Fatal(err)
			}
			drainAfterInterval(server, 3)
			if _, err := os.Stat(unreadCopy(lab, letter)); err != nil {
				t.Fatalf("the letter moved despite the blocker: %v", err)
			}
			if _, tried := server.attempts[letter.ID]; !tried {
				t.Fatal("a settling cut short was taken for done")
			}
			if again, err := os.ReadFile(statusPath(lab.dir, letter.To, letter.ID)); err != nil || string(again) != string(status) {
				t.Fatalf("the status was written again while the letter waited: %s, %v", again, err)
			}
			if err := release(); err != nil {
				t.Fatal(err)
			}
			server.drain(context.Background())
			if _, err := os.Stat(unreadCopy(lab, letter)); err != nil {
				t.Fatalf("the letter was tried again before the retry interval: %v", err)
			}
			drainAfterInterval(server, 1)
			settledOnce(t, lab, letter)
			if _, tried := server.attempts[letter.ID]; tried {
				t.Fatal("a settled letter is still tried")
			}
		})
	}
}

// A late refusal whose status could not be written either is known to this
// run alone, and the status on disk still says delivered. A pass writes the
// refusal first and then settles the letter by it.
func TestALateRefusalItsStatusMissedIsRecordedThenSettled(t *testing.T) {
	lab := newTwoSessionLab(t)
	letter := lab.report(Task)
	letter.ToEpoch = lab.run
	server := servingRun(lab, lab.run)
	if err := Put(lab.dir, letter); err != nil {
		t.Fatal(err)
	}
	if err := linkUnread(lab.dir, letter.To, letter.ID); err != nil {
		t.Fatal(err)
	}
	server.finish(letter, Result{State: Delivered})
	server.recent[letter.ID] = recentAnnouncement{members: []Message{letter}, at: time.Now()}
	previous := writeStatusFile
	writeStatusFile = func(string, []byte) error { return errors.New("no space left on device") }
	t.Cleanup(func() { writeStatusFile = previous })
	server.receive(Receipt{ID: letter.ID, Result: Result{State: Failed, Detail: "late refusal"}})
	writeStatusFile = previous
	if status, _, err := ReadStatus(lab.dir, letter.To, letter.ID); err != nil || status.State != Delivered {
		t.Fatalf("the refusal reached the status: %+v, %v", status, err)
	}
	if _, err := os.Stat(unreadCopy(lab, letter)); err != nil {
		t.Fatalf("the letter was settled before its status: %v", err)
	}
	drainAfterInterval(server, 3)
	if status, _, err := ReadStatus(lab.dir, letter.To, letter.ID); err != nil || status.State != Failed {
		t.Fatalf("the refusal was not recorded: %+v, %v", status, err)
	}
	settledOnce(t, lab, letter)
}

// Only a refusal owes the archive. A letter left in unread/ on purpose stays
// there through a run's start and its passes, its status as it was: held,
// delivered, a report made readable after its notice failed, a withdrawal's
// tombstone, a letter being read in parts — and one whose status does not
// read, which is no outcome at all.
func TestALetterReadableOnPurposeStaysUnread(t *testing.T) {
	scenes := map[string]func(t *testing.T, lab twoSessionLab, letter Message){
		"held":      recorded(Result{State: Held}),
		"delivered": recorded(Result{State: Delivered}),
		"report":    recorded(Result{State: Failed, ReportAvailable: true}),
		"withdrawn": recorded(Result{State: Failed, Withdrawn: true, Detail: "withdrawn by api"}),
		"in parts":  claimedFailure,
		"unreadable": func(t *testing.T, lab twoSessionLab, letter Message) {
			writeRaw(t, statusPath(lab.dir, letter.To, letter.ID), "{")
		},
	}
	for scene, record := range scenes {
		t.Run(scene, func(t *testing.T) {
			lab := newTwoSessionLab(t)
			letter := lab.report(Task)
			if err := Put(lab.dir, letter); err != nil {
				t.Fatal(err)
			}
			if err := linkUnread(lab.dir, letter.To, letter.ID); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(filepath.Join(state.InboxPath(lab.dir, letter.To), letter.ID+".json")); err != nil {
				t.Fatal(err)
			}
			record(t, lab, letter)
			status, err := os.ReadFile(statusPath(lab.dir, letter.To, letter.ID))
			if err != nil {
				t.Fatal(err)
			}
			server := &Server{Dir: lab.dir, Name: letter.To, Epoch: letter.ToEpoch}
			if !server.begin(context.Background()) {
				t.Fatal("the run did not begin")
			}
			drainAfterInterval(server, 3)
			if _, err := os.Stat(unreadCopy(lab, letter)); err != nil {
				t.Fatalf("the letter left unread/: %v", err)
			}
			if _, err := os.Stat(filepath.Join(state.DonePath(lab.dir, letter.To), letter.ID+".json")); !os.IsNotExist(err) {
				t.Fatalf("the letter was archived: %v", err)
			}
			if again, err := os.ReadFile(statusPath(lab.dir, letter.To, letter.ID)); err != nil || string(again) != string(status) {
				t.Fatalf("the status was written again: %s, %v", again, err)
			}
		})
	}
}

func recorded(result Result) func(t *testing.T, lab twoSessionLab, letter Message) {
	return func(t *testing.T, lab twoSessionLab, letter Message) {
		t.Helper()
		if err := writeStatus(lab.dir, letter.To, letter.ID, result); err != nil {
			t.Fatal(err)
		}
	}
}

func claimedFailure(t *testing.T, lab twoSessionLab, letter Message) {
	t.Helper()
	recorded(Result{State: Failed, Detail: "refused while it was being read"})(t, lab, letter)
	writeRaw(t, filepath.Join(claimsPath(lab.dir, letter.To), letter.ID), "a read")
}
