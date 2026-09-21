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

func TestServerReportsWithoutInstallingNotify(t *testing.T) {
	codexHome(t, "model = \"configured-model\"\n")
	plan := launchPlan(t, nil)

	if _, ok := configValue(plan.Args, notifyKey); ok || plan.Backend == nil {
		t.Fatal("reporting must use the owned server, without adding notify")
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

func TestMainObservesServerOutcomes(t *testing.T) {
	codexHome(t, "")
	plan, err := New().Launch(harness.LaunchRequest{Name: "lead", Dir: t.TempDir(), Role: role.Main})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if plan.Backend == nil {
		t.Errorf("args = %v, want failures observed for main", plan.Args)
	}
}
