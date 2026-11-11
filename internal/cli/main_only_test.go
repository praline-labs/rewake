package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/core/space"
	"github.com/praline-labs/rewake/internal/infra/state"
)

type mainOnlyHarness struct{ workerSettingsHarness }

func (mainOnlyHarness) ID() string    { return "main-agent" }
func (mainOnlyHarness) Title() string { return "Main agent" }
func (h mainOnlyHarness) Offers() adapter.Offer {
	offer := h.workerSettingsHarness.Offers()
	offer.MainOnly = true
	return offer
}

// The central role check runs before shell entry, a room lease or a harness
// invocation, even when a template's command wrapper supplied the program.
func TestAMainOnlyHarnessRefusesWorkersThroughWrappers(t *testing.T) {
	isolateAliases(t)
	saved := identifyLaunch
	t.Cleanup(func() { identifyLaunch = saved })
	for _, part := range []role.Role{role.General, role.Write} {
		identifyLaunch = func(Call) (launchIdentity, error) {
			return launchIdentity{room: "workers", part: part}, nil
		}
		for _, prefix := range [][]string{nil, {"--command", "sh"}} {
			base := filepath.Join(t.TempDir(), "never-created")
			t.Setenv(state.DirEnv, base)
			code, _, errOut := runOn(adapter.NewCatalog(mainOnlyHarness{}), append(prefix, "main-agent")...)
			if code != ExitFailed || !strings.Contains(errOut, "Main agent launches main only") {
				t.Fatalf("%s %v: %d %s", part.ID, prefix, code, errOut)
			}
			if _, err := os.Stat(base); !os.IsNotExist(err) {
				t.Fatalf("refused launch touched state: %v", err)
			}
		}
	}
}

const mainOnlyAlias = `[alias.old-writer]
harness = "main-agent"
role = "write"
command = "old-worker-wrapper"
`

func TestWorkersOmitsMainOnlyTemplatesAndCountsOnlyLaunchableWriters(t *testing.T) {
	catalog := adapter.NewCatalog(mainOnlyHarness{}, workerSettingsHarness{})
	for _, rest := range []string{"", "[alias.helper]\nharness = \"agent\"\n", workerAliasesExample} {
		workerAliases(t, mainOnlyAlias+rest)
		code, out, errOut := runOn(catalog, "workers", "--json")
		var view workersListing
		if code != ExitOK || json.Unmarshal([]byte(out), &view) != nil {
			t.Fatalf("workers: %d %s %s", code, out, errOut)
		}
		for _, worker := range view.Workers {
			if worker.Name == "old-writer" {
				t.Fatal("a main-only template was listed")
			}
		}
		if view.NoWriter != (rest != workerAliasesExample) {
			t.Fatalf("noWriter counted a non-launchable writer: %+v", view)
		}
		if rest != "" && len(view.Workers) == 0 {
			t.Fatal("main-only template hid worker-capable templates")
		}
		code, out, errOut = runOn(catalog, "workers")
		if code != ExitOK || strings.Contains(out, "old-writer") || (rest == "" && !strings.Contains(out, "No launchable worker templates")) {
			t.Fatalf("workers lines: %d %s %s", code, out, errOut)
		}
	}
}

func TestAMainOnlyTemplateIsRefusedBeforeSpaceCreation(t *testing.T) {
	workerAliases(t, mainOnlyAlias)
	dir := liveSessionAs(t, "main", role.Main.ID)
	self, err := registry.Lookup(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(state.SessionEnv, self.Name)
	t.Setenv(state.EpochEnv, self.Epoch())
	catalog := adapter.NewCatalog(mainOnlyHarness{}, spacesStub{Harness: workerSettingsHarness{}, spaces: true})
	self.Harness = "stub"
	self.Live = append(self.Live, string(adapter.SpacesCapability))
	// The main's offer and live spaces are independent of the worker's offer.
	if err := registry.Update(dir, self); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := runOn(catalog, "space", "open", "old", "--alias", "old-writer")
	if code != ExitUsage || !strings.Contains(errOut, `alias "old-writer"`) || !strings.Contains(errOut, "launches main only") {
		t.Fatalf("space open: %d %s", code, errOut)
	}
	records, problems, err := space.List(dir)
	if err != nil || len(records) != 0 || len(problems) != 0 {
		t.Fatalf("refusal created a space: %v %v %v", records, problems, err)
	}
}
