package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestCodexWorktree is `rewake codex --worktree` end to end on the Codex
// column: a worker is launched from a directory inside a repository with
// --worktree=probe, and rewake makes a detached checkout of HEAD under its
// worktree directory and starts the session there — the wrapper, the
// terminal and the app-server all at the launch directory's place within the
// checkout. A task sent to it is delivered. While it runs, `rewake worktree
// rm` refuses its checkout; once it has ended, rm removes the checkout through
// git, so the repository forgets it too.
//
// What it does not prove: anything of Codex's own worktree, which the terminal
// refuses beside --remote (docs/research-codex.md); rewake never asks it for one.
func TestCodexWorktree(t *testing.T) {
	binary := enterScenario(t, "codex-worktree")
	c := Start(t, Spec{
		Name:         "codex-worktree",
		Harness:      codexColumn.harness,
		Observations: codexWorktreeObservations,
		Deadline:     90 * time.Second,
	})
	for _, finding := range playCodexWorktree(t, c, Isolate(t, c, binary)) {
		if finding.held {
			c.Observed(finding.observation, finding.detail)
		} else {
			c.Contradicted(finding.observation, "%s", finding.detail)
		}
	}
}

const (
	obsTreeEntered   = "a launch with --worktree=probe runs in a detached checkout of HEAD under the worktree directory, at the launch directory's place in it: the session's record, the terminal and the app-server all work there, and the record names the session"
	obsTreeDelivered = "a task sent to the session in the checkout is delivered"
	obsTreeKept      = "rewake worktree rm refuses the checkout while its session runs, saying so, and leaves it"
	obsTreeRemoved   = "after the session ends, rewake worktree rm removes the checkout, its record, and git's own entry for it"
	codexTreeTask    = "codex-worktree-probe: work here"
)

var codexWorktreeObservations = []string{obsTreeEntered, obsTreeDelivered, obsTreeKept, obsTreeRemoved}

// shimCwdFile makes each half of the Codex fixture write the directory it was
// started in to this path with .client or .server appended.
const shimCwdFile = "RW_SHIM_CWD_FILE"

func recordShimCwd(half string) {
	target := os.Getenv(shimCwdFile)
	if target == "" {
		return
	}
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "unreadable: " + err.Error()
	}
	_ = os.WriteFile(target+"."+half, []byte(cwd), 0o600)
}

// treeView is the part of `rewake worktree ls --json` the scenario reads.
type treeView struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Commit  string `json:"commit"`
	Running bool   `json:"running"`
	Session *struct {
		Name string `json:"name"`
	} `json:"session"`
}

func playCodexWorktree(t *testing.T, c *Case, iso *Isolation) []telemetryFinding {
	t.Helper()
	finding := func(observation string, held bool, detail string, args ...any) telemetryFinding {
		return telemetryFinding{observation: observation, held: held, judged: true, detail: fmt.Sprintf(detail, args...)}
	}
	unjudged := func(from int, detail string) []telemetryFinding {
		var out []telemetryFinding
		for _, observation := range codexWorktreeObservations[from:] {
			out = append(out, telemetryFinding{observation: observation, detail: detail})
		}
		return out
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
	trees := filepath.Join(iso.Home, ".local", "share", "rewake", "worktrees")
	cwdFile := filepath.Join(iso.Home, "tree.cwd")
	rewake := func(args ...string) (string, error) {
		out, err := c.OutputAllowingFailure(iso.Command(args...))
		return string(out), err
	}
	listTrees := func() []treeView {
		out, err := rewake("worktree", "ls", "--json")
		var listing struct {
			Worktrees []treeView `json:"worktrees"`
		}
		if err != nil || json.Unmarshal([]byte(out), &listing) != nil {
			return nil
		}
		return listing.Worktrees
	}

	worker := startSessionIn(t, c, iso, nested, "codex", "tree", "--general", nil, []string{"--worktree=probe"},
		shimCwdFile+"="+cwdFile, shimInboxJSON+"=1")
	stopped := false
	defer func() {
		if !stopped {
			stopSession(t, c, worker)
		}
	}()

	var tree treeView
	var client, server []byte
	if !waitFor(c, 30*time.Second, func() bool {
		found := listTrees()
		var err1, err2 error
		client, err1 = os.ReadFile(cwdFile + ".client")
		server, err2 = os.ReadFile(cwdFile + ".server")
		if len(found) != 1 || err1 != nil || err2 != nil {
			return false
		}
		tree = found[0]
		return tree.Session != nil
	}) {
		return unjudged(0, fmt.Sprintf("no checkout with an owner and no started halves: worktrees %+v, client %q, server %q", listTrees(), client, server))
	}
	registered := ""
	if out, err := rewake("list", "--json"); err == nil {
		var listing struct {
			Sessions []struct {
				Name string `json:"name"`
				CWD  string `json:"cwd"`
			} `json:"sessions"`
		}
		if json.Unmarshal([]byte(out), &listing) == nil {
			for _, session := range listing.Sessions {
				if session.Name == worker.name {
					registered = session.CWD
				}
			}
		}
	}
	want := filepath.Join(tree.Path, "src", "nested")
	_, onBranch := git(tree.Path, "symbolic-ref", "-q", "HEAD")
	checkedOut, _ := git(tree.Path, "rev-parse", "HEAD")
	entered := tree.Name == "probe" && strings.HasPrefix(tree.Path, trees+string(filepath.Separator)) &&
		tree.Commit == head && checkedOut == head && onBranch != nil &&
		registered == want && string(client) == want && string(server) == want && tree.Session.Name == worker.name
	out := []telemetryFinding{finding(obsTreeEntered, entered,
		"checkout %s at %s (HEAD %s, source HEAD %s, detached %v) owned by %v; want the work in %s: record %q, terminal %q, server %q",
		tree.Name, tree.Path, checkedOut, head, onBranch != nil, tree.Session, want, registered, client, server)}

	// A session sends it: mail from a plain shell carries no run to
	// answer to, and a Codex delivery refuses it.
	lead := startHarnessSession(t, c, iso, "codex", "lead", "--main",
		shimSendTo+"="+worker.name, shimSendText+"="+codexTreeTask, readinessSwitch(codexColumn, worker))
	defer stopSession(t, c, lead)
	var task reportView
	delivered := waitFor(c, 30*time.Second, func() bool {
		var ok bool
		task, ok = messageCarrying(worker, "codex-worktree-probe")
		return ok && slices.Contains(worker.deliveredIDs(), task.ID)
	})
	out = append(out, finding(obsTreeDelivered, delivered, "the session read %q and was told of %v", task.ID, worker.deliveredIDs()))

	kept, keptErr := rewake("worktree", "rm", "probe")
	_, stillThere := os.Stat(tree.Path)
	refused := keptErr != nil && strings.Contains(keptErr.Error(), "still runs") && stillThere == nil
	out = append(out, finding(obsTreeKept, refused, "rm while running: %v, %q; checkout there: %v", keptErr, kept, stillThere == nil))

	stopped = true
	if err := worker.stop(c); err != nil {
		return append(out, unjudged(3, "the session did not end: "+err.Error())...)
	}
	removedOut, removeErr := rewake("worktree", "rm", "probe")
	_, gone := os.Stat(tree.Path)
	listed, _ := git(repo, "worktree", "list", "--porcelain")
	left := listTrees()
	removed := removeErr == nil && os.IsNotExist(gone) && len(left) == 0 && !strings.Contains(listed, tree.Path)
	return append(out, finding(obsTreeRemoved, removed,
		"rm after the end: %v, %q; checkout gone: %v; records left %d; git lists:\n%s", removeErr, removedOut, os.IsNotExist(gone), len(left), listed))
}

// The launch that does not enter its checkout: the session and both halves
// work in the source, and nothing else notices.
var mutantWorktreeNotEntered = mutation{
	name:  "worktree-not-entered",
	file:  "internal/cli/launch_worktree.go",
	edits: []edit{{"if err := os.Chdir(record.Workdir()); err != nil {", "if err := error(nil); err != nil {"}},
}

// rm that takes no running session for one: it removes the checkout from
// under the session, and after the end there is nothing left to remove.
var mutantWorktreeRunningIgnored = mutation{
	name:  "worktree-running-ignored",
	file:  "internal/cli/worktree.go",
	edits: []edit{{"\tif owner == nil || owner.Dir == \"\" {", "\tif owner == nil || owner.Dir == \"\" || true {"}},
}

// rm that deletes the directory without asking git: the repository keeps an
// entry for a checkout that is gone.
var mutantWorktreeGitKept = mutation{
	name:  "worktree-git-kept",
	file:  "internal/worktree/git.go",
	edits: []edit{{"\t\tif err := gitDir(record.CommonDir, append(args, record.Path)...); err != nil {", "\t\tif err := os.RemoveAll(record.Path); err != nil {"}},
}

func TestAWorktreeLaunchThatStaysInTheSourceFails(t *testing.T) {
	runFindingsControlOn(t, codexColumn.harness, "codex-worktree", playCodexWorktree, mutantWorktreeNotEntered, obsTreeEntered)
}

func TestARemovalThatIgnoresTheSessionFails(t *testing.T) {
	runFindingsControlOn(t, codexColumn.harness, "codex-worktree", playCodexWorktree, mutantWorktreeRunningIgnored, obsTreeKept, obsTreeRemoved)
}

func TestARemovalBehindGitsBackFails(t *testing.T) {
	runFindingsControlOn(t, codexColumn.harness, "codex-worktree", playCodexWorktree, mutantWorktreeGitKept, obsTreeRemoved)
}
