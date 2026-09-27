package claude

import (
	"slices"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/role"
)

// The conversation a launch resumes is the one --resume names; a fork, a bare
// picker and --continue name none the wrapper could look up before the start.
func TestTheResumedConversationIsTheOneTheLaunchNames(t *testing.T) {
	cases := map[string]struct {
		args []string
		want string
	}{
		"long flag":        {[]string{"--resume", "c1"}, "c1"},
		"joined value":     {[]string{"--resume=c1"}, "c1"},
		"short flag":       {[]string{"-r", "c1", "--model", "m"}, "c1"},
		"the last one":     {[]string{"-r", "c1", "--resume", "c2"}, "c2"},
		"a fork":           {[]string{"--resume", "c1", "--fork-session"}, ""},
		"the picker":       {[]string{"--resume", "--model", "m"}, ""},
		"continue":         {[]string{"--continue"}, ""},
		"a new session":    {nil, ""},
		"only a flag here": {[]string{"--add-dir", "c1"}, ""},
	}
	for name, c := range cases {
		if got := (claudeHarness{}).ResumedConversation(c.args); got != c.want {
			t.Errorf("%s: %q, want %q", name, got, c.want)
		}
	}
}

// The directories confirmed again are given at launch, one flag each, beside
// the caller's own.
func TestTheLaunchGivesTheRestoredDirectories(t *testing.T) {
	given := []string{"--resume", "c1", "--add-dir", "/w/own"}
	plan, err := New().Launch(harness.LaunchRequest{
		Name: "worker", Dir: t.TempDir(), Socket: socketPath(t), Role: role.General,
		Args: given, GrantDirs: []string{"/w/lib", "/w/doc"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := harness.FlagValues(plan.Args, addDirFlag)
	for _, want := range []string{"/w/own", "/w/lib", "/w/doc"} {
		if !slices.Contains(got, want) {
			t.Errorf("%s is not given: %v", want, plan.Args)
		}
	}
}
