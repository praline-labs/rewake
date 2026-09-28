/*
Package catalog is the one place that says which harnesses exist.

Import it for side effects wherever the catalog must be populated:

	import _ "github.com/praline-labs/rewake/internal/harness/catalog"

Adding a harness is two lines: import its package and register it below.
*/
package catalog

import (
	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/harness/claude"
	"github.com/praline-labs/rewake/internal/harness/codex"
)

func init() {
	harness.Register(claude.New())
	harness.Register(codex.New())
}
