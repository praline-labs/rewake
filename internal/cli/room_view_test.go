package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/core/session"
	"github.com/praline-labs/rewake/internal/core/taskbrief"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// A list that brief list cannot read is a failure of the view's briefs, with
// its reason, never an empty list: the shell shows it as the list's line.
func TestTheRoomViewCarriesAFailedBriefList(t *testing.T) {
	dir, _ := briefWorld(t)
	lead := otherRun(t, dir, "lead")
	markMain(t, dir, "lead")
	as(t, lead)
	shell := os.Getpid()
	shellParent = func() int { return shell }
	t.Cleanup(func() { shellParent = os.Getppid })
	if code, _, errOut := run("brief", "new", "# Real brief\n"); code != ExitOK {
		t.Fatalf("new: %d %s", code, errOut)
	}
	if err := state.WriteAtomic(filepath.Join(state.BriefsPath(dir), "index.json"), []byte("not json")); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := run("brief", "list", "--closed", "--json"); code == ExitOK {
		t.Fatal("brief list reads what the view must report")
	}
	view := roomView(t, "--wrapper", strconv.Itoa(lead.ServicePID), "--briefs")
	if !view.AsMain || view.BriefsProblem == nil || *view.BriefsProblem == "" || len(view.Briefs) != 0 {
		t.Fatalf("the failed list: %+v", view)
	}
}

// roomView is a room view's answer, read the way the shell reads it.
func roomView(t *testing.T, args ...string) RoomView {
	t.Helper()
	code, out, errOut := run(append([]string{"room", "--json"}, args...)...)
	if code != ExitOK {
		t.Fatalf("room: %d %q %q", code, out, errOut)
	}
	var view RoomView
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	return view
}

// The shell proves itself main's by the process tree, and only then does the
// view carry what only main reads: the sessions' telemetry and the briefs.
// Without the proof the spaces and the sessions still come, bare.
func TestTheRoomViewCarriesWhatOnlyMainReadsToMainsShell(t *testing.T) {
	dir, worker := briefWorld(t)
	lead := otherRun(t, dir, "lead")
	markMain(t, dir, "lead")
	as(t, lead)
	saveState(t, dir, lead.Name, lead.Epoch(), 1)
	saveState(t, dir, worker.Name, worker.Epoch(), 2)
	openSpace(t, dir, "api", "100.1700.boot")
	if code, _, errOut := run("brief", "new", "# Auth refactor\n\n- [ ] move the login\n"); code != ExitOK {
		t.Fatalf("new: %d %s", code, errOut)
	}
	run("brief", "new", "# Old work\n")
	if code, _, errOut := run("brief", "close", "2"); code != ExitOK {
		t.Fatalf("close: %d %s", code, errOut)
	}
	if err := taskbrief.Bind(taskbrief.Home{Room: dir}, 1, taskbrief.Author{Name: worker.Name, Epoch: worker.Epoch()}, taskbrief.Work); err != nil {
		t.Fatal(err)
	}
	shell := os.Getpid()
	shellParent = func() int { return shell }
	t.Cleanup(func() { shellParent = os.Getppid })

	bare := roomView(t, "--briefs", "--brief", "1")
	if bare.AsMain || bare.Briefs != nil || bare.Brief != nil || len(bare.Sessions) != 2 || len(bare.Spaces) != 1 {
		t.Fatalf("without a proof: %+v", bare)
	}
	for _, row := range bare.Sessions {
		if row.Verdict != nil || row.Own != nil || row.Work != nil {
			t.Errorf("%s carries telemetry to a caller that proved nothing: %+v", row.Name, row)
		}
	}
	if space := bare.Spaces[0]; space.Name != "api" || space.State != SpaceStateOpen || space.Socket == "" || space.Worker != "api-claude" {
		t.Fatalf("the space: %+v", space)
	}

	view := roomView(t, "--wrapper", strconv.Itoa(lead.ServicePID), "--briefs", "--brief", "1")
	if !view.AsMain {
		t.Fatalf("main's shell was not proved: %+v", view)
	}
	for _, row := range view.Sessions {
		switch row.Name {
		case "lead":
			if row.Own == nil || !*row.Own || row.Work != nil {
				t.Errorf("main's own row is not marked: %+v", row)
			}
		case "worker":
			if row.Verdict == nil || row.Verdict.Label != "working" || row.Verdict.Mark != MarkSteady || row.Model == nil || row.Percent == nil || *row.Percent != 42 || row.Work == nil || *row.Work != "brief 1 0/1" {
				t.Errorf("the worker's row: %+v", row)
			}
		}
	}
	if len(view.Briefs) != 2 || view.Briefs[0].Text != nil || len(view.UnreadableBriefs) != 0 {
		t.Fatalf("the briefs: %+v", view.Briefs)
	}
	opened := view.Brief
	if opened == nil || opened.ID != 1 || opened.Text == nil || !strings.Contains(*opened.Text, "move the login") || len(opened.Bound) != 1 || opened.Bound[0].Run != RunStateRunning {
		t.Fatalf("the opened brief: %+v", opened)
	}

	// A pid that is not main's wrapper, or that the shell did not start,
	// proves nothing.
	if roomView(t, "--wrapper", strconv.Itoa(worker.ServicePID)).AsMain {
		t.Error("a worker's pid proved main's shell")
	}
	shellParent = func() int { return shell + 1 }
	if roomView(t, "--wrapper", strconv.Itoa(lead.ServicePID)).AsMain {
		t.Error("a wrapper another process started proved main's shell")
	}
}

// The view's marks are the ones the verdict emits: the schema's enum and
// session's constants say the same three words.
func TestTheSchemaNamesTheMarksTheVerdictEmits(t *testing.T) {
	for want, got := range map[string]Mark{session.MarkOff: MarkOff, session.MarkSteady: MarkSteady, session.MarkBlink: MarkBlink} {
		if want != string(got) {
			t.Errorf("session says %q, the schema %q", want, got)
		}
	}
}

// The branch is read from HEAD itself: a branch, a linked worktree's file
// that points to its git directory, a detached head's short commit, nothing
// outside a repository.
func TestTheBranchIsReadFromHead(t *testing.T) {
	write := func(path, text string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	root := t.TempDir()
	write(filepath.Join(root, "repo", ".git", "HEAD"), "ref: refs/heads/feature/x\n")
	write(filepath.Join(root, "repo", "sub", "keep"), "")
	write(filepath.Join(root, "main", ".git", "worktrees", "wt", "HEAD"), "0123456789abcdef0123456789abcdef01234567\n")
	write(filepath.Join(root, "wt", ".git"), "gitdir: "+filepath.Join(root, "main", ".git", "worktrees", "wt")+"\n")
	for dir, want := range map[string]string{
		filepath.Join(root, "repo"):        "feature/x",
		filepath.Join(root, "repo", "sub"): "feature/x",
		filepath.Join(root, "wt"):          "0123456",
		filepath.Join(root, "nowhere"):     "",
	} {
		// The temp directory may lie below a repository of its own: only a
		// directory with none above it says nothing.
		if want == "" && branchAt(dir) != "" {
			continue
		}
		if got := branchAt(dir); got != want {
			t.Errorf("%s: %q, want %q", dir, got, want)
		}
	}
}
