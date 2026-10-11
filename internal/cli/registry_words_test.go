package cli

import (
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
)

// otherAgent is a second adapter beside the test agent, offering what it does.
type otherAgent struct{ agentHarness }

func (otherAgent) ID() string    { return "other" }
func (otherAgent) Title() string { return "Other agent" }

// A clause about the harnesses that lack a capability agrees with how many
// they are: one's phrase after one, many's after several, none when all offer
// it.
func TestADecliningClauseAgreesWithHowManyDecline(t *testing.T) {
	lacksGit := func(o adapter.Offer) bool { return o.Permissions.Git }
	for _, test := range []struct {
		catalog adapter.Catalog
		want    string
	}{
		{adapter.NewCatalog(adapter.Protected{}, agentHarness{}), " Test agent takes none."},
		{adapter.NewCatalog(adapter.Protected{}, agentHarness{}, otherAgent{}), " Test agent and Other agent take none."},
		{adapter.NewCatalog(adapter.Protected{}), ""},
	} {
		if got := declining(test.catalog, lacksGit, "takes none", "take none"); got != test.want {
			t.Fatalf("%q, want %q", got, test.want)
		}
	}
}
