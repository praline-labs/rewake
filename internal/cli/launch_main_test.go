package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/brief"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/infra/state"
)

type roleLaunchProbe struct {
	adapter.Harness
	request adapter.LaunchRequest
}

func (p *roleLaunchProbe) Launch(request adapter.LaunchRequest) (adapter.LaunchPlan, error) {
	p.request = request
	return adapter.LaunchPlan{Command: "/bin/true", Env: request.Env}, nil
}

// A person's launch is main, in the room its directory names — the directory
// with symlinks followed, recorded in the room — and its briefing says why.
func TestALaunchIsMainInTheRoomOfItsDirectory(t *testing.T) {
	for _, h := range testCatalog().All() {
		t.Run(h.ID(), func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Chmod(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			t.Setenv(state.DirEnv, dir)
			t.Setenv(state.RoomEnv, "")
			project := filepath.Join(t.TempDir(), "My Project")
			if err := os.Mkdir(project, 0o700); err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(t.TempDir(), "link")
			if err := os.Symlink(project, link); err != nil {
				t.Fatal(err)
			}
			t.Chdir(link)
			// The version is asked of a stand-in, never of whatever
			// program of that name this machine has.
			t.Setenv("PATH", launchPath(t, h))
			parsed, err := testTable().parse([]string{h.ID()})
			if err != nil {
				t.Fatal(err)
			}
			probe := &roleLaunchProbe{Harness: h}
			if err := handleLaunch(probe)(&Context{Stdout: io.Discard, Stderr: io.Discard, table: testTable()}, parsed.Call); err != nil {
				t.Fatal(err)
			}
			got := probe.request
			room, resolved, err := state.RoomOf(project)
			if err != nil {
				t.Fatal(err)
			}
			if got.Role.ID != role.Main.ID || got.Name != role.Main.ID+"-"+h.ID() || got.Room != room || !strings.HasPrefix(room, "my-project-") {
				t.Fatalf("wrong role, name or room: %s %s %s", got.Role.ID, got.Name, got.Room)
			}
			if recorded := state.RoomDirectory(got.RoomDir); recorded != resolved {
				t.Fatalf("the room records %q, want %q", recorded, resolved)
			}
			if intro := brief.Intro(got.BriefContext()); !strings.Contains(intro, "always starts main") {
				t.Fatalf("the briefing lacks the reason: %s", intro)
			}
			outsideAnySession(t)
			if code, out, errOut := run("list"); code != ExitOK || !strings.Contains(out, room+" ("+resolved+")") {
				t.Fatalf("list from the directory: %d %q %s", code, out, errOut)
			}
		})
	}
}

func TestGuideSaysAPersonStartsOnlyMain(t *testing.T) {
	code, out, errOut := run("guide")
	out = strings.Join(strings.Fields(out), " ")
	if code != ExitOK || !strings.Contains(out, "A person starts only main") || strings.Contains(out, "--general") || strings.Contains(out, "No role flag means general") {
		t.Fatalf("guide=%d %s %s", code, out, errOut)
	}
}

// A project that still carries the old worker type and lists a template the
// catalog no longer has does not block main: it starts on the rest, and its
// briefing names each obsolete setting with what to write
// (docs/work-modes.md#the-old-worker-types).
func TestAMainLaunchesOnAStaleProjectFileAndIsToldWhatToFix(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv(state.DirEnv, dir)
	t.Setenv(state.RoomEnv, "")
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, ".rewake"), 0o700); err != nil {
		t.Fatal(err)
	}
	stale := "[alias.gone]\nharness = \"agent\"\ntype = \"code\"\n[alias.kept]\nharness = \"agent\"\n[aliases]\noff = [\"removed-write\"]\n"
	if err := os.WriteFile(filepath.Join(project, ".rewake", "aliases.toml"), []byte(stale), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(project)
	h := testCatalog().All()[0]
	t.Setenv("PATH", launchPath(t, h))
	parsed, err := testTable().parse([]string{h.ID()})
	if err != nil {
		t.Fatal(err)
	}
	outsideAnySession(t)
	probe := &roleLaunchProbe{Harness: h}
	if err := handleLaunch(probe)(&Context{Stdout: io.Discard, Stderr: io.Discard, table: testTable()}, parsed.Call); err != nil {
		t.Fatalf("a stale project file blocked main: %v", err)
	}
	intro := brief.Intro(probe.request.BriefContext())
	for _, want := range []string{
		"skipped as obsolete",
		"alias.gone",
		"obsolete type field",
		"start_mode",
		`turns off "removed-write" in [aliases] off`,
		"already off",
	} {
		if !strings.Contains(intro, want) {
			t.Errorf("main's briefing lacks %q: %s", want, intro)
		}
	}
}
