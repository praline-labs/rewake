package pi

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/role"
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
		Env:   []string{"PATH=/usr/bin", AgentDirEnv + "=/home/someone/.pi/agent", SkipVersionCheckEnv + "=0"},
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
		if strings.HasPrefix(value, AgentDirEnv+"=") || strings.HasPrefix(value, SkipVersionCheckEnv+"=") {
			named = append(named, value)
		}
	}
	if want := []string{AgentDirEnv + "=" + dir, SkipVersionCheckEnv + "=1"}; !slices.Equal(named, want) {
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

// The version read gets the run's agent directory without making it: pi
// reads its settings before it answers, and nothing is made before the claim.
func TestTheVersionReadUsesRewakesAgentDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".config", "rewake", "pi", "agent")
	if env := (harness{}).Version().Env; !slices.Equal(env, []string{AgentDirEnv + "=" + dir, SkipVersionCheckEnv + "=1"}) {
		t.Errorf("the version read's environment: %q", env)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("the version gate made the directory: %v", err)
	}
}

// A directory made before the first launch, wider than private, is made
// private: it holds the login.
func TestAnExistingAgentDirectoryIsMadePrivate(t *testing.T) {
	request := launchRequest(t)
	dir := filepath.Join(request.Dir, ".config", "rewake", "pi", "agent")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := (harness{}).Launch(request); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(dir); err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("the agent directory: %v, %v", info, err)
	}
}

// The folder a person adds extensions to is made on a launch, skills/ is
// not; a skills/ the person keeps, a skills installer's link included, stays.
func TestTheAgentDirectoryGetsExtensionsButNoSkillsFolder(t *testing.T) {
	request := launchRequest(t)
	dir := filepath.Join(request.Dir, ".config", "rewake", "pi", "agent")
	if _, err := (harness{}).Launch(request); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(filepath.Join(dir, "extensions")); err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Errorf("extensions: %v, %v", info, err)
	}
	skills := filepath.Join(dir, "skills")
	if _, err := os.Stat(skills); !os.IsNotExist(err) {
		t.Errorf("skills/ was made: %v", err)
	}
	if err := os.MkdirAll(skills, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(skills, "mine")
	if err := os.Symlink(request.Dir, link); err != nil {
		t.Fatal(err)
	}
	if _, err := (harness{}).Launch(request); err != nil {
		t.Fatal(err)
	}
	if got, err := os.Readlink(link); err != nil || got != request.Dir {
		t.Errorf("the link in skills/: %q, %v", got, err)
	}
}

// A project with its own resources would hold the run on pi's trust prompt
// until a person answered; the launch trusts it unless its arguments decide.
func TestPiTrustsTheProjectUnlessTheLaunchDecides(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want []string
	}{
		{nil, []string{"--approve"}},
		{[]string{"-na"}, []string{"-na"}},
		{[]string{"--no-approve"}, []string{"--no-approve"}},
		{[]string{"-a"}, []string{"-a"}},
		{[]string{"--", "-na"}, []string{"--approve", "-na"}},
	} {
		request := launchRequest(t)
		request.Args = tc.args
		plan, err := harness{}.Launch(request)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, arg := range plan.Args {
			if slices.Contains([]string{"--approve", "-a", "--no-approve", "-na"}, arg) {
				got = append(got, arg)
			}
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("launch %q: trust flags %q, want %q", tc.args, got, tc.want)
		}
		if terminator := slices.Index(plan.Args, "--"); terminator >= 0 && slices.Index(plan.Args, "--approve") > terminator {
			t.Errorf("launch %q: --approve after the terminator: %q", tc.args, plan.Args)
		}
	}
}

// A worker keeps its sessions in a folder of its own in the agent directory,
// whatever the person's environment names, and its own --session-dir is
// refused; main keeps pi's own place.
func TestAWorkersSessionsAreKeptApart(t *testing.T) {
	request := launchRequest(t)
	request.Role = role.Write
	request.Env = append(request.Env, SessionDirEnv+"=/home/someone/sessions")
	plan, err := harness{}.Launch(request)
	if err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(request.Dir, ".config", "rewake", "pi", "agent", workerSessions)
	var named []string
	for _, value := range plan.Env {
		if strings.HasPrefix(value, SessionDirEnv+"=") {
			named = append(named, value)
		}
	}
	if !slices.Equal(named, []string{SessionDirEnv + "=" + folder}) {
		t.Errorf("the worker's sessions: %q", named)
	}
	if info, err := os.Stat(folder); err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("the sessions folder: %v, %v", info, err)
	}
	request.Args = []string{sessionDirFlag + "=/x"}
	if _, err := (harness{}).Launch(request); err == nil || !strings.Contains(err.Error(), workerSessions) {
		t.Errorf("a worker's own %s: %v", sessionDirFlag, err)
	}
	// Main's environment reaches the adapter through the host's
	// launch.SessionEnv, which drops every EnvNotInherited name first (the
	// review of October 23, 2026 found main's choice lost there).
	request.Role, request.Args = role.Main, nil
	request.Env = slices.DeleteFunc(request.Env, func(value string) bool {
		name, _, _ := strings.Cut(value, "=")
		return slices.Contains(harness{}.EnvNotInherited(), name)
	})
	plan, err = harness{}.Launch(request)
	if err != nil || !slices.Contains(plan.Env, SessionDirEnv+"=/home/someone/sessions") {
		t.Errorf("main's environment: %q, %v", plan.Env, err)
	}
}
