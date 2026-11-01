package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/parts"
	"github.com/praline-labs/rewake/internal/infra/state"
	"github.com/praline-labs/rewake/internal/infra/update"
)

// fromNPM makes this test's rewake an npm install's under /opt/node, whose
// npm answers as answer says, with no network.
func fromNPM(t *testing.T, answer update.Runner) *[][]string {
	t.Helper()
	was, wasRunner, wasDir := update.Channel, npmRunner, installedDir
	t.Cleanup(func() { update.Channel, npmRunner, installedDir = was, wasRunner, wasDir })
	update.Channel = "npm"
	var calls [][]string
	npmRunner = func(ctx context.Context, dir string, args []string, stdout, stderr io.Writer) error {
		calls = append(calls, args)
		return answer(ctx, dir, args, stdout, stderr)
	}
	installedDir = func() string {
		return "/opt/node/lib/node_modules/@praline-labs/rewake/node_modules/@praline-labs/rewake-linux-x64/bin"
	}
	return &calls
}

func updateRoom(t *testing.T) {
	t.Helper()
	t.Setenv(state.DirEnv, filepath.Join(t.TempDir(), "state"))
	t.Setenv(state.RoomEnv, "")
	t.Setenv(state.SessionEnv, "")
	t.Setenv(epochEnv, "")
	buildtest.Current(t)
}

// rewake update <version> installs only a release the registry has: one it
// does not have is refused, and npm is never asked to install it.
func TestUpdateRefusesAVersionThatDoesNotExist(t *testing.T) {
	updateRoom(t)
	calls := fromNPM(t, func(_ context.Context, _ string, args []string, stdout, _ io.Writer) error {
		if args[0] == "view" {
			_, _ = fmt.Fprint(stdout, `{"error":{"code":"E404","summary":"No match found for version 9.9.9"}}`)
			return errors.New("exit status 1")
		}
		return nil
	})
	code, _, errOut := run("update", "9.9.9")
	if code != ExitFailed || !strings.Contains(errOut, "no such release 9.9.9 of @praline-labs/rewake on the registry") {
		t.Errorf("update 9.9.9: %d %q", code, errOut)
	}
	for _, args := range *calls {
		if args[0] == "install" {
			t.Errorf("npm was asked to install: %v", args)
		}
	}
	for _, wrong := range []string{"latest", "^2.12.0", "2.12.0-linux-x64"} {
		if code, _, errOut := run("update", wrong); code != ExitUsage {
			t.Errorf("update %s: %d %q, want a wrong call", wrong, code, errOut)
		}
	}
}

// npm replaces the running install by moving it aside, so the executable's
// path names a removed directory once it has: the plan after the install
// reads the install the command came from (seen live, November 1, 2026).
func TestUpdatePlansFromTheInstallItCameFrom(t *testing.T) {
	updateRoom(t)
	origin := filepath.Join(t.TempDir(), "lib", "node_modules", "@praline-labs", "rewake", "node_modules", "@praline-labs", "rewake-linux-x64", "bin")
	if err := os.MkdirAll(origin, 0o755); err != nil {
		t.Fatal(err)
	}
	platformBuild := func(version string) {
		pkg := fmt.Sprintf(`{"name":%q,"version":"%s-linux-x64"}`, update.Package, version)
		if err := os.WriteFile(filepath.Join(origin, "..", "package.json"), []byte(pkg), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	platformBuild("2.12.0")
	fromNPM(t, func(_ context.Context, _ string, args []string, stdout, _ io.Writer) error {
		switch args[0] {
		case "view":
			_, _ = fmt.Fprint(stdout, `"2.12.1"`)
		case "install":
			platformBuild("2.12.1")
			m := parts.Manifest{Parts: []parts.Part{{Name: parts.Rewake, Class: parts.Room, File: "another build"}}}
			if err := os.WriteFile(filepath.Join(origin, parts.FileName), parts.Encode(m), 0o644); err != nil {
				return err
			}
			moved := filepath.Join(filepath.Dir(origin), "..", "..", ".rewake-moved", "bin")
			installedDir = func() string { return moved }
		}
		return nil
	})
	installedDir = func() string { return origin }
	code, out, errOut := run("update", "2.12.1")
	if code != ExitOK || !strings.Contains(out, "Rewake: installed 2.12.1") || !strings.Contains(out, "rewake: changed → restart the room") {
		t.Errorf("update 2.12.1: %d %q %q", code, out, errOut)
	}
}

// A build not from npm — tools/land's, one by hand — updates by being built
// again; it asks npm nothing.
func TestUpdateRefusesABuildNotFromNPM(t *testing.T) {
	updateRoom(t)
	was := update.Channel
	t.Cleanup(func() { update.Channel = was })
	update.Channel = ""
	code, _, errOut := run("update")
	if code != ExitFailed || !strings.Contains(errOut, "this rewake was not installed from npm") {
		t.Errorf("update: %d %q", code, errOut)
	}
}

// The most a plan asks of the room is what the banner names.
func TestRestartsOfAPlan(t *testing.T) {
	for _, c := range []struct {
		steps []parts.Step
		want  Restarts
	}{
		{nil, RestartsNothing},
		{[]parts.Step{{Part: "face", Class: parts.Live}, {Part: "pi", Class: parts.Sessions}}, RestartsNothing},
		{[]parts.Step{{Part: "shell", Class: parts.Shell}}, RestartsShell},
		{[]parts.Step{{Part: "shell", Class: parts.Shell}, {Part: "rewake", Class: parts.Room}}, RestartsRoom},
	} {
		if got := restartsOf(c.steps); got != c.want {
			t.Errorf("restartsOf(%v) = %s, want %s", c.steps, got, c.want)
		}
	}
}
