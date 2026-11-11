package cli

// Main's authorship policy through the commands a session and a person see:
// a live main's frozen record, the configuration a fresh launch would take,
// the guide, and the launch that resolves it once (docs/roles.md).

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/brief"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// aLiveMain makes a live main record and returns the room's state directory.
func aLiveMain(t *testing.T, implements bool) string {
	t.Helper()
	dir := liveSessionAs(t, "lead", role.Main.ID)
	record, err := registry.Load(dir, "lead")
	if err != nil {
		t.Fatal(err)
	}
	record.MainImplements = implements
	encoded, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(state.SessionPath(dir, "lead"), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	// The commands under test run as that session, as a session's own shell
	// makes them.
	t.Setenv(state.SessionEnv, "lead")
	return dir
}

// inProject runs every following command from a project whose file says what
// it is given, with its own empty personal file.
func inProject(t *testing.T, text string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return writeProject(t, text)
}

func writeProject(t *testing.T, text string) string {
	t.Helper()
	project := t.TempDir()
	t.Chdir(project)
	path := filepath.Join(project, ".rewake", "aliases.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return project
}

// A live main's record is the room's policy for every caller, and editing the
// configuration does not change it. Outside a live main the listing says the
// configuration is what a fresh launch here would take.
func TestWorkersTakesALiveMainsPolicyFromItsRecord(t *testing.T) {
	aLiveMain(t, true)
	inProject(t, "[main]\nimplements = false\n[alias.agent-general]\nharness = \"agent\"\n")
	code, out, errOut := run("workers", "--json")
	var view workersListing
	if code != ExitOK || json.Unmarshal([]byte(out), &view) != nil {
		t.Fatalf("workers: %d %s %s", code, out, errOut)
	}
	if !view.MainImplements || view.MainPolicySource != mainPolicySession || !view.NoWriter {
		t.Fatalf("a live main's policy: %+v", view)
	}
	code, out, errOut = run("workers")
	if code != ExitOK || !strings.Contains(out, "Main implementation: enabled by this room's live main") ||
		!strings.Contains(out, "editing the alias file does not change this run") || !strings.Contains(out, "Main may implement product changes itself") {
		t.Fatalf("workers lines: %d %s %s", code, out, errOut)
	}

	// The same command with no live main answers from the file it stands in:
	// false here, marked as policy for a fresh launch.
	t.Setenv(state.DirEnv, filepath.Join(t.TempDir(), "never-created"))
	code, out, errOut = run("workers", "--json")
	if code != ExitOK || json.Unmarshal([]byte(out), &view) != nil {
		t.Fatalf("workers outside a room: %d %s %s", code, out, errOut)
	}
	if view.MainImplements || view.MainPolicySource != mainPolicyConfiguration {
		t.Fatalf("a fresh launch's policy: %+v", view)
	}
	code, out, errOut = run("workers")
	if code != ExitOK || !strings.Contains(out, "delegated by this directory's project configuration ([main] implements = false)") {
		t.Fatalf("workers lines outside a room: %d %s %s", code, out, errOut)
	}
}

// A project that sets nothing is the default, said as such, not as a decision
// somebody made.
func TestWorkersSaysTheDefaultIsNotAProjectDecision(t *testing.T) {
	inProject(t, "[alias.agent-general]\nharness = \"agent\"\n")
	code, out, errOut := run("workers")
	if code != ExitOK || !strings.Contains(out, "delegated, the default because this directory's project sets no [main] implements") {
		t.Fatalf("the default: %d %s %s", code, out, errOut)
	}
}

// The guide is the role's own text under the run's policy, and an edit after
// the launch changes neither: a delegating main keeps the prohibition, an
// implementing one gets the implementing rule.
func TestGuideFollowsTheRunsFrozenPolicy(t *testing.T) {
	for _, tc := range []struct {
		name       string
		implements bool
		want, bad  string
	}{
		{"delegating", false, "Do not write product code yourself", "may implement product changes yourself"},
		{"implementing", true, "may implement product changes yourself", "Do not write product code yourself"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			aLiveMain(t, tc.implements)
			// A later file says the opposite, and must not reach the guide.
			inProject(t, "[main]\nimplements = "+map[bool]string{true: "false", false: "true"}[tc.implements]+"\n")
			code, out, errOut := run("guide")
			out = strings.Join(strings.Fields(out), " ")
			if code != ExitOK || !strings.Contains(out, tc.want) || strings.Contains(out, tc.bad) {
				t.Fatalf("guide=%d %s %s", code, out, errOut)
			}
		})
	}
}

// The launch resolves the policy once, from the project file of the directory
// it starts in, and the briefing says which. A fresh launch here takes the
// current value; the launch after a change takes the changed one.
func TestALaunchFreezesTheProjectPolicyItFound(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv(state.DirEnv, dir)
	t.Setenv(state.RoomEnv, "")
	h := testCatalog().All()[0]
	t.Setenv("PATH", launchPath(t, h))
	inProject(t, "[main]\nimplements = true\n")
	probe := launchProbe(t, h)
	if !probe.request.MainImplements || !probe.request.MainPolicyExplicit {
		t.Fatalf("an opted-in main launched with %+v", probe.request)
	}
	if text := briefingOf(probe); !strings.Contains(text, "lets main implement product changes itself") {
		t.Fatalf("the briefing: %s", text)
	}
	// The record the launch wrote while it ran held the same value: the
	// request is what claimName writes.
	writeProject(t, "[main]\nimplements = false\n")
	probe = launchProbe(t, h)
	if probe.request.MainImplements || !probe.request.MainPolicyExplicit {
		t.Fatalf("a changed project launched with %+v", probe.request)
	}
	if text := briefingOf(probe); !strings.Contains(text, "the project's [main] implements = false") {
		t.Fatalf("the briefing after the change: %s", text)
	}
}

func launchProbe(t *testing.T, h adapter.Harness) *roleLaunchProbe {
	t.Helper()
	parsed, err := testTable().parse([]string{h.ID()})
	if err != nil {
		t.Fatal(err)
	}
	probe := &roleLaunchProbe{Harness: h}
	if err := handleLaunch(probe)(&Context{Stdout: io.Discard, Stderr: io.Discard, table: testTable()}, parsed.Call); err != nil {
		t.Fatal(err)
	}
	return probe
}

func briefingOf(probe *roleLaunchProbe) string {
	return brief.Intro(probe.request.BriefContext())
}
