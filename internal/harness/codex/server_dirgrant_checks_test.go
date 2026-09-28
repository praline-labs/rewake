package codex

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/grant"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/role"
)

// A thread whose roots are not one local list cannot take a grant: a task is
// never handed over without the grant it was sent with. Git metadata alone
// only goes without, as it always may.
func TestAGrantIntoAThreadWithoutLocalRootsFailsItsTask(t *testing.T) {
	workspace, lib := t.TempDir(), t.TempDir()
	remote := map[string]any{
		"id": fixtureRoot, "status": map[string]string{"type": "idle"},
		"environments": []threadEnvironment{{ID: "remote", Cwd: workspace, Roots: []string{workspace}}},
	}
	server, captured := gitDeliveryFixture(t, role.General, gitReadFixture{thread: remote})
	dirGrantSession(t, server)
	roots, result := deliverDirs(t, server, captured, inbox.Message{ID: "grant", Kind: inbox.Task, GrantDirs: []string{lib}})
	if result.State == inbox.Delivered || !strings.Contains(result.Detail, "its directory grant could not be applied") || roots != nil {
		t.Fatalf("roots=%q result=%+v", roots, result)
	}
	if journal(server) != nil {
		t.Fatal("a grant not applied was journaled")
	}
}

// Git metadata outside the checkout is judged by the hard tier on its own:
// a gitfile pointing into rewake's state directory gets the checkout its
// grant and not the metadata, though the metadata is what --grant-git names.
func TestGitMetadataInTheHardTierIsNotGranted(t *testing.T) {
	workspace, repo := t.TempDir(), t.TempDir()
	stateRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	metadata := filepath.Join(stateRoot, "checkout.git")
	if err := os.MkdirAll(metadata, 0o700); err != nil {
		t.Fatal(err)
	}
	populateGitMetadata(t, metadata, true)
	writeGitPointer(t, filepath.Join(repo, ".git"), "gitdir: "+metadata+"\n")
	server, captured := gitDeliveryFixture(t, role.Write,
		gitReadFixture{thread: gitThreadFixture(workspace, []string{workspace}, "idle")},
	)
	dirGrantSession(t, server)
	server.stateRoot = stateRoot
	waiting(t, server, "both", true)
	roots, result := deliverDirs(t, server, captured, inbox.Message{ID: "both", Kind: inbox.Task, GrantGit: true, GrantDirs: []string{repo}})
	if result.State != inbox.Delivered || !slices.Equal(roots, []string{workspace, repo}) {
		t.Fatalf("roots=%q result=%+v", roots, result)
	}
	if !strings.Contains(result.Detail, "no Git metadata granted for "+repo) || !strings.Contains(result.Detail, "rewake's state directory") {
		t.Fatalf("the detail does not say why: %s", result.Detail)
	}
	if got := outcomes(journal(server)); !slices.Equal(got, []string{repo + "=granted"}) {
		t.Fatalf("journal = %v", got)
	}
}

// A grant this run journaled itself is not asked for again as if a resume
// had left it: asked again, a grant the answer would not match loses its
// root, and this run's own grant would be taken out from under its task.
func TestAResumeLooksOnlyAtGrantsThisRunDidNotJournal(t *testing.T) {
	server := &serverSession{mailbox: t.TempDir(), name: "writer", epoch: "e1"}
	main := runOf(t, os.Getpid())
	lib, doc := t.TempDir(), t.TempDir()
	copied := []grant.Entry{
		{Path: lib, Message: "m1", Outcome: grant.Granted, Thread: fixtureRoot, From: "lead", FromEpoch: main},
		{Path: doc, Message: "m2", Outcome: grant.Granted, Thread: fixtureRoot, From: "lead", FromEpoch: main},
	}
	if err := grant.Save(server.mailbox, server.name, endedRunOf(t), copied); err != nil {
		t.Fatal(err)
	}
	own := []grant.Entry{copied[0]}
	hints := server.resumedHints(fixtureRoot, own)
	if len(hints) != 1 || hints[0].Message != "m2" {
		t.Fatalf("hints = %+v", hints)
	}
	server.follow(fixtureRoot)
	if hints := server.resumedHints(fixtureRoot, nil); hints != nil {
		t.Fatalf("a conversation looked at once is looked at again: %+v", hints)
	}
}
