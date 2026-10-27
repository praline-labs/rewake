package claude

import (
	"slices"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/role"
)

// launchWindow launches a session of the role with the arguments and returns
// the --autocompact values the plan passes, and the launch's error.
func launchWindow(t *testing.T, part role.Role, args ...string) ([]string, error) {
	t.Helper()
	privateHome(t)
	plan, err := New().Launch(adapter.LaunchRequest{Name: "worker", Dir: t.TempDir(), Socket: socketPath(t), Args: args, Role: part})
	return adapter.FlagValues(plan.Args, autocompactFlag), err
}

// The window is 272k unless something nearer names one: the owner's default.
func TestClaudeLaunchKeepsTo272kUnlessSomethingNearerSays(t *testing.T) {
	cases := []struct {
		name string
		env  string
		args []string
		role role.Role
		want []string
	}{
		{"nothing set", "", nil, role.General, []string{"272k"}},
		{"main gets it too", "", nil, role.Main, []string{"272k"}},
		{"the setting beats it", "300k", nil, role.General, []string{"300k"}},
		{"auto in the setting leaves the model's window", "auto", nil, role.General, []string{"auto"}},
		{"a template's flag beats the setting", "300k", []string{"--autocompact", "auto"}, role.General, []string{"auto"}},
		{"a typed flag beats the setting", "300k", []string{"--autocompact=500k"}, role.General, []string{"500k"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("REWAKE_CLAUDE_CONTEXT", c.env)
			got, err := launchWindow(t, c.role, c.args...)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, c.want) {
				t.Errorf("--autocompact %v, want %v", got, c.want)
			}
		})
	}
}

func TestClaudeLaunchRefusesABadWindowFromTheSettings(t *testing.T) {
	t.Setenv("REWAKE_CLAUDE_CONTEXT", "50k")
	_, err := launchWindow(t, role.General)
	if err == nil || !strings.Contains(err.Error(), "REWAKE_CLAUDE_CONTEXT") || !strings.Contains(err.Error(), "the environment") {
		t.Fatalf("a bad window did not refuse the launch naming its source: %v", err)
	}
	if _, err := launchWindow(t, role.General, "--autocompact", "300k"); err != nil {
		t.Errorf("a bad setting refused a launch that never used it: %v", err)
	}
}
