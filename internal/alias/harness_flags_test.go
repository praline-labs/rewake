package alias

import (
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/adapter/catalog"
)

// The alias help promises that a typed flag replaces the alias's copy, and
// --worktree is one a line types beside an alias: on Claude Code the two once
// reached rewake together and the launch was refused as naming two worktrees.
// Checked against what each harness that takes a worktree publishes, not a
// stand-in.
func TestATypedWorktreeReplacesTheAliasOnEveryHarness(t *testing.T) {
	adapters := catalog.New()
	for _, h := range adapters.All() {
		if _, ok := h.(adapter.WorktreeHarness); !ok {
			continue
		}
		id := h.ID()
		t.Run(id, func(t *testing.T) {
			s := set(t, `
[alias.w]
harness = "`+id+`"
args = ["--worktree=from-alias"]
`)
			flags := func(name string) []adapter.Flag {
				found, _ := adapters.Find(name)
				return found.SingleUseFlags()
			}
			got, err := s.Expand([]string{"w", "--worktree=typed"}, adapters.IDs(), valued, nil, flags)
			if err != nil {
				t.Fatal(err)
			}
			joined := strings.Join(got, " ")
			if strings.Contains(joined, "from-alias") || count(got, "--worktree=typed") != 1 {
				t.Fatalf("the typed --worktree did not replace the alias's: %s", joined)
			}
		})
	}
}
