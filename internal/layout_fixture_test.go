//go:build rewakefixture

package internal

import "github.com/praline-labs/rewake/internal/harness/fixture"

// The fixture is in the catalog only under its tag, and only for the workflow
// suite: rule 2 looks for it as nameWords says of a test-only harness.
func init() { testOnlyHarnesses = append(testOnlyHarnesses, fixture.ID) }
