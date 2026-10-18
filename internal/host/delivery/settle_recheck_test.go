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
// unread/: a withdrawal leaves its tombstone there, a read its text, and a
// status that stops reading is no outcome at all.
func changesWhileAwaited(lab twoSessionLab, letter mail.Message) map[string]func() error {
	return map[string]func() error{
		"withdrawn": func() error {
			_, err := mail.Withdraw(lab.dir, letter, nil)
			return err
		},
		"read": func() error {
			return mail.WriteStatus(lab.dir, letter.To, letter.ID, mail.Result{State: mail.Read})
		},
		"unreadable": func() error {
			return os.WriteFile(mailtest.StatusPath(lab.dir, letter.To, letter.ID), []byte("{"), 0o600)
		},
	}
}

func keptReadable(t *testing.T, lab twoSessionLab, letter mail.Message, change string) {
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
	for _, change := range []string{"withdrawn", "read", "unreadable"} {
		t.Run(change, func(t *testing.T) {
			lab := newTwoSessionLab(t)
			letter := lab.report(mail.Task)
			letter.ToEpoch = lab.run
			if err := mail.Put(lab.dir, letter); err != nil {
				t.Fatal(err)
			}
			if err := mail.LinkUnread(lab.dir, letter.To, letter.ID); err != nil {
				t.Fatal(err)
			}
			if err := mail.WriteStatus(lab.dir, letter.To, letter.ID, mail.Result{State: mail.Failed}); err != nil {
				t.Fatal(err)
			}
			server := servingRun(lab, lab.web.Epoch())
			whileAwaitingTheLock(t, lab, server, func() { server.pendingMessages(context.Background()) }, changesWhileAwaited(lab, letter)[change])
			keptReadable(t, lab, letter, change)
			if change == "withdrawn" {
				if status, known, err := mail.ReadStatus(lab.dir, letter.To, letter.ID); err != nil || !known || !status.Withdrawn {
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
// again under the lock, and a withdrawal that came while
// the pass waited for it keeps the letter where it is.
func TestARecordedRefusalIsRecheckedUnderTheLock(t *testing.T) {
	for _, change := range []string{"withdrawn"} {
		t.Run(change, func(t *testing.T) {
			lab := newTwoSessionLab(t)
			letter := lab.report(mail.Task)
			if err := mail.Put(lab.dir, letter); err != nil {
				t.Fatal(err)
			}
			if err := mail.LinkUnread(lab.dir, letter.To, letter.ID); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(filepath.Join(state.InboxPath(lab.dir, letter.To), letter.ID+".json")); err != nil {
				t.Fatal(err)
			}
			if err := mail.WriteStatus(lab.dir, letter.To, letter.ID, mail.Result{State: mail.Failed}); err != nil {
				t.Fatal(err)
			}
			changes := changesWhileAwaited(lab, letter)
			server := servingRun(lab, letter.ToEpoch)
			whileAwaitingTheLock(t, lab, server, server.settleRecorded, changes[change])
			keptReadable(t, lab, letter, change)
		})
	}
}
