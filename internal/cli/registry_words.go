package cli

import (
	"github.com/praline-labs/rewake/internal/adapter"
)

// The harness words a help text needs come from the catalog it was built
// from, so the text names what this build carries and the CLI names no
// harness of its own (docs/rules/layout.md).

// launchExample is a launch of the catalog's first harness, for a text that
// shows how a session starts; a catalog without one shows the placeholder.
func launchExample(catalog adapter.Catalog) string {
	if all := catalog.All(); len(all) > 0 {
		return all[0].ID()
	}
	return "<harness>"
}
