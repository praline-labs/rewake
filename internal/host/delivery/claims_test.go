package delivery

import (
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
)

// A late failure of a letter being read in parts is recorded as delivered and
// leaves the letter where its reader is: only a read takes it out of unread/.
// A failed status an earlier run left behind does not take it out either.
func TestTheServerKeepsALetterBeingRead(t *testing.T) {
	dir := stateDir(t)
	letter := announcedUnread(t, dir, mail.Message{ID: mail.NewID(), From: "web", FromEpoch: "w1", To: "api", ToEpoch: "e1", Kind: mail.Task, Text: "long", CreatedAt: time.Now()})
	if err := mail.ClaimRead(dir, "api", letter.ID, "token"); err != nil {
		t.Fatal(err)
	}
	server := &Server{Dir: dir, Name: "api", Epoch: "e1", outcomes: map[string]mail.Result{}}
	outcome, err := server.recordLocked(letter.ID, mail.Result{State: mail.Failed, Detail: "late delivery failed"})
	if err != nil {
		t.Fatal(err)
	}
	server.settleOutcome(letter.ID, outcome, false)
	if !mail.StillUnread(dir, "api", letter.ID) {
		t.Fatal("the server took a letter being read out of unread/")
	}
	if status, _, _ := mail.ReadStatus(dir, "api", letter.ID); status.State != mail.Delivered {
		t.Fatalf("its sender reads %s", status.State)
	}
	mail.SettleLocked(dir, "api", letter.ID, mail.Failed)
	if !mail.StillUnread(dir, "api", letter.ID) {
		t.Fatal("an old failed status took a letter being read out of unread/")
	}
}
