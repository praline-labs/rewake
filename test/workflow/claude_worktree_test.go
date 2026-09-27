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

// TestClaudeWorktree is `rewake claude --worktree` end to end on the Claude
// column: the same checkout, branch and record a Codex launch gets. A worker
// launched from a directory inside a repository with --worktree=probe runs at
// that directory's place in a checkout on the new branch probe; the flag does
// not reach the harness, whose fixture refuses a flag it does not know. After
// the session ends, a commit made in the checkout is landed and the checkout
// removed by finish.
//
// What it does not prove: anything of Claude Code's own -w, which stays the
// harness's and is not started here.
func TestClaudeWorktree(t *testing.T) {
	binary := enterScenario(t, "claude-worktree")
	c := Start(t, Spec{
		Name:         "claude-worktree",
		Harness:      claudeColumn.harness,
		Observations: claudeWorktreeObservations,
		Deadline:     60 * time.Second,
	})
	for _, finding := range playClaudeWorktree(t, c, Isolate(t, c, binary)) {
		if finding.held {
			c.Observed(finding.observation, finding.detail)
		} else {
			c.Contradicted(finding.observation, "%s", finding.detail)
		}
	}
}

const (
	obsClaudeTreeEntered  = "a Claude Code launch with --worktree=probe runs in a checkout of HEAD on a new branch probe, at the launch directory's place in it: the session's record and the harness work there, the flag does not reach the harness, and the record names the session"
	obsClaudeTreeFinished = "after the session ends, finish lands the commit made in the checkout in the source's main, hash kept, and removes the checkout, its record and its branch"
)

var claudeWorktreeObservations = []string{obsClaudeTreeEntered, obsClaudeTreeFinished}

func playClaudeWorktree(t *testing.T, c *Case, iso *Isolation) []telemetryFinding {
	t.Helper()
	finding := func(observation string, held bool, detail string, args ...any) telemetryFinding {
		return telemetryFinding{observation: observation, held: held, judged: true, detail: fmt.Sprintf(detail, args...)}
	}
	repo := filepath.Join(iso.Home, "project")
	nested := filepath.Join(repo, "src", "nested")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "file"), []byte("one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git := func(dir string, args ...string) (string, error) {
		command := exec.Command("git", append([]string{"-C", dir}, args...)...)
		command.Env = append(iso.Env(), "GIT_CONFIG_NOSYSTEM=1",
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.invalid",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.invalid")
		out, err := c.OutputAllowingFailure(command)
		return strings.TrimSpace(string(out)), err
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "."}, {"commit", "-q", "-m", "First"}} {
		if _, err := git(repo, args...); err != nil {
			t.Fatalf("preparing the repository: git %q: %v", args, err)
		}
	}
	head, _ := git(repo, "rev-parse", "HEAD")
	rewake := func(args ...string) (string, error) {
		out, err := c.OutputAllowingFailure(iso.Command(args...))
		return string(out), err
	}
	cwdFile := filepath.Join(iso.Home, "claude.cwd")
	worker := startSessionIn(t, c, iso, nested, "claude", "tree", "--general", nil, []string{"--worktree=probe"}, shimCwdFile+"="+cwdFile)
	stopped := false
	defer func() {
		if !stopped {
			stopSession(t, c, worker)
		}
	}()

	var tree treeView
	var harnessCwd []byte
	if !waitFor(c, 30*time.Second, func() bool {
		found := listTreesFrom(rewake)
		harnessCwd, _ = os.ReadFile(cwdFile + ".claude")
		if len(found) != 1 || len(harnessCwd) == 0 {
			return false
		}
		tree = found[0]
		return tree.Session != nil
	}) {
		refused, _ := os.ReadFile(worker.ready + ".refused")
		detail := fmt.Sprintf("no checkout with an owner and no started harness: worktrees %+v, harness in %q, a refused launch said %q", listTreesFrom(rewake), harnessCwd, refused)
		return []telemetryFinding{finding(obsClaudeTreeEntered, false, "%s", detail), finding(obsClaudeTreeFinished, false, "%s", detail)}
	}
	want := filepath.Join(tree.Path, "src", "nested")
	registered := registeredCWD(rewake, worker.name)
	branch, _ := git(tree.Path, "symbolic-ref", "-q", "HEAD")
	entered := tree.Name == "probe" && tree.Commit == head && branch == "refs/heads/probe" &&
		registered == want && string(harnessCwd) == want && tree.Session.Name == worker.name
	out := []telemetryFinding{finding(obsClaudeTreeEntered, entered,
		"checkout %s at %s on %q from %s (source HEAD %s) owned by %v; want the work in %s: record %q, harness %q",
		tree.Name, tree.Path, branch, tree.Commit, head, tree.Session, want, registered, harnessCwd)}

	if err := os.WriteFile(filepath.Join(tree.Path, "work"), []byte("work\n"), 0o600); err != nil {
		return append(out, finding(obsClaudeTreeFinished, false, "cannot write in the checkout: %v", err))
	}
	if _, err := git(tree.Path, "add", "work"); err != nil {
		return append(out, finding(obsClaudeTreeFinished, false, "cannot stage in the checkout: %v", err))
	}
	if _, err := git(tree.Path, "commit", "-q", "-m", "Work"); err != nil {
		return append(out, finding(obsClaudeTreeFinished, false, "cannot commit in the checkout: %v", err))
	}
	work, _ := git(tree.Path, "rev-parse", "HEAD")
	stopped = true
	if err := worker.stop(c); err != nil {
		return append(out, finding(obsClaudeTreeFinished, false, "the session did not end: %v", err))
	}
	finished, finishErr := rewake("worktree", "finish", "probe")
	main, _ := git(repo, "rev-parse", "main")
	_, gone := os.Stat(tree.Path)
	branches, _ := git(repo, "branch", "--list", "probe")
	left := listTreesFrom(rewake)
	held := finishErr == nil && main == work && os.IsNotExist(gone) && branches == "" && len(left) == 0
	return append(out, finding(obsClaudeTreeFinished, held, "finish: %v, %q; main %s, the commit %s; checkout gone: %v; branch probe %q; records left %d",
		finishErr, finished, main, work, os.IsNotExist(gone), branches, len(left)))
}

// A Claude Code adapter that names the parent of the launch directory: the
// checkout is made and finished as before, but the session and the harness work
// one level above the directory the person launched from. A mutant that left
// --worktree to the harness would prove less here: the fixture refuses the
// flag, the launch exits 2, and a case whose process failed fails its cleanup
// whatever the observations say; the unit tests hold that refusal instead.
var mutantClaudeWorktreeParent = mutation{
	name:  "claude-worktree-parent",
	file:  "internal/harness/claude/worktree.go",
	edits: []edit{{"\treturn resolved, args, nil", "\treturn filepath.Dir(resolved), args, nil"}},
}

func TestAClaudeLaunchInTheWrongPlaceOfItsWorktreeFails(t *testing.T) {
	runFindingsControlOn(t, claudeColumn.harness, "claude-worktree", playClaudeWorktree, mutantClaudeWorktreeParent, obsClaudeTreeEntered)
}
