package launch

import (
	"reflect"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
)

type settingsHarness struct{ adapter.Harness }

func (settingsHarness) ID() string    { return "agent" }
func (settingsHarness) Title() string { return "Test agent" }
func (settingsHarness) SingleUseFlags() []adapter.Flag {
	return []adapter.Flag{{Spellings: []string{"--model"}, TakesValue: true}, {Spellings: []string{"--effort"}, TakesValue: true}}
}

func (settingsHarness) LaunchSettings(model, effort, _ string) ([]string, error) {
	return []string{"--model", model, "--effort", effort}, nil
}

func (settingsHarness) AppliesContext() bool { return false }

// These are file fields and precedence rules, not a second space workflow.
func TestAliasSettingsParsingAndExpansion(t *testing.T) {
	catalog := adapter.NewCatalog(settingsHarness{})
	text := `[alias.x]
harness = "agent"
main = true
description = "Checks a patch"
args = ["--model", "old", "--", "literal prompt"]
[alias.x.launch]
model = "chosen"
effort = "high"
context = "large"
`
	s := set(t, text)
	value, _, ok := s.Lookup("x")
	if !ok || !value.Main || value.Description != "Checks a patch" || value.Launch.Context != "large" || !value.Launch.Applied() {
		t.Fatalf("parsed alias: %+v", value)
	}
	got, err := s.ExpandConfigured([]string{"x"}, catalog, valued, nil)
	want := []string{"agent", "--model", "chosen", "--effort", "high", "--", "literal prompt"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("expanded %v, %v; want %v", got, err, want)
	}
	if !reflect.DeepEqual(value.Args, []string{"--model", "old", "--", "literal prompt"}) {
		t.Fatalf("expansion changed stored args: %v", value.Args)
	}

	s = set(t, strings.Replace(text, `, "--", "literal prompt"`, "", 1))
	got, err = s.ExpandConfigured([]string{"x", "--model=typed", "--effort", "low"}, catalog, valued, nil)
	want = []string{"agent", "--model=typed", "--effort", "low"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("typed overrides: %v, %v; want %v", got, err, want)
	}

	s = set(t, text+"apply = false\n")
	got, err = s.ExpandConfigured([]string{"x"}, catalog, valued, nil)
	want = []string{"agent", "--model", "old", "--", "literal prompt"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("display-only settings reached launch: %v, %v", got, err)
	}
}

func TestUnreadableAliasesNameSourceAndLine(t *testing.T) {
	s := set(t, "[alias.x]\nharness = [\n")
	if s.Err() == nil || !strings.Contains(s.Err().Error(), ".rewake.toml:2:") {
		t.Fatalf("broken file has no source and line: %v", s.Err())
	}
}
