package inbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/state"
)

// whileAwaitingTheLock runs settling while another holder has the mailbox
// lock, and lets the holder change the letter once settling has read the
// status and waits for the lock — the moment state.LockWait marks, so no
// sleep stands in for the race.
func whileAwaitingTheLock(t *testing.T, lab twoSessionLab, server *Server, settling func(), change func() error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	server.lockContext = ctx
	waiting := make(chan struct{}, 1)
	finished := make(chan struct{})
	before := state.LockWait
	state.LockWait = func(path string) {
		if path == state.InboxPath(lab.dir, server.Name) {
			select {
			case waiting <- struct{}{}:
			default:
			}
		}
	}
	defer func() { state.LockWait = before }()
	err := withLock(lab.dir, server.Name, func() error {
		go func() { settling(); close(finished) }()
		select {
		case <-waiting:
		case <-finished:
			t.Error("settling never waited for the lock")
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
		return change()
	})
	select {
	case <-finished:
	case <-ctx.Done():
		t.Fatal("settling did not finish")
	}
	if err != nil {
		t.Fatal(err)
	}
}

// changesWhileAwaited are what a holder of the lock may do to a refused
// letter while settling waits for it, each of which keeps the letter in
// unread/: a withdrawal leaves its tombstone there, a read its text, a claim
// a read in parts, and a status that stops reading is no outcome at all.
func changesWhileAwaited(lab twoSessionLab, letter Message) map[string]func() error {
	return map[string]func() error{
		"withdrawn": func() error {
			_, err := Withdraw(lab.dir, letter, nil)
			return err
		},
		"read": func() error {
			return writeStatus(lab.dir, letter.To, letter.ID, Result{State: Read})
		},
		"claimed": func() error {
			return ClaimRead(lab.dir, letter.To, letter.ID, "reader")
		},
		"unreadable": func() error {
			return os.WriteFile(statusPath(lab.dir, letter.To, letter.ID), []byte("{"), 0o600)
		},
	}
}

func keptReadable(t *testing.T, lab twoSessionLab, letter Message, change string) {
	t.Helper()
	if _, err := os.Stat(unreadCopy(lab, letter)); err != nil {
		t.Fatalf("settling took the %s letter out of unread/: %v", change, err)
	}
	if _, err := os.Stat(filepath.Join(state.DonePath(lab.dir, letter.To), letter.ID+".json")); !os.IsNotExist(err) {
		t.Fatalf("settling archived the %s letter: %v", change, err)
	}
}

// Mail for another session that arrives after the start with a refusal on
// record is settled by an ordinary pass. What settles it is the status read
// under the lock that moves it: one changed while the pass waited for that
// lock decides, and the refusal read before it does not.
func TestAForeignLetterSettlesByItsStatusUnderTheLock(t *testing.T) {
	for _, change := range []string{"withdrawn", "read", "claimed", "unreadable"} {
		t.Run(change, func(t *testing.T) {
			lab := newTwoSessionLab(t)
			letter := lab.report(Task)
			letter.ToEpoch = lab.run
			if err := Put(lab.dir, letter); err != nil {
				t.Fatal(err)
			}
			if err := linkUnread(lab.dir, letter.To, letter.ID); err != nil {
				t.Fatal(err)
			}
			if err := writeStatus(lab.dir, letter.To, letter.ID, Result{State: Failed}); err != nil {
				t.Fatal(err)
			}
			server := servingRun(lab, lab.web.Epoch())
			whileAwaitingTheLock(t, lab, server, func() { server.pendingMessages(context.Background()) }, changesWhileAwaited(lab, letter)[change])
			keptReadable(t, lab, letter, change)
			if change == "withdrawn" {
				if status, known, err := ReadStatus(lab.dir, letter.To, letter.ID); err != nil || !known || !status.Withdrawn {
					t.Fatalf("the withdrawal is not on record: %+v, %v, %v", status, known, err)
				}
			}
			if change == "unreadable" {
				if _, err := os.Stat(filepath.Join(state.InboxPath(lab.dir, letter.To), letter.ID+".json")); err != nil {
					t.Fatalf("a letter whose status does not read was settled: %v", err)
				}
				if _, tried := server.attempts[letter.ID]; !tried {
					t.Fatal("a letter whose status does not read was taken for settled")
				}
			}
		})
	}
}

// A refusal left only in unread/ is settled the same way: the status is read
// again under the lock, and a withdrawal, a claim or a stop that came while
// the pass waited for it keeps the letter where it is.
func TestARecordedRefusalIsRecheckedUnderTheLock(t *testing.T) {
	for _, change := range []string{"withdrawn", "claimed", "stop"} {
		t.Run(change, func(t *testing.T) {
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
			if err := writeStatus(lab.dir, letter.To, letter.ID, Result{State: Failed}); err != nil {
				t.Fatal(err)
			}
			changes := changesWhileAwaited(lab, letter)
			changes["stop"] = func() error {
				_, err := live(lab.dir).recordOccurrence(letter.To, stopCause{Kind: causeUnreadable, Paths: []string{relative(lab.dir, unreadCopy(lab, letter))}, Cause: "the holder found the letter unreadable"})
				return err
			}
			server := servingRun(lab, letter.ToEpoch)
			whileAwaitingTheLock(t, lab, server, server.settleRecorded, changes[change])
			keptReadable(t, lab, letter, change)
		})
	}
}

// A refusal on record that a stop keeps from settling is still owed: the
// passes inside the retry interval leave it and its status alone, and the
// first pass after the stop is resolved and the interval has passed settles
// it.
func TestARefusalOnRecordAStopKeepsWaitsTheRetryInterval(t *testing.T) {
	lab := newTwoSessionLab(t)
	letter := lab.report(Task)
	letter.ToEpoch = lab.run
	if err := Put(lab.dir, letter); err != nil {
		t.Fatal(err)
	}
	if err := writeStatus(lab.dir, letter.To, letter.ID, Result{State: Failed, Detail: "refused by the earlier session"}); err != nil {
		t.Fatal(err)
	}
	waiting := filepath.Join(state.InboxPath(lab.dir, letter.To), letter.ID+".json")
	var stop openStop
	if err := withLock(lab.dir, letter.To, func() error {
		var err error
		stop, err = live(lab.dir).recordOccurrence(letter.To, stopCause{Kind: causeUnreadable, Paths: []string{relative(lab.dir, waiting)}, Cause: "the letter does not read"})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	server := servingRun(lab, lab.web.Epoch())
	server.pendingMessages(context.Background())
	if _, err := os.Stat(waiting); err != nil {
		t.Fatalf("the letter moved despite the stop: %v", err)
	}
	status, err := os.ReadFile(statusPath(lab.dir, letter.To, letter.ID))
	if err != nil {
		t.Fatal(err)
	}
	server.pendingMessages(context.Background())
	if again, err := os.ReadFile(statusPath(lab.dir, letter.To, letter.ID)); err != nil || string(again) != string(status) {
		t.Fatalf("the letter was taken up again inside the retry interval: %s, %v", again, err)
	}
	if err := withLock(lab.dir, letter.To, func() error {
		return live(lab.dir).resolveStop(letter.To, stop, []string{"the letter reads again"})
	}); err != nil {
		t.Fatal(err)
	}
	server.attempts = map[string]time.Time{}
	server.pendingMessages(context.Background())
	if _, err := os.Stat(waiting); !os.IsNotExist(err) {
		t.Fatalf("the refused letter was not settled once it could be: %v", err)
	}
	if _, err := os.Stat(filepath.Join(state.DonePath(lab.dir, letter.To), letter.ID+".json")); err != nil {
		t.Fatalf("the refused letter was not archived: %v", err)
	}
}
