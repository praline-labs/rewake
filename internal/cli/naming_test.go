package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
)

// A worker the flow raises is the address its next step sends to.
func TestLaunchFlowUsesTheAssembledAddress(t *testing.T) {
	steps := flow(testCatalog())
	for i, step := range steps {
		parsed, err := testTable().parse(splitExample(step.Command)[1:])
		if err != nil || parsed.Call.Command == nil || parsed.Call.Command.Name != "space" || len(parsed.Call.Positionals) < 2 {
			continue
		}
		address := parsed.Call.Positionals[1] + "-" + parsed.Call.Flag("harness", "")
		if i+1 >= len(steps) {
			t.Fatal("a raised worker lacks its matching send")
		}
		next, err := testTable().parse(splitExample(steps[i+1].Command)[1:])
		if err != nil || next.Call.Command.Name != "send" || len(next.Call.Positionals) == 0 || next.Call.Positionals[0] != address {
			t.Fatalf("%q does not match %q", step.Command, steps[i+1].Command)
		}
	}
}

type flowOfferHarness struct {
	adapter.Harness
	offer adapter.Offer
}

func (h flowOfferHarness) Offers() adapter.Offer { return h.offer }

// The guide still teaches every main launch, but only raises workers on
// worker-capable harnesses, regardless of their ability to host spaces.
func TestGuideFlowOmitsMainOnlyWorkerLaunches(t *testing.T) {
	main := flowOfferHarness{Harness: mainOnlyHarness{}, offer: adapter.Offer{MainOnly: true, Spaces: true}}
	worker := flowOfferHarness{Harness: workerSettingsHarness{}, offer: adapter.Offer{Spaces: true}}
	for _, catalog := range []adapter.Catalog{adapter.NewCatalog(main), adapter.NewCatalog(main, worker)} {
		steps := flow(catalog)
		table := newTable(catalog)
		mainSeen, workerSeen := false, false
		for _, step := range steps {
			parsed, err := table.parse(splitExample(step.Command)[1:])
			if err != nil {
				t.Fatalf("guide example %q: %v", step.Command, err)
			}
			mainSeen = mainSeen || step.Command == "rewake main-agent"
			workerSeen = workerSeen || step.Command == "rewake space open helper --alias agent-general"
			if strings.Contains(step.Command, "main-agent-general") || strings.Contains(step.Command, "helper-main-agent") {
				t.Fatalf("guide teaches a main-only worker: %q", step.Command)
			}
			if parsed.Call.Command != nil && parsed.Call.Command.Name == "space" && parsed.Call.Flags["alias"] != "agent-general" {
				t.Fatalf("unexpected worker template: %q", step.Command)
			}
		}
		if !mainSeen || workerSeen != (len(catalog.All()) > 1) {
			t.Fatalf("guide lost main or worker examples: %+v", steps)
		}
	}
}

// launchPath is the PATH of a launch these tests expect refused by its name: an
// empty one, so no harness can start. A harness that refuses an unreadable
// version before the claim — as a version is read before the name is taken —
// finds a program answering only --version there, so its refusal is the name's
// and not the version's; the program fails the test if it is run for anything
// else.
func launchPath(t *testing.T, h adapter.Harness) string {
	t.Helper()
	path := t.TempDir()
	if h.Version().Minimum == "" {
		return path
	}
	calls := filepath.Join(t.TempDir(), "calls")
	script := "#!/bin/sh\necho \"$*\" >> " + calls + "\n[ \"$*\" = --version ] && echo 999.0.0\n"
	if err := os.WriteFile(filepath.Join(path, h.ID()), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		raw, _ := os.ReadFile(calls)
		for _, call := range strings.Fields(strings.ReplaceAll(string(raw), " ", "_")) {
			if call != "--version" {
				t.Errorf("%s was started for %q, not only asked its version", h.ID(), call)
			}
		}
	})
	return path
}
