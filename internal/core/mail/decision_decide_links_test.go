package mail

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// The notes a decision owes are frozen from the room's main, so a registry
// put behind a link — dangling, or to a directory elsewhere with no main in
// it — is unreadable, never a room with no main: nothing is decided and
// nothing written. A registry missing for real holds no main, and the
// decision is made.
func TestDecideTakesNoRegistryBehindALinkForNoMain(t *testing.T) {
	for _, dangling := range []bool{true, false} {
		name := "a link to a directory"
		if dangling {
			name = "a dangling link"
		}
		t.Run(name, func(t *testing.T) {
			lab := newUnknownLab(t)
			req := lab.ask(t, ChoiceArrived, DecidedBy{Kind: ByPerson})
			before := wholeTree(t, lab.dir)
			root := state.SessionsPath(lab.dir)
			saved := filepath.Join(t.TempDir(), "sessions")
			if err := os.Rename(root, saved); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(lab.dir, "missing-registry")
			if !dangling {
				target = t.TempDir()
			}
			if err := os.Symlink(target, root); err != nil {
				t.Fatal(err)
			}
			answer, err := Decide(context.Background(), lab.dir, req)
			if outcomeOf(err) != DecisionUnreadable || fileExists(DecisionPath(lab.dir, "api", lab.report.ID)) {
				t.Fatalf("a registry behind a link was taken for no main: %+v, %v", answer, err)
			}
			if err := os.Remove(root); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(saved, root); err != nil {
				t.Fatal(err)
			}
			if after := wholeTree(t, lab.dir); !sameTree(before, after) {
				t.Fatalf("a refused decision wrote: %v", treeDiff(before, after))
			}
		})
	}
	t.Run("missing for real", func(t *testing.T) {
		lab := newUnknownLab(t)
		req := lab.ask(t, ChoiceArrived, DecidedBy{Kind: ByPerson})
		if err := os.RemoveAll(state.SessionsPath(lab.dir)); err != nil {
			t.Fatal(err)
		}
		if answer, err := Decide(context.Background(), lab.dir, req); err != nil || answer.Decision.ID == "" {
			t.Fatalf("a room with no registry answered %+v, %v", answer, err)
		}
	})
}

// The stops a decision disposes of are listed as the mailbox's own: put
// behind a link to a copy of themselves, they are unreadable, never followed
// to records that are not the mailbox's.
func TestDecideListsNoStopsBehindALink(t *testing.T) {
	lab := newUnknownLab(t)
	req := lab.ask(t, ChoiceArrived, DecidedBy{Kind: ByPerson})
	stops := stopsPath(lab.dir, "api")
	copied := filepath.Join(t.TempDir(), "stops")
	if err := os.CopyFS(copied, os.DirFS(stops)); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(stops); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(copied, stops); err != nil {
		t.Fatal(err)
	}
	answer, err := Decide(context.Background(), lab.dir, req)
	if outcomeOf(err) != DecisionUnreadable || fileExists(DecisionPath(lab.dir, "api", lab.report.ID)) {
		t.Fatalf("stops behind a link answered %+v, %v", answer, err)
	}
	if err := os.Remove(stops); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(stops, os.DirFS(copied)); err != nil {
		t.Fatal(err)
	}
}
