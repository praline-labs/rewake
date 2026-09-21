package harness_test

import (
	"strings"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/harness"
)

// The three rules of a launch default, each checked on its own: an explicit
// flag wins, an unset variable substitutes nothing, and whatever is
// substituted is announced.

func modelDefault() harness.Default {
	return harness.Default{
		Env:     "REWAKE_TEST_MODEL",
		What:    "model",
		Present: func(args []string) bool { return harness.HasFlag(args, "--model") },
		Apply: func(args []string, value string) []string {
			return harness.AddFlags(args, "--model", value)
		},
	}
}

func TestAnUnsetVariableSubstitutesNothing(t *testing.T) {
	args, notes := harness.ApplyDefaults([]string{"--name", "worker"}, []harness.Default{modelDefault()})
	if len(args) != 2 {
		t.Errorf("arguments were changed with nothing configured: %v", args)
	}
	if len(notes) != 0 {
		t.Errorf("a note was printed with nothing configured: %v", notes)
	}
}

func TestAnExplicitFlagIsNotReplaced(t *testing.T) {
	t.Setenv("REWAKE_TEST_MODEL", "from-the-environment")
	for _, given := range [][]string{
		{"--model", "chosen-by-the-person"},
		{"--model=chosen-by-the-person"},
	} {
		args, notes := harness.ApplyDefaults(append([]string{}, given...), []harness.Default{modelDefault()})
		if strings.Contains(strings.Join(args, " "), "from-the-environment") {
			t.Errorf("%v: the default overrode what the person asked for: %v", given, args)
		}
		if len(notes) != 0 {
			t.Errorf("%v: a note claimed a substitution that did not happen: %v", given, notes)
		}
	}
}

func TestASubstitutionIsAnnounced(t *testing.T) {
	t.Setenv("REWAKE_TEST_MODEL", "from-the-environment")
	args, notes := harness.ApplyDefaults([]string{"--name", "worker"}, []harness.Default{modelDefault()})
	if !strings.Contains(strings.Join(args, " "), "--model from-the-environment") {
		t.Errorf("the configured default was not applied: %v", args)
	}
	if len(notes) != 1 {
		t.Fatalf("notes=%v, want exactly one", notes)
	}
	// The note has to name the variable: a person seeing an unexpected model
	// needs to know which setting produced it.
	if !strings.Contains(notes[0], "REWAKE_TEST_MODEL") || !strings.Contains(notes[0], "model") {
		t.Errorf("the note does not say what was substituted or from where: %q", notes[0])
	}
}

func TestAnEmptyVariableIsTreatedAsUnset(t *testing.T) {
	t.Setenv("REWAKE_TEST_MODEL", "   ")
	_, notes := harness.ApplyDefaults([]string{"--name", "worker"}, []harness.Default{modelDefault()})
	if len(notes) != 0 {
		t.Errorf("blank configuration was treated as a value: %v", notes)
	}
}

// Every spelling of a flag, including the joined short form. Missing one is
// not cosmetic: for a setting the harness refuses twice, it stops the session
// from starting at all.
func TestFlagValuesReadsEverySpelling(t *testing.T) {
	for _, given := range [][]string{
		{"--model", "chosen"},
		{"--model=chosen"},
		{"-m", "chosen"},
		{"-m=chosen"},
		{"-mchosen"},
	} {
		values := harness.FlagValues(given, "--model", "-m")
		if len(values) != 1 || values[0] != "chosen" {
			t.Errorf("%v: read %v, want [chosen]", given, values)
		}
	}
	for _, given := range [][]string{
		{"--name", "worker"},
		{"--model-provider", "something"},
		// After the terminator the words belong to the agent, not to the CLI.
		{"--", "-mchosen"},
	} {
		if values := harness.FlagValues(given, "--model", "-m"); len(values) != 0 {
			t.Errorf("%v: read %v, want nothing", given, values)
		}
	}
}
