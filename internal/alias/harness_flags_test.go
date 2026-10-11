package alias

import (
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
)

// The alias help promises that a typed flag replaces the alias's copy, and
// --worktree is one a line types beside an alias: on one harness the two once
// reached rewake together and the launch was refused as naming two worktrees.
// A harness that takes rewake's worktree lists the flag as a switch it takes
// once, which the catalog's test holds for every adapter
// (TestEveryWorktreeFlagIsTakenOnce); given that, the typed one wins.
func TestATypedWorktreeReplacesTheAlias(t *testing.T) {
	s := set(t, `
[alias.w]
harness = "agent"
args = ["--worktree=from-alias"]
`)
	flags := func(string) []adapter.Flag {
		return []adapter.Flag{{Spellings: []string{"--model"}, TakesValue: true}, {Spellings: []string{"--worktree"}}}
	}
	got, err := s.Expand([]string{"w", "--worktree=typed"}, []string{"agent"}, valued, nil, flags)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(got, " ")
	if strings.Contains(joined, "from-alias") || count(got, "--worktree=typed") != 1 {
		t.Fatalf("the typed --worktree did not replace the alias's: %s", joined)
	}
}
