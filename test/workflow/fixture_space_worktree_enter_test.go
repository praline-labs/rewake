package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// TestFixtureSpaceWorktreeEnter is a worker opened from a template with
// enter_worktree = true, on the fixture: it starts in main's repository with
// no checkout, enters one mid-session with rewake worktree enter, commits
// there, and main's space close keeps that checkout, saying why — the
// checkout the worker claimed after its launch is the space's own.
func TestFixtureSpaceWorktreeEnter(t *testing.T) {
	binary := enterScenario(t, "fixture-space-worktree-enter")
	c := Start(t, Spec{
		Name:         "fixture-space-worktree-enter",
		Harness:      fixtureColumn.harness,
		Observations: fixtureSpaceWorktreeEnterObservations,
		Deadline:     90 * time.Second,
	})
	room, ok := openTreeWorker(t, c, binary, "enter_worktree = true", obsEnterMade)
	if !ok {
		return
	}
	defer stopSession(t, c, room.lead)
	if room.opened.Worktree {
		c.Contradicted(obsEnterMade, "space open made a worktree at the launch")
		return
	}

	var entry struct {
		Outcome  string `json:"outcome"`
		Checkout struct {
			Path, Branch, Base string
		} `json:"checkout"`
	}
	code, out, _ := room.workerAsks.ask(c, "worktree", "enter", "--json")
	head, _ := room.git(room.repo, "rev-parse", "HEAD")
	if code != 0 || json.Unmarshal([]byte(out), &entry) != nil || entry.Outcome != "created" ||
		!strings.HasPrefix(entry.Checkout.Path, room.trees) || entry.Checkout.Base != head {
		c.Contradicted(obsEnterMade, "the worker's enter answered %d: %s", code, out)
		return
	}
	code, out, _ = room.workerAsks.ask(c, "worktree", "status", "--json")
	if code != 0 || !strings.Contains(out, entry.Checkout.Path) {
		c.Contradicted(obsEnterMade, "the worker's status after entry answered %d: %s", code, out)
		return
	}
	c.Observed(obsEnterMade, fmt.Sprintf("the worker entered %s on %s, which its status names", entry.Checkout.Path, entry.Checkout.Branch))

	if _, err := room.git(entry.Checkout.Path, "commit", "-q", "--allow-empty", "-m", "Work"); err != nil {
		t.Fatalf("committing in the checkout: %v", err)
	}
	code, out, _ = room.asks.ask(c, "space", "close", "api", "--json")
	var closed struct{ State, Stays string }
	_, statErr := os.Stat(entry.Checkout.Path)
	if code != 0 || json.Unmarshal([]byte(out), &closed) != nil || closed.State != "kept" || statErr != nil ||
		!strings.Contains(closed.Stays, "on no local branch but its own") {
		c.Contradicted(obsEnterKept, "space close answered %d: %s; checkout: %v", code, out, statErr)
		return
	}
	c.Observed(obsEnterKept, "the close kept the entered checkout with its unlanded commit: "+closed.Stays)
}

const (
	obsEnterMade = "a worker opened from a template with enter_worktree = true starts without a checkout and enters one with rewake worktree enter, which its status names"
	obsEnterKept = "main's space close keeps that entered checkout, which holds an unlanded commit, and says why"
)

var fixtureSpaceWorktreeEnterObservations = []string{obsEnterMade, obsEnterKept}
