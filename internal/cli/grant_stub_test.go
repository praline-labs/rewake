package cli

import (
	"github.com/praline-labs/rewake/internal/adapter"
)

// grantStub is a registered harness whose grant capabilities the test sets:
// no harness in the catalog takes a Git grant, none of an untagged build takes
// a directory, and every one that issues grants reaches its wrapper, so the
// other half of each of those branches is reached only through a stub.
type grantStub struct {
	adapter.Harness
	git, reaches, dirs bool
}

func (s grantStub) SupportsGitGrant() bool { return s.git }
func (s grantStub) ReachesWrapper() bool   { return s.reaches }

func (s grantStub) SupportsDirGrant() bool {
	taker, ok := s.Harness.(adapter.DirGrantHarness)
	return s.dirs || ok && taker.SupportsDirGrant()
}

// grantCatalog is base with the harness id answering as a grantStub.
func grantCatalog(base adapter.Catalog, id string, git, reaches bool) adapter.Catalog {
	return remap(base, func(found adapter.Harness) adapter.Harness {
		if found.ID() == id {
			return grantStub{Harness: found, git: git, reaches: reaches}
		}
		return found
	})
}
