//go:build rewakefixture

package catalog

import (
	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/adapter/fixture"
)

// testOnly is the fixture: in the catalog only in a build with its tag, the
// workflow suite's, never a release's.
func testOnly() []adapter.Harness { return []adapter.Harness{fixture.New()} }
