package inbox

import (
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/state"
)

// A message that moves on while the mailbox is being walked is found further
// along, not missed in both places: the walk follows its way, waiting, unread,
// done. Here it is read right after the first listing, whichever that is.
func TestTheWalkFindsAMessageThatMovesOn(t *testing.T) {
	dir := stateDir(t)
	sent := Message{ID: NewID(), From: "lead", FromEpoch: "run-1", To: "api", ToEpoch: "api-1", Kind: Task, Text: "a task", CreatedAt: time.Now()}
	if err := Put(dir, sent); err != nil {
		t.Fatal(err)
	}
	if err := move(sent.ID, state.InboxPath(dir, "api"), state.UnreadPath(dir, "api")); err != nil {
		t.Fatal(err)
	}
	moved := false
	afterListing = func(string) {
		if !moved {
			moved = true
			if err := move(sent.ID, state.UnreadPath(dir, "api"), state.DonePath(dir, "api")); err != nil {
				t.Error(err)
			}
		}
	}
	t.Cleanup(func() { afterListing = func(string) {} })
	if found := sentBy(dir, "api", "lead", "run-1"); len(found) != 1 || found[0].ID != sent.ID {
		t.Fatalf("found %+v, want the task once", found)
	}
}

// A message briefly in two places — linked into unread/ before its waiting
// copy goes — is counted once.
func TestTheWalkCountsAMessageOnce(t *testing.T) {
	dir := stateDir(t)
	sent := Message{ID: NewID(), From: "lead", FromEpoch: "run-1", To: "api", ToEpoch: "api-1", Kind: Task, Text: "a task", CreatedAt: time.Now()}
	if err := Put(dir, sent); err != nil {
		t.Fatal(err)
	}
	if err := Put(dir, sent); err != nil {
		t.Fatal(err)
	}
	if err := move(sent.ID, state.InboxPath(dir, "api"), state.UnreadPath(dir, "api")); err != nil {
		t.Fatal(err)
	}
	if err := Put(dir, sent); err != nil {
		t.Fatal(err)
	}
	if found := everywhere(dir, "api"); len(found) != 1 {
		t.Fatalf("found %d copies", len(found))
	}
}
