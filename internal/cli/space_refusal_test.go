package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/registry"
)

// spacesStub is a harness whose spaces offer the test sets.
type spacesStub struct {
	adapter.Harness
	spaces bool
}

func (spacesStub) ID() string    { return "stub" }
func (spacesStub) Title() string { return "Stub harness" }

func (s spacesStub) Offers() adapter.Offer { return adapter.Offer{Spaces: s.spaces} }

// Opening a space is refused by the level that is missing, each named: the
// role, the harness's offer, the run's live set. Only all three let it go on.
func TestASpaceIsRefusedByTheMissingCapability(t *testing.T) {
	offering := adapter.NewCatalog(spacesStub{spaces: true})
	lacking := adapter.NewCatalog(spacesStub{})
	live := []string{string(adapter.WakeCapability), string(adapter.SpacesCapability)}
	for _, test := range []struct {
		name    string
		catalog adapter.Catalog
		session registry.Session
		usage   bool
		says    string
	}{
		{"a worker", offering, registry.Session{Name: "api", Role: "write", Harness: "stub", Live: live}, true, "only main opens a space"},
		{"no offer", lacking, registry.Session{Name: "api", Role: "main", Harness: "stub", Live: live}, true, "Stub harness session, which hosts no space"},
		{"another build's harness", offering, registry.Session{Name: "api", Role: "main", Harness: "gone", Live: live}, true, "gone session, which hosts no space"},
		{"not live", offering, registry.Session{Name: "api", Role: "main", Harness: "stub", Live: live[:1]}, false, "api's spaces are not live"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := spacesRefusal(nil, test.catalog, test.session)
			var wrong *UsageError
			var failed *FailedError
			switch {
			case err == nil:
				t.Fatal("not refused")
			case test.usage && !errors.As(err, &wrong), !test.usage && !errors.As(err, &failed):
				t.Fatalf("refused as %T: %v", err, err)
			case !strings.Contains(err.Error(), test.says):
				t.Fatalf("refused with %q, want it to say %q", err, test.says)
			}
		})
	}
	ready := registry.Session{Name: "api", Role: "main", Harness: "stub", Live: live}
	if err := spacesRefusal(nil, offering, ready); err != nil {
		t.Fatalf("a main run with its spaces live was refused: %v", err)
	}
}
