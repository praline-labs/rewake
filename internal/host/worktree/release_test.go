package worktree

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// A worker's checkout goes at its close, with its branch, only when nothing
// would be lost: untouched, or with every commit already on another local
// branch; files git ignores go with it, unlike rm. A commit only its branch
// holds, a change or a lock keeps it, and its branch, with the reason.
func TestReleaseRemovesOnlyWhatLosesNothing(t *testing.T) {
	isolate(t)
	t.Setenv(state.DirEnv, t.TempDir())
	source := repository(t, "project")
	write(t, filepath.Join(source, ".gitignore"), ".env\nnode_modules/\n")
	must(t, source, "add", ".gitignore")
	must(t, source, "commit", "-q", "-m", "Ignore")
	root := t.TempDir()

	commit := func(record Record, message string) {
		must(t, record.Path, "commit", "-q", "--allow-empty", "-m", message)
	}
	cases := []struct {
		name  string
		work  func(Record)
		stays string
	}{
		{"untouched", func(Record) {}, ""},
		{"merged", func(r Record) {
			commit(r, "Merged")
			must(t, source, "merge", "-q", "--ff-only", r.Branch)
		}, ""},
		{"landed-elsewhere", func(r Record) {
			commit(r, "Kept by another branch")
			must(t, source, "branch", "elsewhere", r.Branch)
		}, ""},
		{"unmerged", func(r Record) { commit(r, "Unmerged") }, "on no local branch but its own"},
		{"tagged-only", func(r Record) {
			commit(r, "Tagged")
			must(t, source, "tag", "only-a-tag", r.Branch)
		}, "on no local branch but its own"},
		{"left-its-branch", func(r Record) {
			commit(r, "Left behind")
			must(t, r.Path, "switch", "-q", "--detach", "main")
		}, "holds commits no other local branch has"},
		{"uncommitted", func(r Record) { write(t, filepath.Join(r.Path, "draft"), "draft\n") }, "changes git status shows"},
		{"ignored", func(r Record) {
			write(t, filepath.Join(r.Path, ".env"), "SECRET=1\n")
			write(t, filepath.Join(r.Path, "node_modules", "pkg", "index.js"), "built\n")
		}, ""},
		{"ignored-beside-a-change", func(r Record) {
			write(t, filepath.Join(r.Path, ".env"), "SECRET=1\n")
			write(t, filepath.Join(r.Path, "draft"), "draft\n")
		}, "changes git status shows"},
		{"assume-unchanged", func(r Record) {
			must(t, r.Path, "update-index", "--assume-unchanged", "src/nested/file")
			write(t, filepath.Join(r.Path, "src", "nested", "file"), "local work\n")
		}, "assume-unchanged or skip-worktree"},
		{"skip-worktree", func(r Record) {
			must(t, r.Path, "update-index", "--skip-worktree", "src/nested/file")
			write(t, filepath.Join(r.Path, "src", "nested", "file"), "local work\n")
		}, "assume-unchanged or skip-worktree"},
		{"locked", func(r Record) { must(t, source, "worktree", "lock", "--reason", "mine", r.Path) }, "locked with git worktree lock (mine)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			record, err := Create(root, source, c.name)
			if err != nil {
				t.Fatal(err)
			}
			c.work(record)
			stays, _ := Release(record)
			_, statErr := os.Stat(record.Path)
			branch, _ := refTip(record.CommonDir, branchRef(record.Branch))
			switch {
			case c.stays == "" && stays != "":
				t.Fatalf("kept: %s", stays)
			case c.stays == "" && (!os.IsNotExist(statErr) || branch != ""):
				t.Fatalf("released, yet the checkout (%v) or the branch (%q) is there", statErr, branch)
			case c.stays != "" && !strings.Contains(stays, c.stays):
				t.Fatalf("kept for %q, want %q", stays, c.stays)
			case c.stays != "" && (statErr != nil || branch == ""):
				t.Fatalf("kept, yet the checkout (%v) or the branch (%q) is gone", statErr, branch)
			}
		})
	}
}

// The branch a worker switched its checkout to goes with it by DropBranch's
// rule: when another ref holds its tip. One that holds work nothing else has
// stays — the checkout may go, since its commits are on that branch.
func TestReleaseDropsTheBranchTheCheckoutSwitchedTo(t *testing.T) {
	isolate(t)
	t.Setenv(state.DirEnv, t.TempDir())
	source := repository(t, "project")
	root := t.TempDir()
	for _, landed := range []bool{true, false} {
		name := "unlanded"
		if landed {
			name = "landed"
		}
		t.Run(name, func(t *testing.T) {
			record, err := Create(root, source, name)
			if err != nil {
				t.Fatal(err)
			}
			own := "own-" + name
			must(t, record.Path, "switch", "-q", "-c", own)
			must(t, record.Path, "commit", "-q", "--allow-empty", "-m", "Work")
			if landed {
				must(t, source, "merge", "-q", "--ff-only", own)
			}
			if stays, _ := Release(record); stays != "" {
				t.Fatalf("kept: %s", stays)
			}
			if tip, _ := refTip(record.CommonDir, branchRef(record.Branch)); tip != "" {
				t.Fatalf("the record's branch stays at %s", tip)
			}
			tip, _ := refTip(record.CommonDir, branchRef(own))
			if landed && tip != "" || !landed && tip == "" {
				t.Fatalf("landed %v, yet the switched branch is %q", landed, tip)
			}
		})
	}
}
