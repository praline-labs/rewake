package host

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/core/workmode"
)

// modesForHandover is a resolved catalog as space open leaves it: the
// built-ins plus one mode of the person's own, each with its text.
func modesForHandover() workmode.Config {
	all := workmode.BuiltIns()
	all["audit"] = workmode.Mode{Name: "audit", Description: "audit the change", Text: "AUDIT-TEXT: read the diff and the brief, then report.", Source: workmode.User}
	return workmode.Config{StartMode: workmode.Review, Catalog: workmode.Inventory(all)}
}

// A wrapper handover carries the resolved catalog as JSON, so every mode's
// text must survive it: a mode that lost its text would switch to a bare
// header, and a reloaded extension would lose the discipline it was given.
// The test runs at the boundary itself — marshal, ReadHandover, Request —
// rather than over plans built apart from it.
func TestAHandoverKeepsEveryModesDiscipline(t *testing.T) {
	modes := modesForHandover()
	encoded, err := json.Marshal(Handover{
		Version: HandoverVersion, Harness: "pi", Dir: t.TempDir(), Name: "api-pi", Epoch: "e1",
		Role: "write", Mode: modes.Catalog[0], Modes: modes,
	})
	if err != nil {
		t.Fatal(err)
	}
	handed, err := ReadHandover(string(encoded))
	if err != nil {
		t.Fatal(err)
	}
	request := handed.Request(nil)
	if request.Mode.Text == "" {
		t.Errorf("the startup mode lost its text")
	}
	if len(request.Modes.Catalog) != len(modes.Catalog) {
		t.Fatalf("the catalog lost modes: %d of %d", len(request.Modes.Catalog), len(modes.Catalog))
	}
	for _, want := range modes.Catalog {
		got, ok := workmode.Find(request.Modes.Catalog, want.Name)
		if !ok {
			t.Errorf("the catalog lost %q", want.Name)
			continue
		}
		if got.Text != want.Text {
			t.Errorf("%s: %d of %d bytes survived the handover", want.Name, len(got.Text), len(want.Text))
		}
	}
	// The modes a run switches to, one built-in and one of the person's own:
	// each carries its whole discipline, one section and no other mode's.
	for _, name := range []string{workmode.Advisor, "audit"} {
		got, ok := workmode.Find(request.Modes.Catalog, name)
		if !ok {
			t.Fatalf("no %q after the handover", name)
		}
		section := workmode.Instructions(got, false, false)
		if strings.Count(section, "Runtime work mode:") != 1 || !strings.Contains(section, got.Text) {
			t.Fatalf("%s after the handover reads %q", name, section)
		}
		if strings.TrimSpace(got.Text) == "" {
			t.Fatalf("%s after the handover has no text", name)
		}
	}
	if next := workmode.Next(request.Mode.Name, request.Modes.Catalog); next != workmode.Advisor {
		t.Fatalf("the cycle after the handover starts at %q, not advisor", next)
	}
}
