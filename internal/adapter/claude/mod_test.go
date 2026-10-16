package claude

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// notShipped are the files of the mod's source no run loads: its tests, run
// by the harness's test kit, and the schema the generated types come from.
func notShipped(name string) bool {
	return name == "mod/protocol.schema.json" || strings.HasPrefix(name, "mod/tests/")
}

// What a run loads is the source as it stands: every file of the mod's
// directory is embedded byte for byte, or named as one no run loads, so a
// file added to the mod cannot be left out of the binary unnoticed.
func TestTheEmbeddedModIsItsSource(t *testing.T) {
	var onDisk []string
	err := filepath.WalkDir("mod", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		name = filepath.ToSlash(name)
		onDisk = append(onDisk, name)
		embedded, embedErr := modFiles.ReadFile(name)
		if notShipped(name) {
			if embedErr == nil {
				t.Errorf("%s is embedded and named as not shipped", name)
			}
			return nil
		}
		if embedErr != nil {
			t.Errorf("%s is in the mod and not embedded", name)
			return nil
		}
		source, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		if !bytes.Equal(source, embedded) {
			t.Errorf("%s differs from its embedded copy", name)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = fs.WalkDir(modFiles, "mod", func(name string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && !slices.Contains(onDisk, name) {
			t.Errorf("%s is embedded and not in the mod's source", name)
		}
		return err
	})
}

// claudeProgram is the installed harness the validator and the test kit come
// with, or a skip that says why the check did not run.
func claudeProgram(t *testing.T) string {
	t.Helper()
	program, err := exec.LookPath("claude")
	if err != nil {
		t.Skip("the harness's validator is not installed: claude is not in PATH")
	}
	return program
}

// The mod a run writes passes the harness's validator, strict: it is inside
// the static rules the engine loads a module by.
func TestTheWrittenModPassesTheValidator(t *testing.T) {
	program := claudeProgram(t)
	room := buildtest.Dir(t)
	dir := modDir(filepath.Join(room, "runs", "api.e1"))
	if err := writeMod(dir); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, program, "plugin", "validate", "--strict", dir).CombinedOutput()
	if err != nil || !regexp.MustCompile(`env reads: .*\b`+modSocketEnv+`\b`).Match(out) {
		t.Fatalf("the validator: %v\n%s", err, out)
	}
}

// The module's own tests pass in the harness's test kit.
func TestTheModsTestsPass(t *testing.T) {
	program := claudeProgram(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, program, "plugin", "test", "mod").CombinedOutput()
	if err != nil || !strings.Contains(string(out), " 0 fail") {
		t.Fatalf("the mod's tests: %v\n%s", err, out)
	}
}

// A launch with a run directory loads the mod from it for this launch alone,
// names its own mod socket in the harness's environment over any inherited,
// and serves it beside the lane; one without loads nothing.
func TestALaunchLoadsTheModOfItsRun(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "sock")
	request := adapter.LaunchRequest{
		Name: "api", Epoch: "e1", Dir: t.TempDir(), Socket: filepath.Join(sock, "api.e1.sock"),
		RunDir: filepath.Join(t.TempDir(), "api.e1"), Role: role.Main,
		Env: []string{"HOME=/home/someone", modSocketEnv + "=/elsewhere/parent.mod.sock"},
	}
	plan, err := New().Launch(request)
	if err != nil {
		t.Fatal(err)
	}
	if at := indexOf(plan.Args, pluginDirFlag); at < 0 || plan.Args[at+1] != modDir(request.RunDir) {
		t.Fatalf("the launch's plugin directory: %v", plan.Args)
	}
	var named []string
	for _, kv := range plan.Env {
		if value, ok := strings.CutPrefix(kv, modSocketEnv+"="); ok {
			named = append(named, value)
		}
	}
	if want := filepath.Join(sock, "api.e1.mod.sock"); !slices.Equal(named, []string{want}) {
		t.Fatalf("the mod socket in the environment: %v, want %s", named, want)
	}
	if _, ok := plan.Service.(adapter.HarnessWatcher); !ok {
		t.Fatalf("the run's service does not learn the harness's process: %T", plan.Service)
	}
	if plan.Wake == nil {
		t.Fatal("the run lost its wake")
	}
	request.RunDir = ""
	plan, err = New().Launch(request)
	if err != nil {
		t.Fatal(err)
	}
	if indexOf(plan.Args, pluginDirFlag) >= 0 {
		t.Fatalf("a launch with no run directory loads a mod: %v", plan.Args)
	}
	if !slices.Contains(New().EnvNotInherited(), modSocketEnv) {
		t.Fatal("a session started from a run inherits that run's mod socket")
	}
}

// A mod socket's path stays within a unix socket's limit, however long the
// inbox socket's beside it.
func TestTheModSocketFitsItsLimit(t *testing.T) {
	long := filepath.Join("/", strings.Repeat("d", 60), "sock", "a-session-name-of-some-length.e1.sock")
	got := modSocketPath(adapter.LaunchRequest{Name: "a-session-name-of-some-length", Epoch: "e1", Socket: long})
	if len(got) > maxSocketPath || filepath.Dir(got) != filepath.Dir(long) || !strings.HasSuffix(got, ".mod.sock") {
		t.Fatalf("a long path's mod socket: %s (%d bytes)", got, len(got))
	}
}

// A run's service writes the mod before anything starts and serves its
// socket until it is closed; the socket's file goes with it.
func TestARunWritesItsModAndServesItsSocket(t *testing.T) {
	room := buildtest.Dir(t)
	request := adapter.LaunchRequest{
		Name: "api", Epoch: "e1", Dir: room, RoomDir: room, Socket: filepath.Join(room, "sock", "api.e1.sock"),
		RunDir: filepath.Join(room, "runs", "api.e1"), Role: role.Main,
	}
	if err := state.EnsureSubdir(request.RunDir); err != nil {
		t.Fatal(err)
	}
	plan, err := New().Launch(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Service.Start(context.Background(), func(string) {}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".claude-plugin/plugin.json", "hooks/hooks.json", "hooks/rewake.ts", "hooks/outbox.ts", "protocol.gen.ts"} {
		if _, err := os.Stat(filepath.Join(modDir(request.RunDir), name)); err != nil {
			t.Errorf("the run's mod lacks %s: %v", name, err)
		}
	}
	socket := modSocketPath(request)
	if info, err := os.Stat(socket); err != nil || info.Mode()&os.ModeSocket == 0 {
		t.Fatalf("the mod socket: %v", err)
	}
	plan.Service.Close()
	if _, err := os.Stat(socket); !os.IsNotExist(err) {
		t.Fatalf("the mod socket outlived its run: %v", err)
	}
}
