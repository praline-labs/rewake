package pi

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
)

// shippedPackage lays out pi's package under dir/node_modules as npm does.
func shippedPackage(t *testing.T, dir, manifest string) string {
	t.Helper()
	pkg := filepath.Join(dir, "node_modules", Package)
	if err := os.MkdirAll(filepath.Join(pkg, "dist"), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"package.json": manifest, "dist/cli.js": ""} {
		if err := os.WriteFile(filepath.Join(pkg, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(pkg, "dist", "cli.js")
}

// pi is found as node finds a dependency, in a node_modules above the
// executable, its entry read from the manifest in either form npm allows.
func TestTheShippedPiIsFoundAboveTheExecutable(t *testing.T) {
	for _, manifest := range []string{`{"bin":{"pi":"dist/cli.js"}}`, `{"bin":"dist/cli.js"}`} {
		root := t.TempDir()
		entry := shippedPackage(t, root, manifest)
		below := filepath.Join(root, "node_modules", "@praline-labs", "rewake-linux-x64", "bin")
		if err := os.MkdirAll(below, 0o700); err != nil {
			t.Fatal(err)
		}
		if found, ok := findShipped(below); !ok || found != entry {
			t.Errorf("%s: found %q, %v", manifest, found, ok)
		}
	}
	root := t.TempDir()
	shippedPackage(t, root, `{"bin":{"other":"dist/cli.js"}}`)
	if found, ok := findShipped(root); ok {
		t.Errorf("a package naming no pi was taken: %s", found)
	}
}

// REWAKE_PI names the package or its entry for a build npm did not install.
func TestRewakePiNamesThePackageOrItsEntry(t *testing.T) {
	entry := shippedPackage(t, t.TempDir(), `{"bin":{"pi":"dist/cli.js"}}`)
	for _, named := range []string{filepath.Dir(filepath.Dir(entry)), entry} {
		t.Setenv(shippedEnv, named)
		if found, err := shippedEntry(); err != nil || found != entry {
			t.Errorf("%s: found %q, %v", named, found, err)
		}
	}
	t.Setenv(shippedEnv, filepath.Join(t.TempDir(), "absent"))
	if _, err := shippedEntry(); err == nil || !strings.Contains(err.Error(), shippedEnv) {
		t.Errorf("a missing pi: %v", err)
	}
}

func launchRequest(t *testing.T) adapter.LaunchRequest {
	t.Helper()
	dir, err := os.MkdirTemp("", "rw")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	t.Setenv("HOME", dir)
	// A model or effort default of the person running the test would add a note.
	t.Setenv("REWAKE_PI_MODEL", "")
	t.Setenv("REWAKE_PI_EFFORT", "")
	return adapter.LaunchRequest{
		Name: "api", Dir: dir, RoomDir: dir, Room: "default", Epoch: "1-1",
		Socket: filepath.Join(dir, "api.sock"), RunDir: dir, Version: PinnedVersion,
		Start: adapter.Start{Command: "/usr/bin/node", Args: []string{"/shipped/cli.js"}},
		Env:   []string{"PATH=/usr/bin", agentDirEnv + "=/home/someone/.pi/agent", skipVersionCheckEnv + "=0"},
	}
}

// The shipped pi starts under node with rewake's extension added to those pi
// finds, and keeps everything in rewake's own private agent directory,
// whatever the person's environment names.
func TestPiRunsInRewakesOwnAgentDirectory(t *testing.T) {
	request := launchRequest(t)
	plan, err := harness{}.Launch(request)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Command != "/usr/bin/node" || len(plan.Args) < 3 || plan.Args[0] != "/shipped/cli.js" || plan.Args[1] != "-e" {
		t.Errorf("started %s %q", plan.Command, plan.Args)
	}
	for _, off := range []string{"-ne", "-na", "--no-extensions", "--no-approve"} {
		if slices.Contains(plan.Args, off) {
			t.Errorf("%s turns off what the person installed: %q", off, plan.Args)
		}
	}
	dir := filepath.Join(request.Dir, ".config", "rewake", "pi", "agent")
	var named []string
	for _, value := range plan.Env {
		if strings.HasPrefix(value, agentDirEnv+"=") || strings.HasPrefix(value, skipVersionCheckEnv+"=") {
			named = append(named, value)
		}
	}
	if want := []string{agentDirEnv + "=" + dir, skipVersionCheckEnv + "=1"}; !slices.Equal(named, want) {
		t.Errorf("pi's environment names %q, want %q", named, want)
	}
	if info, err := os.Stat(dir); err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("the agent directory: %v, %v", info, err)
	}
	if len(plan.Notes) != 0 {
		t.Errorf("the pinned pi got notes: %q", plan.Notes)
	}
}

// A pi named with --command replaces node and the shipped entry, and one not
// at the pinned version starts with a note.
func TestAPiNamedWithCommandStartsWithANote(t *testing.T) {
	request := launchRequest(t)
	request.Command, request.Version = "/opt/pi/bin/pi", "1.2.0"
	plan, err := harness{}.Launch(request)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Command != "/opt/pi/bin/pi" || slices.Contains(plan.Args, "/shipped/cli.js") {
		t.Errorf("started %s %q", plan.Command, plan.Args)
	}
	if len(plan.Notes) != 1 || !strings.Contains(plan.Notes[0], "1.2.0") || !strings.Contains(plan.Notes[0], PinnedVersion) {
		t.Errorf("the notes: %q", plan.Notes)
	}
}
