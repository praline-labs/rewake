package cli

import (
	"testing"

	"github.com/praline-labs/rewake/internal/harness"
)

// aHarness is the id of a harness in the catalog, for a test that needs one
// to launch or parse and does not care which: the core's tests name none.
func aHarness(t testing.TB) string {
	t.Helper()
	all := harness.All()
	if len(all) == 0 {
		t.Fatal("the catalog holds no harness")
	}
	return all[0].ID()
}

// aWorktreeProbe is a probe of the first harness in the catalog that takes
// rewake's worktree flag.
func aWorktreeProbe(t *testing.T) *worktreeProbe {
	t.Helper()
	for _, candidate := range harness.All() {
		if _, ok := candidate.(harness.WorktreeHarness); ok {
			return harnessProbe(t, candidate.ID())
		}
	}
	t.Fatal("no harness in the catalog takes a worktree")
	return nil
}
