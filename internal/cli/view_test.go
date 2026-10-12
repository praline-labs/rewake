package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/build"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// viewForms are the invocations a view must answer without a write
// (docs/v2/stage3-state.md#what-is-a-writer): every documented view form,
// every command's help page in both forms, and calls refused before they
// parse — an unknown command, an unknown flag, a stray argument — which have
// no form at all.
func viewForms(t *testing.T) [][]string {
	t.Helper()
	table := testTable()
	var forms [][]string
	for _, form := range documentedForms(table.catalog.IDs()[0]) {
		if form.want == accessView {
			forms = append(forms, form.argv)
		}
	}
	for _, group := range table.groups {
		for _, command := range group.Commands {
			forms = append(forms, []string{command.Name, "--help"}, []string{command.Name, "--help", "--json"})
		}
	}
	return append(forms, []string{"frobnicate"}, []string{"list", "--bogus"}, []string{"whoami", "extra"})
}

// TestEveryViewLeavesMissingAndExistingStateUnchanged holds the view
// guarantee: on a base nobody made, asked by no session and by one, and on a
// populated room — a live session, a dead record, a record that does not read
// — asked by its session while a launch holds the lease exclusive, every view
// form leaves the tree as it was, takes no lease and waits for none, and
// never meets a refused write.
func TestEveryViewLeavesMissingAndExistingStateUnchanged(t *testing.T) {
	forms := viewForms(t)
	check := func(t *testing.T, tree string) {
		t.Helper()
		for _, argv := range forms {
			before := snapshot(t, tree)
			started := time.Now()
			code, _, errOut := run(argv...)
			if elapsed := time.Since(started); elapsed > 5*time.Second {
				t.Errorf("%q waited %s", argv, elapsed)
			}
			if strings.Contains(errOut, "internal error") || strings.Contains(errOut, state.ErrViewWrite.Error()) {
				t.Errorf("%q tried to write: %d %s", argv, code, errOut)
			}
			if after := snapshot(t, tree); after != before {
				t.Fatalf("%q changed the state:\nbefore\n%s\nafter\n%s", argv, before, after)
			}
		}
	}

	t.Run("missing", func(t *testing.T) {
		parent := t.TempDir()
		t.Setenv(state.DirEnv, filepath.Join(parent, "state"))
		t.Setenv(state.RoomEnv, "")
		t.Setenv(state.SessionEnv, "")
		t.Setenv(epochEnv, "")
		check(t, parent)
		if code, out, _ := run("list"); code != ExitOK || !strings.Contains(out, "No sessions are running in room default") {
			t.Fatalf("list of a missing base: %d %q", code, out)
		}
		t.Setenv(state.SessionEnv, "api")
		t.Setenv(epochEnv, "1.1")
		check(t, parent)
	})

	t.Run("populated", func(t *testing.T) {
		dir := liveSession(t, "api")
		live, err := registry.Load(dir, "api")
		if err != nil {
			t.Fatal(err)
		}
		dead := live
		dead.Name, dead.ServiceStart = "gone", live.ServiceStart+1
		if err := registry.Publish(dir, dead); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(state.SessionPath(dir, "broken"), []byte("{not json"), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv(state.SessionEnv, "api")
		holder := holdExclusive(t, dir)
		defer holder()
		check(t, os.Getenv(state.DirEnv))
		// Asked by the dead session too: its record stays.
		t.Setenv(state.SessionEnv, "gone")
		t.Setenv(epochEnv, dead.Epoch())
		check(t, os.Getenv(state.DirEnv))
	})
}

// holdExclusive holds the room's build lease exclusive through a descriptor
// of its own, as a launch stalled while switching the build would: a view
// that asked for the lease would wait for it.
func holdExclusive(t *testing.T, room string) func() {
	t.Helper()
	file, err := os.OpenFile(filepath.Join(room, "build.lock"), os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	return func() { _ = file.Close() }
}

// TestForeignBuildViewsWarnOnce holds that a view of a room of another build
// says so once, on stderr, whatever the form — the guide, a help page,
// --version and --json among them — that an unreadable record is "unknown",
// never the same build, and that the room's own build and a room no launch
// made add nothing.
func TestForeignBuildViewsWarnOnce(t *testing.T) {
	dir := liveSession(t, "api")
	t.Setenv(state.SessionEnv, "api")
	record := filepath.Join(dir, "build")
	own, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	foreign := strings.Repeat("cd", 32)
	forms := [][]string{nil, {"--json"}, {"guide"}, {"--help"}, {"send", "--help"}, {"--version", "--json"}, {"list"}, {"list", "--json"}, {"whoami", "--json"}, {"inbox", "--owed"}}
	for _, world := range []struct {
		name, content, want string
	}{
		{"foreign", foreign + "\n", "belongs to build " + build.Short(foreign)},
		{"unreadable", "garbage\n", "is unknown"},
		{"own", string(own), ""},
	} {
		if err := os.WriteFile(record, []byte(world.content), 0o600); err != nil {
			t.Fatal(err)
		}
		for _, argv := range forms {
			_, out, errOut := run(argv...)
			lines := strings.Count(errOut, "Rewake: room default belongs to build") + strings.Count(errOut, "Rewake: the build of room default is unknown")
			switch {
			case world.want == "" && lines != 0:
				t.Errorf("%s %q: %q", world.name, argv, errOut)
			case world.want != "" && (lines != 1 || !strings.Contains(errOut, world.want)):
				t.Errorf("%s %q: %d lines in %q", world.name, argv, lines, errOut)
			case strings.Contains(out, "belongs to build"):
				t.Errorf("%s %q warned on stdout: %q", world.name, argv, out)
			}
		}
	}
	t.Setenv(state.RoomEnv, "empty")
	if _, _, errOut := run("list"); strings.Contains(errOut, "build") {
		t.Fatalf("a room without a build: %q", errOut)
	}
}

// TestViewWriteAttemptFailsEvenIfIgnored holds the mode's evidence: a view
// whose code meets the refusal of a write and swallows it still fails, says
// what it tried, and wrote nothing; a change in the same process afterwards
// writes, the mode left behind.
func TestViewWriteAttemptFailsEvenIfIgnored(t *testing.T) {
	var stdout, stderr strings.Builder
	ctx := &Context{Stdout: &stdout, Stderr: &stderr}
	t.Setenv(state.DirEnv, filepath.Join(t.TempDir(), "state"))
	path := filepath.Join(t.TempDir(), "written")
	code := ctx.within(accessView, Call{}, func() int {
		_ = state.WriteAtomic(path, []byte("x"))
		return ExitOK
	}, nil)
	if code != ExitFailed || !strings.Contains(stderr.String(), "a view does not write the state") || !strings.Contains(stderr.String(), path) {
		t.Fatalf("exit %d, %q", code, stderr.String())
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("the view wrote: %v", err)
	}
	if err := state.WriteAtomic(path, []byte("x")); err != nil {
		t.Fatalf("after the view: %v", err)
	}
}

// TestToolOutputContinuationHoldsLease holds D2's scope: a tool call whose
// answer is too long to print whole keeps the rest for continuing, a write
// after its handler returned, and that write — every write of the call — is
// made while the call holds its room's lease, which goes only after it.
func TestToolOutputContinuationHoldsLease(t *testing.T) {
	dir, _, _ := toolSession(t)
	caller := newToolCaller(t)
	var steps []string
	state.Fault = func(op, path string) error {
		switch op {
		case state.OpRead, state.OpStep:
		case state.OpNote:
			steps = append(steps, op+" "+path)
		default:
			steps = append(steps, fmt.Sprintf("%s %s leased=%v", op, strings.TrimPrefix(path, dir), state.Leased(dir)))
		}
		return nil
	}
	t.Cleanup(func() { state.Fault = nil })
	answer := caller.run("send", "--help", "--json")
	state.Fault = nil
	if answer.code != ExitOK || !strings.Contains(answer.out, "nextWords") {
		t.Fatalf("no continuation: %d %s %s", answer.code, answer.out, answer.errOut)
	}
	kept, released := false, false
	for _, step := range steps {
		switch {
		case step == state.OpNote+" lease release":
			released = true
		case released:
			t.Fatalf("%q after the lease went: %v", step, steps)
		case !strings.HasSuffix(step, "leased=true"):
			t.Fatalf("%q without the lease: %v", step, steps)
		case strings.Contains(step, "/receipts/"):
			kept = true
		}
	}
	if !kept || !released {
		t.Fatalf("the continuation kept %v, the lease released %v: %v", kept, released, steps)
	}
}
