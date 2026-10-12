//go:build rewakefault

package delivery

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/infra/state"
)

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
			report := lab.report(mail.Finished)
			if err := mail.Put(lab.dir, report); err != nil {
				t.Fatal(err)
			}
			if err := mail.LinkUnread(lab.dir, report.To, report.ID); err != nil {
				t.Fatal(err)
			}
			waiting := filepath.Join(state.InboxPath(lab.dir, report.To), report.ID+".json")
			server := &Server{Dir: lab.dir, Name: report.To, attempts: map[string]time.Time{}, outcomes: map[string]mail.Result{}}
			delivered := mail.Result{State: mail.Delivered, Via: "socket"}
			settled := false
			if err := withLock(lab.dir, report.To, func() error {
				lock := filepath.Join(state.InboxPath(lab.dir, report.To), ".lock")
				if err := os.Chmod(lock, 0); err != nil {
					return err
				}
				defer func() { _ = os.Chmod(lock, 0o600) }()
				restore := mail.AfterStopListing(lab.dir, report.To, func() {
					settled = true
					// The holder, whose descriptor stays locked.
					stopNaming(t, lab.dir, report.To, waiting)
				})
				t.Cleanup(restore)
				defer restore()
				if via == "publish" {
					server.publish(report.ID, delivered)
				} else {
					if err := mail.WriteStatus(lab.dir, report.To, report.ID, delivered); err != nil {
						return err
					}
					server.alreadySettled(report)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if settled {
				t.Error("the server settled without the lock")
			}
			if _, err := os.Stat(waiting); err != nil {
				t.Fatalf("the waiting copy went without the lock: %v", err)
			}
			if status, ok, err := mail.ReadStatus(lab.dir, report.To, report.ID); err != nil || !ok || status.State != mail.Delivered {
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
