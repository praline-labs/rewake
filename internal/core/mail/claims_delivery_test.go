package mail

import (
	"testing"
	"time"
)

// A late failure of a letter being read in parts is recorded as delivered and
// leaves the letter where its reader is: only a read takes it out of unread/.
// A failed status an earlier run left behind does not take it out either.
func TestTheServerKeepsALetterBeingRead(t *testing.T) {
	dir := stateDir(t)
	letter := announcedUnread(t, dir, Message{ID: NewID(), From: "web", FromEpoch: "w1", To: "api", ToEpoch: "e1", Kind: Task, Text: "long", CreatedAt: time.Now()})
	if err := ClaimRead(dir, "api", letter.ID, "token"); err != nil {
		t.Fatal(err)
	}
	server := &Server{Dir: dir, Name: "api", Epoch: "e1", outcomes: map[string]Result{}}
	outcome, err := server.recordLocked(letter.ID, Result{State: Failed, Detail: "late delivery failed"})
	if err != nil {
		t.Fatal(err)
	}
	server.settleOutcome(letter.ID, outcome, false)
	if !StillUnread(dir, "api", letter.ID) {
		t.Fatal("the server took a letter being read out of unread/")
	}
	if status, _, _ := ReadStatus(dir, "api", letter.ID); status.State != Delivered {
		t.Fatalf("its sender reads %s", status.State)
	}
	SettleLocked(dir, "api", letter.ID, Failed)
	if !StillUnread(dir, "api", letter.ID) {
		t.Fatal("an old failed status took a letter being read out of unread/")
	}
}
