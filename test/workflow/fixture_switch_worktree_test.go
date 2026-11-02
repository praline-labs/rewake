package workflow

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestFixtureSwitchWorktree is a switch of the room's build while workers
// launched with --worktree run (docs/rules/state.md#switching-a-running-room),
// the room the owner's first live switch met, November 2, 2026: every writer
// works in a checkout its launch made, and each such run answered cannot.
//
//   - Two workers launched with --worktree hand over: each wrapper keeps its
//     pid and epoch and runs the new build, and the room's build is the new
//     one.
//   - Their checkouts are settled at the adopted run's end as a launch
//     settles them: one whose harness exits cleanly stays, and stderr says
//     where; one whose harness exits with an error and holds nothing new is
//     taken back, its record and its branch with it.
//
// What it does not prove: a checkout entered through worktree_enter, which no
// launch settles and which its space's close releases by the run's epoch, the
// same across the exec (fixture_space_worktree_enter_test.go holds the close).
func TestFixtureSwitchWorktree(t *testing.T) {
	binary := enterScenario(t, "fixture-switch-worktree")
	c := Start(t, Spec{Name: "fixture-switch-worktree", Harness: fixtureColumn.harness, Observations: []string{obsSwitchTrees, obsSwitchTreeStays, obsSwitchTreeBack}, Deadline: 120 * time.Second})
	iso := Isolate(t, c, binary)
	other, err := buildRewakeWith(t.TempDir(), "-X github.com/praline-labs/rewake/internal/cli.built="+switchBuilt)
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	newer, err := os.ReadFile(other)
	if err != nil {
		t.Fatal(err)
	}
	installed := filepath.Join(iso.Home, "install", "rewake")
	if err := os.MkdirAll(filepath.Dir(installed), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := replaceBinary(installed, original); err != nil {
		t.Fatal(err)
	}
	iso.binary = installed
	room := filepath.Join(iso.StateDir, "v2", "rooms", iso.HomeRoom())
	repo := switchRepository(t, c, iso)
	rewake := func(args ...string) (string, error) {
		out, err := c.OutputAllowingFailure(iso.Command(args...))
		return string(out), err
	}

	asks := newRequests(iso, "lead")
	lead := startHarnessSession(t, c, iso, fixtureColumn.harness, "lead", "--main", asks.env(), shimInboxJSON+"=1")
	defer stopSession(t, c, lead)
	stays := startSessionIn(t, c, iso, repo, fixtureColumn.harness, "stays", "--general", nil, []string{"--worktree=stays"}, staysUp(iso, "stays"))
	// Its harness exits 1: a failure the case expects.
	back, launch := sessionLaunch(t, c, iso, repo, fixtureColumn.harness, "back", "--general", nil, []string{"--worktree=back"}, staysUp(iso, "back"))
	release := c.captureStderr(launch, iso.Home, "back")
	back.process, err = c.start(launch, true)
	release()
	if err != nil {
		t.Fatalf("launching the session: %v", err)
	}
	back.home = iso.Home
	trees := map[string]treeView{}
	owned := sessionsUp(c, lead, stays, back) == nil && waitFor(c, 30*time.Second, func() bool {
		for _, tree := range listTreesFrom(rewake) {
			if tree.Session != nil {
				trees[tree.Name] = tree
			}
		}
		return len(trees) == 2
	})
	if !owned {
		stopSession(t, c, stays)
		stopSession(t, c, back)
		c.Contradicted(obsSwitchTrees, "the workers never came up in checkouts they own: %+v", trees)
		return
	}
	before := map[string]reexecRecord{stays.name: reexecRecordOf(iso, stays.name), back.name: reexecRecordOf(iso, back.name)}

	if err := replaceBinary(installed, newer); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := switchRoom(c, iso)
	var moved []string
	for name, was := range before {
		now := reexecRecordOf(iso, name)
		exe, _ := os.ReadFile(fmt.Sprintf("/proc/%d/exe", was.ServicePID))
		if now.epoch() != was.epoch() || idOf(exe) != idOf(newer) {
			moved = append(moved, fmt.Sprintf("%s: epoch %s, was %s; runs %.12s", name, now.epoch(), was.epoch(), idOf(exe)))
		}
	}
	switch {
	case code != 0:
		c.Contradicted(obsSwitchTrees, "restart rewake exited %d: %s %s", code, out, errOut)
	case len(moved) > 0 || recordOf(room) != idOf(newer):
		c.Contradicted(obsSwitchTrees, "the room's build %.12s, B is %.12s; %s", recordOf(room), idOf(newer), strings.Join(moved, "; "))
	default:
		c.Observed(obsSwitchTrees, firstLine(out))
	}

	stopSession(t, c, stays)
	said, _ := os.ReadFile(filepath.Join(iso.Home, "stays.stderr"))
	_, there := os.Stat(trees["stays"].Path)
	if there != nil || !strings.Contains(string(said), "stays at "+trees["stays"].Path+" on the branch stays") {
		c.Contradicted(obsSwitchTreeStays, "checkout there: %v; stderr:\n%s", there, said)
	} else {
		c.Observed(obsSwitchTreeStays, "the adopted run's end said where its checkout stays")
	}

	if err := os.WriteFile(back.exitFile, []byte("1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ended := waitFor(c, 20*time.Second, func() bool { return back.process.finished() && !back.process.groupAlive() })
	said, _ = os.ReadFile(filepath.Join(iso.Home, "back.stderr"))
	_, gone := os.Stat(trees["back"].Path)
	branch, _ := switchGit(c, iso, repo, "branch", "--list", "back")
	listed := false
	for _, tree := range listTreesFrom(rewake) {
		listed = listed || tree.Name == "back"
	}
	if !ended || !os.IsNotExist(gone) || listed || branch != "" || strings.Contains(string(said), " stays at ") {
		c.Contradicted(obsSwitchTreeBack, "ended %v, checkout gone %v, record left %v, branch %q; stderr:\n%s", ended, os.IsNotExist(gone), listed, branch, said)
	} else {
		c.Observed(obsSwitchTreeBack, "the failed adopted run's checkout, holding nothing new, was taken back with its branch")
	}
	clearSwitchSocket(t, room)
}

const (
	obsSwitchTrees     = "a switch while two workers launched with --worktree run goes through: each wrapper keeps its pid and epoch and runs the new build"
	obsSwitchTreeStays = "after the switch, a worker whose harness exits cleanly leaves its checkout, and its stderr says where it stays"
	obsSwitchTreeBack  = "after the switch, a worker whose harness exits with an error has its checkout, holding nothing new, taken back with its record and branch, and says nothing of it staying"
)

// switchRepository makes a repository of one commit in the case's home.
func switchRepository(t *testing.T, c *Case, iso *Isolation) string {
	t.Helper()
	repo := filepath.Join(iso.Home, "project")
	if err := os.MkdirAll(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "file"), []byte("one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "."}, {"commit", "-q", "-m", "First"}} {
		if _, err := switchGit(c, iso, repo, args...); err != nil {
			t.Fatalf("preparing the repository: git %q: %v", args, err)
		}
	}
	return repo
}

func switchGit(c *Case, iso *Isolation, dir string, args ...string) (string, error) {
	command := exec.Command("git", append([]string{"-C", dir}, args...)...)
	command.Env = append(iso.Env(), "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.invalid",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.invalid")
	out, err := c.OutputAllowingFailure(command)
	return strings.TrimSpace(string(out)), err
}
