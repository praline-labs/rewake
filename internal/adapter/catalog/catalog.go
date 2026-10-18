/*
Package catalog is the one place that says which harnesses exist: New returns
the catalog value, which cmd/rewake builds and hands to the CLI.

Adding a harness is two lines: import its package and add it to New.
*/
package catalog

import (
	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/adapter/claude"
	"github.com/praline-labs/rewake/internal/adapter/pi"
)

// New returns the catalog in guide order: the adapters of this build, the
// fixture's only in a build with its tag (testOnly).
func New() adapter.Catalog {
	return adapter.NewCatalog(append([]adapter.Harness{claude.New(), pi.New()}, testOnly()...)...)
}
