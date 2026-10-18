package catalog

import (
	"slices"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
)

// A registered harness has to say which of its flags it takes at most once.
//
// The compiler makes the method impossible to forget, but the cheapest way to
// satisfy it is to return nothing — and nothing does not mean "no opinion", it
// means "every flag may be repeated". On a harness that refuses repeats that
// turns an alias plus a typed flag into a launch that will not parse, which is
// exactly the defect this list exists to prevent. So the list being non-empty
// is checked here rather than left to whoever adds the next harness.
func TestEveryHarnessNamesItsSingleUseFlags(t *testing.T) {
	for _, h := range New().All() {
		t.Run(h.ID(), func(t *testing.T) {
			flags := h.SingleUseFlags()
			if len(flags) == 0 {
				t.Fatal("no single-use flags: an empty list says every flag may be repeated")
			}
			seen := map[string]bool{}
			for _, flag := range flags {
				if len(flag.Spellings) == 0 {
					t.Fatal("a flag with no spellings")
				}
				for _, spelling := range flag.Spellings {
					if !strings.HasPrefix(spelling, "-") {
						t.Fatalf("%q is not a flag", spelling)
					}
					if seen[spelling] {
						t.Fatalf("%q is listed twice; one spelling belongs to one parameter", spelling)
					}
					seen[spelling] = true
				}
			}
		})
	}
}

// An adapter that takes rewake's worktree lists the flag as a switch it takes
// once: rewake reads it only with =<name>, and a typed one then replaces an
// alias's instead of reaching the launch beside it (alias's
// TestATypedWorktreeReplacesTheAlias).
func TestEveryWorktreeFlagIsTakenOnce(t *testing.T) {
	for _, h := range New().All() {
		taker, ok := h.(adapter.WorktreeHarness)
		if !ok {
			continue
		}
		t.Run(h.ID(), func(t *testing.T) {
			for _, flag := range h.SingleUseFlags() {
				if slices.Contains(flag.Spellings, taker.WorktreeFlag()) {
					if flag.TakesValue {
						t.Fatalf("%s is listed as taking a value; rewake reads it as a switch", taker.WorktreeFlag())
					}
					return
				}
			}
			t.Fatalf("%s is not among the flags taken once", taker.WorktreeFlag())
		})
	}
}
