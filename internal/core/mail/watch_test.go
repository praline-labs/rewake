package mail

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// A message answered long ago is swept away with its status. A sender that comes
// back after that must not be told its message is still on its way.
func TestASweptAnswerIsNotReportedAsPending(t *testing.T) {
	dir := stateDir(t)
	sent := message("answered a long time ago")

	// What the sweep leaves behind: no message, no status.
	if err := state.EnsureSubdir(state.DonePath(dir, "api")); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if must(Answered(dir, "api", sent.ID)) != true {
		t.Error("a message that is nowhere to be found was reported as still waiting")
	}

	// And while it is waiting, it is waiting.
	if err := Put(dir, sent); err != nil {
		t.Fatalf("put: %v", err)
	}
	if must(Answered(dir, "api", sent.ID)) {
		t.Error("a message still in the mailbox was reported as answered")
	}
}

// The age that matters is the age of the answer. A rename carries the original
// time along, and an old message refused at startup was swept in the same breath.
func TestArchivingRefreshesTheAge(t *testing.T) {
	dir := stateDir(t)
	old := message("written long ago")
	if err := Put(dir, old); err != nil {
		t.Fatalf("put: %v", err)
	}
	ancient := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(filepath.Join(state.InboxPath(dir, "api"), old.ID+".json"), ancient, ancient); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	if err := archive(dir, "api", old.ID); err != nil {
		t.Fatalf("archive: %v", err)
	}
	info, err := os.Stat(filepath.Join(state.DonePath(dir, "api"), old.ID+".json"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if time.Since(info.ModTime()) > time.Minute {
		t.Errorf("the archived message kept the age it was written at: %v", info.ModTime())
	}
}
