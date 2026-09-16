package codex

import (
	"strings"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/role"
)

func launchPlan(t *testing.T, args []string) harness.LaunchPlan {
	t.Helper()
	plan, err := New().Launch(harness.LaunchRequest{Name: "web", Dir: t.TempDir(), Args: args, Intro: false})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	return plan
}

func TestTurnNotifyIsPassedWhenTheUserHasNone(t *testing.T) {
	codexHome(t, "model = \"gpt-5\"\n")
	plan := launchPlan(t, nil)

	value, ok := configValue(plan.Args, notifyKey)
	if !ok {
		t.Fatalf("notify was not passed: %v", plan.Args)
	}
	if !strings.HasPrefix(value, "[") || !strings.HasSuffix(value, `"turn-ended"]`) {
		t.Errorf("notify = %s, want an array running rewake turn-ended", value)
	}
}

// notify replaces the user's program, so any sign of one keeps rewake's out.
func TestTurnNotifyLeavesTheUsersProgram(t *testing.T) {
	for name, config := range map[string]string{
		"a top-level key": "notify = [\"notify-send\", \"codex\"]\n",
		"a quoted key":    "\"notify\" = [\"x\"]\n",
		"a nested table":  "[profiles.work]\nnotify = [\"x\"]\n",
		"an escaped key":  "\"not\\u0069fy\" = [\"mine\"]\n",
	} {
		t.Run(name, func(t *testing.T) {
			codexHome(t, config)
			plan := launchPlan(t, nil)
			if _, ok := configValue(plan.Args, notifyKey); ok {
				t.Errorf("notify was passed over %q: %v", config, plan.Args)
			}
			if len(plan.Notes) == 0 {
				t.Error("nothing says the end of turns will not be reported")
			}
		})
	}
}

func TestTurnNotifyLeavesTheCallersValue(t *testing.T) {
	codexHome(t, "")
	plan := launchPlan(t, []string{"-c", `notify=["mine"]`})

	count := 0
	for _, arg := range plan.Args {
		if strings.HasPrefix(arg, notifyKey+"=") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("args = %v, want only the caller's notify", plan.Args)
	}
}

func TestTheMainSessionGetsNoNotify(t *testing.T) {
	codexHome(t, "")
	plan, err := New().Launch(harness.LaunchRequest{Name: "lead", Dir: t.TempDir(), Role: role.Main})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if _, ok := configValue(plan.Args, notifyKey); ok {
		t.Errorf("args = %v, want no notify for the main session", plan.Args)
	}
}
