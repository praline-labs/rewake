package codex

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/grant"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/role"
)

// linkedWorktree makes a checkout of the repository at common, the way git
// worktree add lays one out.
func linkedWorktree(t *testing.T, common, name string) string {
	t.Helper()
	checkout := filepath.Join(filepath.Dir(common), name)
	metadata := filepath.Join(common, "worktrees", name)
	if err := os.MkdirAll(metadata, 0o700); err != nil {
		t.Fatal(err)
	}
	populateGitMetadata(t, metadata, true)
	writeGitPointer(t, filepath.Join(metadata, "commondir"), "../..\n")
	writeGitPointer(t, filepath.Join(checkout, ".git"), "gitdir: "+metadata+"\n")
	return checkout
}

func sharedRepository(t *testing.T) string {
	t.Helper()
	common := filepath.Join(t.TempDir(), "main.git")
	if err := os.MkdirAll(common, 0o700); err != nil {
		t.Fatal(err)
	}
	populateGitMetadata(t, common, true)
	return common
}

// A message is granted once: the same id again, from a forged or replayed
// letter, is refused rather than journaled a second time.
func TestDirGrantIsNotGrantedTwice(t *testing.T) {
	workspace, lib := t.TempDir(), t.TempDir()
	server, captured := gitDeliveryFixture(t, role.Write,
		gitReadFixture{thread: gitThreadFixture(workspace, []string{workspace}, "idle")},
		gitReadFixture{thread: gitThreadFixture(workspace, []string{workspace, lib}, "idle")},
	)
	dirGrantSession(t, server)
	waiting(t, server, "once", true)
	message := inbox.Message{ID: "once", Kind: inbox.Task, GrantDirs: []string{lib}}
	if _, result := deliverDirs(t, server, captured, message); result.State != inbox.Delivered {
		t.Fatalf("first: %+v", result)
	}
	roots, result := deliverDirs(t, server, captured, message)
	if result.State != inbox.Failed || !strings.Contains(result.Detail, "granted once already") || roots != nil {
		t.Fatalf("again: roots=%q result=%+v", roots, result)
	}
}

// A directory inside one another task's grant lets the worker write is
// refused: the worker could swap it for a link before the harness resolves it.
func TestDirGrantInsideAnotherGrantIsRefused(t *testing.T) {
	workspace, lib := t.TempDir(), t.TempDir()
	inner := filepath.Join(lib, "pkg")
	if err := os.Mkdir(inner, 0o700); err != nil {
		t.Fatal(err)
	}
	server, captured := gitDeliveryFixture(t, role.Write,
		gitReadFixture{thread: gitThreadFixture(workspace, []string{workspace}, "idle")},
		gitReadFixture{thread: gitThreadFixture(workspace, []string{workspace, lib}, "idle")},
	)
	dirGrantSession(t, server)
	waiting(t, server, "outer", true)
	if _, result := deliverDirs(t, server, captured, inbox.Message{ID: "outer", Kind: inbox.Task, GrantDirs: []string{lib}}); result.State != inbox.Delivered {
		t.Fatalf("outer: %+v", result)
	}
	roots, result := deliverDirs(t, server, captured, inbox.Message{ID: "inner", Kind: inbox.Task, GrantDirs: []string{inner}})
	if result.State != inbox.Failed || !strings.Contains(result.Detail, "replaced by a link") || roots != nil {
		t.Fatalf("inner: roots=%q result=%+v", roots, result)
	}
}

// Past the most a run holds, a new grant is refused and every earlier one
// kept: an entry pushed out would be a grant nobody takes back.
func TestDirGrantPastTheLimitIsRefused(t *testing.T) {
	workspace, lib := t.TempDir(), t.TempDir()
	server, captured := gitDeliveryFixture(t, role.Write,
		gitReadFixture{thread: gitThreadFixture(workspace, []string{workspace}, "idle")},
	)
	dirGrantSession(t, server)
	held := make([]grant.Entry, 0, grant.MaxLive)
	for index := range grant.MaxLive {
		id := "held-" + strconv.Itoa(index)
		waiting(t, server, id, true)
		held = append(held, grant.Entry{Path: "/held/" + strconv.Itoa(index), Message: id, Outcome: grant.Granted})
	}
	server.grants = held
	roots, result := deliverDirs(t, server, captured, inbox.Message{ID: "more", Kind: inbox.Task, GrantDirs: []string{lib}})
	if result.State != inbox.Failed || !strings.Contains(result.Detail, "rewake keeps") || roots != nil {
		t.Fatalf("roots=%q result=%+v", roots, result)
	}
	if len(server.grantJournal()) != grant.MaxLive {
		t.Fatalf("the journal changed: %d entries", len(server.grantJournal()))
	}
}

// A checkout some root covers already still gets its .git with --grant-git:
// the harness keeps .git read-only inside every root, its own included.
func TestDirGrantWithGitOnAWritableCheckoutAddsItsMetadata(t *testing.T) {
	workspace, repo := t.TempDir(), gitRepository(t)
	metadata := filepath.Join(repo, ".git")
	server, captured := gitDeliveryFixture(t, role.Write,
		gitReadFixture{thread: gitThreadFixture(workspace, []string{workspace, repo}, "idle")},
	)
	dirGrantSession(t, server)
	waiting(t, server, "both", true)
	roots, result := deliverDirs(t, server, captured, inbox.Message{ID: "both", Kind: inbox.Task, GrantGit: true, GrantDirs: []string{repo}})
	if !slices.Equal(roots, []string{workspace, repo, metadata}) || !strings.Contains(result.Detail, "already writable: "+repo) {
		t.Fatalf("roots=%q result=%+v", roots, result)
	}
}

// --grant-git on a worktree of another repository would open the metadata
// every checkout of it shares, hooks included; a worktree of the session's own
// repository opens nothing it could not write before.
func TestDirGrantWithGitOnASharedRepository(t *testing.T) {
	t.Run("another repository", func(t *testing.T) {
		workspace := t.TempDir()
		checkout := linkedWorktree(t, sharedRepository(t), "feature")
		server, captured := gitDeliveryFixture(t, role.Write,
			gitReadFixture{thread: gitThreadFixture(workspace, []string{workspace}, "idle")},
		)
		dirGrantSession(t, server)
		roots, result := deliverDirs(t, server, captured, inbox.Message{ID: "shared", Kind: inbox.Task, GrantGit: true, GrantDirs: []string{checkout}})
		if result.State != inbox.Failed || !strings.Contains(result.Detail, "every checkout of it shares") || roots != nil {
			t.Fatalf("roots=%q result=%+v", roots, result)
		}
	})
	t.Run("its own repository", func(t *testing.T) {
		common := sharedRepository(t)
		workspace, checkout := linkedWorktree(t, common, "own"), linkedWorktree(t, common, "feature")
		server, captured := gitDeliveryFixture(t, role.Write,
			gitReadFixture{thread: gitThreadFixture(workspace, []string{workspace}, "idle")},
		)
		dirGrantSession(t, server)
		waiting(t, server, "own", true)
		roots, result := deliverDirs(t, server, captured, inbox.Message{ID: "own", Kind: inbox.Task, GrantGit: true, GrantDirs: []string{checkout}})
		if result.State != inbox.Delivered || !slices.Contains(roots, checkout) || !slices.Contains(roots, common) {
			t.Fatalf("roots=%q result=%+v", roots, result)
		}
	})
}
