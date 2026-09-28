package cli

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

func TestReviewSenderObservationCannotBlockReceiverMailbox(t *testing.T) {
	dir, self, peer := stateCaller(t, "main")
	peer.HarnessPID, peer.HarnessStart = 2147483647, 1
	if err := registry.Update(dir, peer); err != nil {
		t.Fatal(err)
	}
	leaveUnread(t, dir, inbox.Message{From: peer.Name, FromEpoch: peer.Epoch(), To: self.Name, ToEpoch: self.Epoch(), Kind: inbox.Finished, Text: "report must remain readable"})
	held, release, unlocked := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(unlocked)
		_ = state.WithNameLock(dir, peer.Name, func() error { close(held); <-release; return nil })
	}()
	<-held
	done := make(chan string, 1)
	go func() { _, output, _ := run("inbox"); done <- output }()
	completed := false
	var output string
	select {
	case output = <-done:
		completed = true
	case <-time.After(300 * time.Millisecond):
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	mailboxErr := state.WithMailboxLock(ctx, dir, self.Name, func() error { return nil })
	cancel()
	close(release)
	<-unlocked
	if !completed {
		select {
		case output = <-done:
		case <-time.After(time.Second):
			t.Fatal("inbox did not recover after releasing the sender registry lock")
		}
	}
	if !strings.Contains(output, "report must remain readable") {
		t.Fatal("report body lost")
	}
	t.Logf("inbox completed while sender lock held=%v receiverMailboxError=%v", completed, mailboxErr)
	if !completed || mailboxErr != nil {
		t.Fatal("optional sender telemetry blocked the receiver inbox and its mailbox")
	}
}
