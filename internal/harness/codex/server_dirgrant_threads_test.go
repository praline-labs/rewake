package codex

import (
	"slices"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/grant"
	"github.com/praline-labs/rewake/internal/inbox"
)

// readOf is a read of a conversation's one local environment.
func readOf(thread, cwd string, roots ...string) func() threadRead {
	return func() threadRead {
		return threadRead{id: thread, status: "idle", environments: []threadEnvironment{{ID: "local", Cwd: cwd, Roots: roots}}}
	}
}

// Roots belong to a conversation. A grant made in one and settled is taken
// back at the next notice into it, not at a notice into another: there it
// would be settled against roots it was never in, and would take out a root
// that conversation has of its own, or be recorded as dropped while its own
// conversation keeps it for good.
func TestASettledGrantIsTakenBackOnlyInItsConversation(t *testing.T) {
	workspace, lib := t.TempDir(), t.TempDir()
	server := &serverSession{}
	dirGrantSession(t, server)
	server.grants = []grant.Entry{{Path: lib, Message: "m1", Outcome: grant.Granted, Thread: "thread-a"}}

	for _, roots := range [][]string{{workspace, lib}, {workspace}} {
		change, err := server.planRoots("thread-b", readOf("thread-b", workspace, roots...), false, inbox.Message{})
		if err != nil || change.roots != nil || change.journal != nil || len(change.notes) != 0 {
			t.Fatalf("a notice into another conversation with roots %q: %+v %v", roots, change, err)
		}
	}

	change, err := server.planRoots("thread-a", readOf("thread-a", workspace, workspace, lib), false, inbox.Message{})
	if err != nil || !slices.Equal(change.roots, []string{workspace}) || !slices.Equal(outcomes(change.journal), []string{lib + "=revoked"}) {
		t.Fatalf("a notice into its own conversation: %+v %v", change, err)
	}
}

// A directory held in one conversation by a live grant is no grant of
// rewake's in another. There a root the person gave covers it, so a task
// granted it there finds it writable and journals nothing, and its report
// does not take out the person's root.
func TestAGrantInAnotherConversationDoesNotOwnARoot(t *testing.T) {
	workspace, lib := t.TempDir(), t.TempDir()
	server := &serverSession{}
	dirGrantSession(t, server)
	waiting(t, server, "m1", true)
	server.grants = []grant.Entry{{Path: lib, Message: "m1", Outcome: grant.Granted, Thread: "thread-a"}}

	granted := inbox.Message{ID: "m2", Kind: inbox.Task, From: "lead", GrantDirs: []string{lib}}
	change, err := server.planRoots("thread-b", readOf("thread-b", workspace, workspace, lib), false, granted)
	if err != nil || change.roots != nil || len(change.applied) != 1 || !strings.Contains(strings.Join(change.notes, "; "), "already writable: "+lib) {
		t.Fatalf("a root the person gave: %+v %v", change, err)
	}
	if change.journal != nil {
		t.Fatalf("journaled a root rewake did not give: %v", outcomes(change.journal))
	}
}
