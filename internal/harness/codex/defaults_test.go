package codex

import (
	"strings"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/role"
)

// The adapter applies the environment defaults in the form Codex takes them:
// a flag for the model, a configuration key for the reasoning effort.
func TestLaunchTakesModelAndEffortFromTheEnvironment(t *testing.T) {
	codexHome(t, "")
	t.Setenv("REWAKE_CODEX_MODEL", "configured-model")
	t.Setenv("REWAKE_CODEX_EFFORT", "low")

	plan, err := New().Launch(harness.LaunchRequest{Name: "worker", Dir: t.TempDir(), Role: role.General})
	if err != nil {
		t.Fatal(err)
	}
	line := strings.Join(plan.Args, " ")
	if !strings.Contains(line, "--model configured-model") {
		t.Errorf("the configured model was not passed: %v", plan.Args)
	}
	if !strings.Contains(line, `model_reasoning_effort="low"`) {
		t.Errorf("the configured reasoning effort was not passed: %v", plan.Args)
	}
	if len(plan.Notes) == 0 {
		t.Error("the substitution was not announced")
	}
}

// The joined short form is what separates this check from Claude Code's, and
// getting it wrong does not merely override a choice: Codex refuses a repeated
// model flag, so the session does not start.
func TestLaunchRespectsEverySpellingOfTheCallersChoice(t *testing.T) {
	codexHome(t, "")
	t.Setenv("REWAKE_CODEX_MODEL", "configured-model")
	t.Setenv("REWAKE_CODEX_EFFORT", "low")

	for _, given := range [][]string{
		{"--model", "asked-for"},
		{"--model=asked-for"},
		{"-m", "asked-for"},
		{"-m=asked-for"},
		{"-masked-for"},
	} {
		plan, err := New().Launch(harness.LaunchRequest{Name: "worker", Dir: t.TempDir(), Args: given, Role: role.General})
		if err != nil {
			t.Fatalf("%v: %v", given, err)
		}
		if strings.Contains(strings.Join(plan.Args, " "), "configured-model") {
			t.Errorf("%v: a second model flag was added, which Codex refuses: %v", given, plan.Args)
		}
	}
	for _, given := range [][]string{
		{"-c", `model_reasoning_effort="high"`},
		{"--config", `model_reasoning_effort="high"`},
		{"-c=model_reasoning_effort=\"high\""},
		{`-cmodel_reasoning_effort="high"`},
		{`--config=model_reasoning_effort="high"`},
	} {
		plan, err := New().Launch(harness.LaunchRequest{Name: "worker", Dir: t.TempDir(), Args: given, Role: role.General})
		if err != nil {
			t.Fatalf("%v: %v", given, err)
		}
		if strings.Contains(strings.Join(plan.Args, " "), `model_reasoning_effort="low"`) {
			t.Errorf("%v: the caller's own setting was overridden: %v", given, plan.Args)
		}
	}
}

func TestLaunchLeavesTheCallersOwnChoiceAlone(t *testing.T) {
	codexHome(t, "")
	t.Setenv("REWAKE_CODEX_MODEL", "configured-model")
	t.Setenv("REWAKE_CODEX_EFFORT", "low")

	given := []string{"--model", "asked-for", "-c", `model_reasoning_effort="high"`}
	plan, err := New().Launch(harness.LaunchRequest{Name: "worker", Dir: t.TempDir(), Args: given, Role: role.General})
	if err != nil {
		t.Fatal(err)
	}
	line := strings.Join(plan.Args, " ")
	if strings.Contains(line, "configured-model") || strings.Contains(line, `="low"`) {
		t.Errorf("a default overrode what the caller asked for: %v", plan.Args)
	}
	for _, note := range plan.Notes {
		if strings.Contains(note, "REWAKE_CODEX_") {
			t.Errorf("a note claimed a substitution that did not happen: %q", note)
		}
	}
}

// A person can choose a model or an effort with a configuration key instead of
// a flag, and can write whitespace around that key: the CLI trims it. Both are
// the same explicit choice, and a default must not land on top of either.
func TestExplicitConfigChoiceIsNotOverridden(t *testing.T) {
	for _, tc := range []struct{ name, env, value, setting, forbidden string }{
		{"model-config", "REWAKE_CODEX_MODEL", "configured-model", `model="caller-model"`, "configured-model"},
		{"effort-space", "REWAKE_CODEX_EFFORT", "low", `model_reasoning_effort = "high"`, `model_reasoning_effort="low"`},
		{"effort-leading-space", "REWAKE_CODEX_EFFORT", "low", ` model_reasoning_effort="high"`, `model_reasoning_effort="low"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			codexHome(t, "")
			t.Setenv("REWAKE_CODEX_MODEL", "")
			t.Setenv("REWAKE_CODEX_EFFORT", "")
			t.Setenv(tc.env, tc.value)
			plan, err := New().Launch(harness.LaunchRequest{Name: "worker", Dir: t.TempDir(), Args: []string{"-c", tc.setting}, Role: role.General})
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(strings.Join(plan.Args, " "), tc.forbidden) {
				t.Fatalf("explicit choice overridden: %q", plan.Args)
			}
		})
	}
}
