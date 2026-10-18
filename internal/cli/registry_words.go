package cli

import (
	"strings"

	"github.com/praline-labs/rewake/internal/adapter"
)

// The harness words a help text needs come from the catalog it was built
// from, so the text names what this build carries and the CLI names no
// harness of its own (docs/v2/design.md#the-test-that-holds-it, rule 2).

// launchExample is a launch of the catalog's first harness, for a text that
// shows how a session starts; a catalog without one shows the placeholder.
func launchExample(catalog adapter.Catalog) string {
	if all := catalog.All(); len(all) > 0 {
		return all[0].ID()
	}
	return "<harness>"
}

func inWords(words []string, conjunction string) string {
	if len(words) < 2 {
		return strings.Join(words, "")
	}
	return strings.Join(words[:len(words)-1], ", ") + " " + conjunction + " " + words[len(words)-1]
}
