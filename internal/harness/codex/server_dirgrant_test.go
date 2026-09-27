package codex

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/grant"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/role"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// dirGrantSession gives the fixture's session a mailbox of its own, where its
// journal is kept and where a task's copy says whether it is still ahead.
func dirGrantSession(t *testing.T, server *serverSession) {
	t.Helper()
	server.mailbox, server.name, server.epoch = t.TempDir(), "writer", "e1"
	if err := state.EnsureSubdir(state.InboxPath(server.mailbox, server.name)); err != nil {
		t.Fatal(err)
	}
}

// waiting keeps a task ahead of its reader, or with false lets it be settled.
func waiting(t *testing.T, server *serverSession, id string, ahead bool) {
	t.Helper()
	path := filepath.Join(state.InboxPath(server.mailbox, server.name), id+".json")
	if !ahead {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		return
	}
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func deliverDirs(t *testing.T, server *serverSession, captured <-chan map[string]json.RawMessage, message inbox.Message) ([]string, inbox.Result) {
	t.Helper()
	if message.Text == "" {
		message.Text = "do the work"
	}
	result := server.Deliver(context.Background(), message)
	select {
	case params := <-captured:
		raw, exists := params["runtimeWorkspaceRoots"]
		if !exists {
			return nil, result
		}
		var roots []string
		if err := json.Unmarshal(raw, &roots); err != nil || roots == nil {
			t.Fatalf("invalid roots: %s", raw)
		}
		return roots, result
	default:
		return nil, result
	}
}

func journal(server *serverSession) []grant.Entry {
	return grant.Load(server.mailbox, server.name, server.epoch)
}

func outcomes(entries []grant.Entry) []string {
	var got []string
	for _, entry := range entries {
		got = append(got, entry.Path+"="+entry.Outcome)
	}
	return got
}

// A grant reaches the roots with the task, is journaled, and is taken back at
// the first delivery after its task is reported on; a person's turn that
// dropped it in between leaves it recorded as dropped rather than revoked.
func TestDirGrantIsAddedJournaledAndTakenBack(t *testing.T) {
	workspace, lib, other := t.TempDir(), t.TempDir(), t.TempDir()
	existing := []string{workspace}
	granted := []string{workspace, lib}
	server, captured := gitDeliveryFixture(t, role.General,
		gitReadFixture{thread: gitThreadFixture(workspace, existing, "idle")},
		gitReadFixture{thread: gitThreadFixture(workspace, granted, "idle")},
		gitReadFixture{thread: gitThreadFixture(workspace, existing, "idle")},
		gitReadFixture{thread: gitThreadFixture(workspace, existing, "idle")},
	)
	dirGrantSession(t, server)

	waiting(t, server, "grant-1", true)
	roots, result := deliverDirs(t, server, captured, inbox.Message{ID: "grant-1", Kind: inbox.Task, GrantDirs: []string{lib}})
	if result.State != inbox.Delivered || !slices.Equal(roots, granted) || !slices.Equal(result.GrantApplied, []string{lib}) || !strings.Contains(result.Detail, "write granted: "+lib) {
		t.Fatalf("grant: roots=%q result=%+v", roots, result)
	}
	if got := outcomes(journal(server)); !slices.Equal(got, []string{lib + "=granted"}) {
		t.Fatalf("journal = %v", got)
	}

	// While the task is ahead of its reader, a plain notice does not read
	// the roots and leaves them alone.
	roots, result = deliverDirs(t, server, captured, inbox.Message{ID: "plain-1", Kind: inbox.Note})
	if result.State != inbox.Delivered || roots != nil || result.Detail != "" {
		t.Fatalf("an unsettled grant was touched: roots=%q result=%+v", roots, result)
	}

	// Reported on: the next notice takes the directory out.
	waiting(t, server, "grant-1", false)
	roots, result = deliverDirs(t, server, captured, inbox.Message{ID: "plain-2", Kind: inbox.Note})
	if !slices.Equal(roots, existing) || !strings.Contains(result.Detail, "taken back, their tasks reported on: "+lib) {
		t.Fatalf("revoke: roots=%q result=%+v", roots, result)
	}
	entries := journal(server)
	if got := outcomes(entries); !slices.Equal(got, []string{lib + "=revoked"}) || entries[0].EndedAt == nil {
		t.Fatalf("journal = %v", got)
	}

	// Nothing live: no read, no roots.
	roots, result = deliverDirs(t, server, captured, inbox.Message{ID: "plain-3", Kind: inbox.Note})
	if roots != nil || result.Detail != "" {
		t.Fatalf("an ended grant was touched: roots=%q result=%+v", roots, result)
	}

	// Granted again, then dropped by a person's turn before the report.
	waiting(t, server, "grant-2", true)
	if roots, _ = deliverDirs(t, server, captured, inbox.Message{ID: "grant-2", Kind: inbox.Question, GrantDirs: []string{other}}); !slices.Equal(roots, []string{workspace, other}) {
		t.Fatalf("second grant roots=%q", roots)
	}
	waiting(t, server, "grant-2", false)
	roots, result = deliverDirs(t, server, captured, inbox.Message{ID: "plain-4", Kind: inbox.Note})
	if roots != nil || strings.Contains(result.Detail, "taken back") {
		t.Fatalf("dropped grant: roots=%q result=%+v", roots, result)
	}
	if got := outcomes(journal(server)); !slices.Equal(got, []string{lib + "=revoked", other + "=dropped"}) {
		t.Fatalf("journal = %v", got)
	}
}

// A directory a root covers already is not journaled — rewake did not give
// it, so it is not rewake's to take back — while one inside a root rewake
// granted for another task is journaled for this one as well.
func TestDirGrantAlreadyWritableAndSharedRoots(t *testing.T) {
	workspace, lib := t.TempDir(), t.TempDir()
	inside := filepath.Join(workspace, "sub")
	server, captured := gitDeliveryFixture(t, role.Write,
		gitReadFixture{thread: gitThreadFixture(workspace, []string{workspace}, "idle")},
		gitReadFixture{thread: gitThreadFixture(workspace, []string{workspace}, "idle")},
		gitReadFixture{thread: gitThreadFixture(workspace, []string{workspace, lib}, "idle")},
		gitReadFixture{thread: gitThreadFixture(workspace, []string{workspace, lib}, "idle")},
		gitReadFixture{thread: gitThreadFixture(workspace, []string{workspace, lib}, "idle")},
	)
	dirGrantSession(t, server)

	roots, result := deliverDirs(t, server, captured, inbox.Message{ID: "covered", Kind: inbox.Task, GrantDirs: []string{inside}})
	if roots != nil || !slices.Equal(result.GrantApplied, []string{inside}) || !strings.Contains(result.Detail, "already writable: "+inside) || journal(server) != nil {
		t.Fatalf("covered: roots=%q result=%+v journal=%v", roots, result, journal(server))
	}

	waiting(t, server, "first", true)
	waiting(t, server, "second", true)
	if roots, _ = deliverDirs(t, server, captured, inbox.Message{ID: "first", Kind: inbox.Task, GrantDirs: []string{lib}}); !slices.Equal(roots, []string{workspace, lib}) {
		t.Fatalf("first roots=%q", roots)
	}
	roots, result = deliverDirs(t, server, captured, inbox.Message{ID: "second", Kind: inbox.Task, GrantDirs: []string{lib}})
	if roots != nil || !strings.Contains(result.Detail, "write granted: "+lib) {
		t.Fatalf("second: roots=%q result=%+v", roots, result)
	}

	// The first task reported on: the second still holds the directory.
	waiting(t, server, "first", false)
	if roots, result = deliverDirs(t, server, captured, inbox.Message{ID: "plain-1", Kind: inbox.Note}); roots != nil || result.Detail != "" {
		t.Fatalf("a held directory was taken back: roots=%q result=%+v", roots, result)
	}
	waiting(t, server, "second", false)
	roots, _ = deliverDirs(t, server, captured, inbox.Message{ID: "plain-2", Kind: inbox.Note})
	if !slices.Equal(roots, []string{workspace}) {
		t.Fatalf("roots=%q", roots)
	}
	if got := outcomes(journal(server)); !slices.Equal(got, []string{lib + "=revoked", lib + "=revoked"}) {
		t.Fatalf("journal = %v", got)
	}
}

// With --grant-git beside it, a granted checkout's metadata is added too, and
// journaled for that directory.
func TestDirGrantWithGitAddsTheCheckoutsMetadata(t *testing.T) {
	workspace, repo := t.TempDir(), gitRepository(t)
	metadata := filepath.Join(repo, ".git")
	server, captured := gitDeliveryFixture(t, role.Write,
		gitReadFixture{thread: gitThreadFixture(workspace, []string{workspace}, "idle")},
	)
	dirGrantSession(t, server)
	waiting(t, server, "both", true)
	roots, result := deliverDirs(t, server, captured, inbox.Message{ID: "both", Kind: inbox.Task, GrantGit: true, GrantDirs: []string{repo}})
	if !slices.Equal(roots, []string{workspace, repo, metadata}) {
		t.Fatalf("roots=%q result=%+v", roots, result)
	}
	entries := journal(server)
	if len(entries) != 2 || entries[1].Path != metadata || entries[1].For != repo {
		t.Fatalf("journal = %+v", entries)
	}
}

// A grant does not go into a running turn, and does not go at all without a
// snapshot to add it to: a task is never handed over without its grant.
func TestDirGrantWaitsForIdleAndFailsWithoutRoots(t *testing.T) {
	workspace, lib := t.TempDir(), t.TempDir()
	server, captured := gitDeliveryFixture(t, role.General,
		gitReadFixture{thread: gitThreadFixture(workspace, []string{workspace}, "active")},
		gitReadFixture{refuse: true},
	)
	dirGrantSession(t, server)
	message := inbox.Message{ID: "grant", Kind: inbox.Task, GrantDirs: []string{lib}}
	roots, result := deliverDirs(t, server, captured, message)
	if result.State != inbox.Pending || !strings.Contains(result.Detail, idleWait) || roots != nil {
		t.Fatalf("active: roots=%q result=%+v", roots, result)
	}
	roots, result = deliverDirs(t, server, captured, message)
	if result.State != inbox.Failed || !strings.Contains(result.Detail, "directory grant could not be applied") || roots != nil {
		t.Fatalf("unreadable: roots=%q result=%+v", roots, result)
	}
	if journal(server) != nil {
		t.Fatal("a failed grant was journaled")
	}
}

func TestDirGrantRefusedOnAHeadsUpOrInABatch(t *testing.T) {
	lib := t.TempDir()
	for name, message := range map[string]inbox.Message{
		"heads-up": {ID: "n", Kind: inbox.Note, GrantDirs: []string{lib}},
		"batch": {ID: "b", Kind: inbox.Task, Batch: []inbox.Message{
			{ID: "b", Kind: inbox.Task, GrantDirs: []string{lib}},
			{ID: "c", Kind: inbox.Task},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			// A batch member carrying a grant reads the thread before it
			// is refused; a heads-up is refused without one.
			var reads []gitReadFixture
			if name == "batch" {
				reads = append(reads, gitReadFixture{thread: gitThreadFixture(lib, []string{lib}, "idle")})
			}
			server, captured := gitDeliveryFixture(t, role.General, reads...)
			dirGrantSession(t, server)
			roots, result := deliverDirs(t, server, captured, message)
			if result.State != inbox.Failed || !strings.Contains(result.Detail, "announced on its own") || roots != nil {
				t.Fatalf("roots=%q result=%+v", roots, result)
			}
		})
	}
}
