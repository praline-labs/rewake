package launch_test

// A worker's starting-mode selections are already in its arguments when
// pi.Launch sees them: launch.Set.Raise turns the mode's fields into the
// worker's argv. The launch must still name the mode as their source, while an
// explicit argument after -- that changes the selection stays explicit.

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/adapter/catalog"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/core/workmode"
	"github.com/praline-labs/rewake/internal/host/launch"
)

const plainWorkerCatalog = `[alias.helper]
harness = "pi"
role = "general"
`

const workerModeCatalog = `[alias.writer]
harness = "pi"
role = "write"
[alias.writer.launch]
model = "template/model"
[alias.writer.pi]
start_mode = "build"
[alias.writer.pi.modes.build]
model = "mode/model"
effort = "high"
`

// workerModeSet reads the catalog in a private HOME and project.
func workerModeSet(t *testing.T) *launch.Set { return workerSet(t, workerModeCatalog) }

// workerSet reads one catalog in a private HOME and project.
func workerSet(t *testing.T, text string) *launch.Set {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("REWAKE_PI_MODEL", "")
	t.Setenv("REWAKE_PI_EFFORT", "")
	project := t.TempDir()
	t.Chdir(project)
	dir := filepath.Join(home, ".config", "rewake")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "aliases.toml"), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	set := launch.LoadIn(project)
	if err := set.Err(); err != nil {
		t.Fatal(err)
	}
	return set
}

// workerLaunch resolves an alias through Raise and launches it as the space
// host does, with the mode the catalog resolved.
func workerLaunch(t *testing.T, set *launch.Set, typed ...string) adapter.LaunchPlan {
	t.Helper()
	return workerLaunchAlias(t, set, "writer", typed...)
}

func workerLaunchAlias(t *testing.T, set *launch.Set, alias string, typed ...string) adapter.LaunchPlan {
	t.Helper()
	raised, err := set.Raise(catalog.New(), launch.Request{Alias: alias, Args: typed})
	if err != nil {
		t.Fatal(err)
	}
	request := adapter.LaunchRequest{
		Role: raised.Role, Template: raised.Template, Args: raised.Args,
		Mode: raised.Mode, Modes: set.Modes(alias),
		RunDir: t.TempDir(), Socket: filepath.Join(t.TempDir(), "s.sock"),
	}
	h, ok := catalog.New().Find("pi")
	if !ok {
		t.Fatal("the catalog has no pi")
	}
	plan, err := h.Launch(request)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestAWorkersStartingModeSelectionIsNotedThroughRaiseAndLaunch(t *testing.T) {
	set := workerModeSet(t)
	plan := workerLaunch(t, set)
	if got := adapter.FlagValues(plan.Args, "--model"); !slices.Equal(got, []string{"mode/model"}) {
		t.Fatalf("precedence changed: %v", got)
	}
	if got := adapter.FlagValues(plan.Args, "--thinking"); !slices.Equal(got, []string{"high"}) {
		t.Fatalf("effort changed: %v", got)
	}
	if !strings.Contains(strings.Join(plan.Notes, "\n"), `the starting mode "build" supplied the model and thinking level`) {
		t.Fatalf("the worker used the starting mode's selections without a launch-time source note: %q", plan.Notes)
	}
}

// A typed value equal to the mode's cannot be told from the mode's own in the
// argv. It keeps the note: the selection is the same, and the note is a
// launch-time diagnostic, never the choice of a model.
func TestAWorkersEqualExplicitValueKeepsTheNote(t *testing.T) {
	set := workerModeSet(t)
	plan := workerLaunch(t, set, "--model", "mode/model", "--thinking", "high")
	if !strings.Contains(strings.Join(plan.Notes, "\n"), `the starting mode "build" supplied the model and thinking level`) {
		t.Fatalf("the equal explicit value lost the note: %q", plan.Notes)
	}
}

// A template whose mode configures no model or effort has no source to name.
func TestAWorkerWithoutModeFieldsGetsNoNote(t *testing.T) {
	set := workerSet(t, plainWorkerCatalog)
	plan := workerLaunchAlias(t, set, "helper")
	if strings.Contains(strings.Join(plan.Notes, "\n"), "starting mode") {
		t.Fatalf("an unset selection was said: %q", plan.Notes)
	}
}

// A main launch's arguments are the caller's own: there an explicit value equal
// to the mode's stays explicit, and only an absent flag is the mode's.
func TestAMainLaunchesOwnArgumentsKeepTheExplicitRule(t *testing.T) {
	config := workmode.Config{Modes: map[string]workmode.Fields{workmode.Build: {Model: "mode/model", Effort: "high"}}}
	launchAsMain := func(args []string) adapter.LaunchPlan {
		t.Helper()
		h, ok := catalog.New().Find("pi")
		if !ok {
			t.Fatal("the catalog has no pi")
		}
		plan, err := h.Launch(adapter.LaunchRequest{
			Role: role.Main, Args: args, Mode: workmode.BuiltIns()[workmode.Build], Modes: config,
			RunDir: t.TempDir(), Socket: filepath.Join(t.TempDir(), "s.sock"),
		})
		if err != nil {
			t.Fatal(err)
		}
		return plan
	}
	if notes := launchAsMain([]string{"--model", "mode/model", "--thinking", "high"}).Notes; len(notes) != 0 {
		t.Fatalf("main's explicit value was attributed to the mode: %q", notes)
	}
	if notes := launchAsMain(nil).Notes; len(notes) != 1 {
		t.Fatalf("main's absent flags did not name the mode: %q", notes)
	}
}

func TestAWorkersExplicitSelectionStaysExplicit(t *testing.T) {
	set := workerModeSet(t)
	plan := workerLaunch(t, set, "--model", "typed/model", "--thinking", "low")
	if got := adapter.FlagValues(plan.Args, "--model"); !slices.Equal(got, []string{"typed/model"}) {
		t.Fatalf("the explicit model did not win: %v", got)
	}
	if got := adapter.FlagValues(plan.Args, "--thinking"); !slices.Equal(got, []string{"low"}) {
		t.Fatalf("the explicit effort did not win: %v", got)
	}
	if strings.Contains(strings.Join(plan.Notes, "\n"), `the starting mode "build" supplied`) {
		t.Fatalf("an explicit selection was attributed to the mode: %q", plan.Notes)
	}
}
