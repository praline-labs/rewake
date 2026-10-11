/*
Package catalog is the one place that says which harnesses exist: New returns
the catalogue value, which cmd/rewake builds and hands to the CLI.

Adding a harness is two lines: import its package and add it to New.
*/
package catalog

import (
	"os"
	"path/filepath"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/adapter/claude"
)

// New returns the catalogue in guide order: the adapters of this build, the
// fixture's only in a build with its tag (testOnly).
func New() adapter.Catalog {
	return adapter.NewCatalog(foreign(), append([]adapter.Harness{claude.New()}, testOnly()...)...)
}

// foreign is where the harnesses this build has no adapter for keep their
// metadata: Codex in ~/.codex and, inside a project, .codex and .agents. They
// stay protected as long as a session of one may run on the machine; the
// Codex adapter of stage 5 declares them itself, and the list shrinks
// (decided October 11, 2026).
func foreign() adapter.Protected {
	protected := adapter.Protected{Names: []string{".codex", ".agents"}}
	if home, err := os.UserHomeDir(); err == nil {
		protected.Dirs = []string{filepath.Join(home, ".codex")}
	}
	return protected
}
