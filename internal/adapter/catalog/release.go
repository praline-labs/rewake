//go:build !rewakefixture

package catalog

import "github.com/praline-labs/rewake/internal/adapter"

// testOnly is nothing in a build without the fixture's tag: a release.
func testOnly() []adapter.Harness { return nil }
