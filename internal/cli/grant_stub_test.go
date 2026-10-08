package cli

import (
	"testing"

	"github.com/praline-labs/rewake/internal/harness"
)

// grantStub is a registered harness whose grant capabilities the test sets:
// no harness in the catalog takes a Git grant, and every one that issues
// grants reaches its wrapper, so the other half of each of those branches is
// reached only through a stub.
type grantStub struct {
	harness.Harness
	git, reaches bool
}

func (s grantStub) SupportsGitGrant() bool { return s.git }
func (s grantStub) ReachesWrapper() bool   { return s.reaches }

func (s grantStub) SupportsDirGrant() bool {
	taker, ok := s.Harness.(harness.DirGrantHarness)
	return ok && taker.SupportsDirGrant()
}

// stubGrants makes the harness id answer as a grantStub until the test ends.
func stubGrants(t *testing.T, id string, git, reaches bool) {
	t.Helper()
	saved := findHarness
	findHarness = func(asked string) (harness.Harness, bool) {
		found, ok := saved(asked)
		if ok && asked == id {
			return grantStub{Harness: found, git: git, reaches: reaches}, true
		}
		return found, ok
	}
	t.Cleanup(func() { findHarness = saved })
}
