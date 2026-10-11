package cli

import (
	"slices"
	"strings"

	"github.com/praline-labs/rewake/internal/adapter"
)

// The harness words a help text needs come from the catalog it was built
// from, so the text names what this build carries and the CLI names no
// harness of its own (docs/v2/design.md#the-test-that-holds-it, rule 2).

// declining names the harnesses whose offer lacks what has asks for, as a
// clause ending a sentence about that capability, or nothing when every
// harness of the catalog offers it.
func declining(catalog adapter.Catalog, has func(adapter.Offer) bool, verb string) string {
	var titles []string
	for _, h := range catalog.All() {
		if !has(h.Offers()) {
			titles = append(titles, h.Title())
		}
	}
	if len(titles) == 0 {
		return ""
	}
	if len(titles) > 1 {
		verb = strings.TrimSuffix(verb, "s")
	}
	return " " + inWords(titles, "and") + " " + verb + "."
}

// metadataNames lists the directory names a grant refuses anywhere in a path:
// Git's, and each harness's the catalog protects.
func metadataNames(catalog adapter.Catalog) string {
	return inWords(append([]string{".git"}, slices.Sorted(slices.Values(catalog.Protected().Names))...), "or")
}

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
