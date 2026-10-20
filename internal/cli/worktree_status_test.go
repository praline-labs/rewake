package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/space"
	"github.com/praline-labs/rewake/internal/host/worktree"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// statusRun is a live run of the calling session, whose work is in the
// checkout it claimed, and, for a worker, the open space that hosts it.
type statusRun struct {
	dir     string
	session registry.Session
	record  worktree.Record
}

func asStatusRun(t *testing.T, lab worktreeLab, checkout, worker bool) statusRun {
	t.Helper()
	dir, err := state.Dir()
	if err != nil {
		t.Fatal(err)
	}
	name := "api-" + agentID
	run := statusRun{dir: dir, session: otherSession(t, dir, name)}
	if checkout {
		run.record = lab.made(t, "api")
		run.session.CWD = run.record.Path
		run.record, err = worktree.Claim(run.record, worktree.Owner{Name: name, Room: filepath.Base(dir), Epoch: run.session.Epoch(), Harness: agentID, Dir: dir})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := registry.Publish(dir, run.session); err != nil {
		t.Fatal(err)
	}
	t.Setenv(state.SessionEnv, name)
	t.Setenv(state.EpochEnv, run.session.Epoch())
	if worker {
		id, err := space.NewID()
		if err != nil {
			t.Fatal(err)
		}
		if err := space.Create(dir, space.Record{
			ID: id, Name: "api", Main: space.Run{Name: "main-" + agentID, Run: "1.2.boot"},
			Worker: space.Worker{Harness: agentID, Name: "api", Role: "write"}, OpenedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := space.SetHost(dir, id, "6161.1750.boot"); err != nil {
			t.Fatal(err)
		}
		if _, err := space.Opened(dir, id, run.session.Epoch()); err != nil {
			t.Fatal(err)
		}
	}
	return run
}

func readStatus(t *testing.T) worktreeStatus {
	t.Helper()
	code, out, errOut := run("worktree", "status", "--json")
	var status worktreeStatus
	if code != ExitOK || json.Unmarshal([]byte(out), &status) != nil {
		t.Fatalf("status: %d %s %s", code, out, errOut)
	}
	return status
}

// What a worker's close would do if it closed now is Release's policy, with
// the worker's own live run not counted: a clean or landed checkout would go,
// one with commits no other local branch holds, changes, or another session
// working there would stay, and files git ignores go with it.
func TestWorktreeStatusForecastsTheClose(t *testing.T) {
	cases := []struct {
		name string
		work func(t *testing.T, lab worktreeLab, r statusRun)
		keep string
	}{
		{"untouched", func(*testing.T, worktreeLab, statusRun) {}, ""},
		{"landed", func(t *testing.T, lab worktreeLab, r statusRun) {
			lab.git(t, r.record.Path, "commit", "-q", "--allow-empty", "-m", "Work")
			lab.git(t, lab.repo, "merge", "-q", "--ff-only", r.record.Branch)
		}, ""},
		{"ignored", func(t *testing.T, lab worktreeLab, r statusRun) {
			writeFile(t, filepath.Join(lab.repo, ".git", "info", "exclude"), ".env\n")
			writeFile(t, filepath.Join(r.record.Path, ".env"), "TOKEN=x\n")
		}, ""},
		{"unlanded", func(t *testing.T, lab worktreeLab, r statusRun) {
			lab.git(t, r.record.Path, "commit", "-q", "--allow-empty", "-m", "Work")
		}, "on no local branch but its own"},
		{"changed", func(t *testing.T, _ worktreeLab, r statusRun) {
			writeFile(t, filepath.Join(r.record.Path, "draft"), "draft\n")
		}, "changes git status shows"},
		{"visited", func(t *testing.T, lab worktreeLab, r statusRun) {
			runningAt(t, launched(t, lab.state, "elsewhere"), "visitor", r.record.Path)
		}, "visitor in room elsewhere"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lab := newWorktreeLab(t)
			r := asStatusRun(t, lab, true, true)
			c.work(t, lab, r)
			before := snapshotTree(t, lab.root) + snapshotTree(t, lab.state)
			status := readStatus(t)
			if status.Space != "api" || status.Checkout == nil || status.Checkout.Path != r.record.Path || status.Checkout.Branch != r.record.Branch || status.Checkout.Base != r.record.Commit {
				t.Fatalf("status: %+v %+v", status, status.Checkout)
			}
			reasons := strings.Join(status.Close.Reasons, "; ")
			switch {
			case c.keep == "" && status.Close.Outcome != closeRemoves:
				t.Fatalf("forecast %s: %s", status.Close.Outcome, reasons)
			case c.keep != "" && (status.Close.Outcome != closeKeeps || !strings.Contains(reasons, c.keep)):
				t.Fatalf("forecast %s: %s; want kept for %q", status.Close.Outcome, reasons, c.keep)
			case strings.Contains(reasons, "api-"+agentID):
				t.Fatalf("the worker's own run counted: %s", reasons)
			}
			if after := snapshotTree(t, lab.root) + snapshotTree(t, lab.state); after != before {
				t.Fatal("status wrote")
			}
			if _, err := os.Stat(r.record.Path); err != nil {
				t.Fatalf("the forecast touched the checkout: %v", err)
			}
		})
	}
}

// No close removes main's checkout, nor anything of a worker without one;
// the answer says so rather than forecast. Outside a session there is nobody
// to answer for.
func TestWorktreeStatusSaysWhenNoCloseApplies(t *testing.T) {
	lab := newWorktreeLab(t)
	asStatusRun(t, lab, true, false)
	if status := readStatus(t); status.Checkout == nil || status.Space != "" || status.Close.Outcome != closeNone || !strings.Contains(status.Close.Note, "not a space's worker") {
		t.Fatalf("main's checkout: %+v", status)
	}
	lab = newWorktreeLab(t)
	asStatusRun(t, lab, false, true)
	status := readStatus(t)
	if status.Checkout != nil || status.Space != "api" || status.Close.Outcome != closeNone {
		t.Fatalf("a worker without a checkout: %+v", status)
	}
	if _, out, _ := run("worktree", "status"); !strings.Contains(out, "checkout: none") {
		t.Fatalf("lines: %s", out)
	}
	t.Setenv(state.SessionEnv, "")
	if code, _, errOut := run("worktree", "status"); code != ExitUsage || !strings.Contains(errOut, "runs in none") {
		t.Fatalf("outside a session: %d %s", code, errOut)
	}
}

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}
