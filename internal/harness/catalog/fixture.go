//go:build rewakefixture

package catalog

import (
	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/harness/fixture"
)

// The fixture is in the catalog only in a build with its tag: the workflow
// suite's, never a release's.
func init() {
	harness.Register(fixture.New())
}
