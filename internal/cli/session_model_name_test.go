package cli

import (
	"testing"

	"github.com/praline-labs/rewake/internal/core/session"
)

func TestStateCellsShowTheModelNameWhenThereIsOne(t *testing.T) {
	id, name := "some-model-id[1m]", "Some Model 5"
	snapshot := session.Unknown("e")
	snapshot.Model, snapshot.Fresh, snapshot.SettingsFresh = &id, true, true
	if got := stateCells(&snapshot)[1]; got != `"some-model-id[1m]"` {
		t.Fatalf("without a name the cell is %s", got)
	}
	snapshot.ModelName = &name
	if got := stateCells(&snapshot)[1]; got != `"Some Model 5"` {
		t.Fatalf("with a name the cell is %s", got)
	}
}

// A display name loses only a trailing parenthesised part, such as a native
// window the run may not be held to.
func TestShortModelNameDropsTheParenthesisedSuffix(t *testing.T) {
	for name, want := range map[string]string{
		"Some Model 5 (1M context)": "Some Model 5",
		"Some Model 5":              "Some Model 5",
		"Model (a) (b)":             "Model (a)",
		"(only)":                    "(only)",
		"Model (open":               "Model (open",
	} {
		if got := shortModelName(name); got == nil || *got != want {
			t.Fatalf("%q: %v, want %q", name, got, want)
		}
	}
	if got := shortModelName(" "); got != nil {
		t.Fatalf("a blank name: %q", *got)
	}
}
