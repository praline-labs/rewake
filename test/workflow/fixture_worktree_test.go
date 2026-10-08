package workflow

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestFixtureWorktree is `rewake fixture --worktree` end to end on the gate
// column: a worker is launched from a directory inside a repository with
// --worktree=probe, and rewake makes a checkout of HEAD on a new branch probe
// under its worktree directory and starts the session there — the wrapper,
// the terminal and the session half all at the launch directory's place
// within the checkout. A task sent to it is delivered. While it runs, `rewake
// worktree rm` refuses its checkout; a commit made in it lands in the source
// with `rewake worktree land`, hashes kept and the session left running; a
// second commit follows, and finish refuses while the session runs. After it
// has ended rm still refuses while another session started there runs. Three
// more checkouts, each launched and ended for the purpose, show what else rm
// keeps without --force: a file git ignores, a commit no ref holds any more,
// and a directory moved away. Once nothing holds the first checkout, finish
// lands the second commit and removes the checkout through git, and its
// branch, so the repository forgets both.
//
// Every part of it is the core's: the fixture only takes the flag
// (internal/harness/fixture/worktree.go) and says where each half started.
// What a harness does in a checkout of its own making — Claude Code's -w — is
// not started here; claude-worktree holds the Claude Code adapter's answer of
// where its launch works.
func TestFixtureWorktree(t *testing.T) {
	binary := enterScenario(t, "fixture-worktree")
	c := Start(t, Spec{
		Name:         "fixture-worktree",
		Harness:      fixtureColumn.harness,
		Observations: fixtureWorktreeObservations,
		Deadline:     90 * time.Second,
	})
	for _, finding := range playFixtureWorktree(t, c, Isolate(t, c, binary)) {
		if finding.held {
			c.Observed(finding.observation, finding.detail)
		} else {
			c.Contradicted(finding.observation, "%s", finding.detail)
		}
	}
}

const (
	obsTreeEntered    = "a launch with --worktree=probe runs in a checkout of HEAD on a new branch probe under the worktree directory, at the launch directory's place in it: the session's record, the terminal and the session half all work there, and the record names the session"
	obsTreeDelivered  = "a task sent to the session in the checkout is delivered"
	obsTreeKept       = "rewake worktree rm refuses the checkout while its session runs, saying so, and leaves it"
	obsTreeLanded     = "rewake worktree land, while the session runs, fast-forwards the source's main to the commit made in the checkout, hash kept and its file in the source, and leaves the checkout on its branch and the session running"
	obsTreeFinishKept = "rewake worktree finish refuses while the session runs, naming it, and lands and removes nothing"
	obsTreeRemoved    = "after the sessions end, rewake worktree finish lands the last commit by fast-forward and removes the checkout, its record, git's own entry for it and its branch"
	fixtureTreeTask   = "fixture-worktree-probe: work here"
)

var fixtureWorktreeObservations = []string{
	obsTreeEntered, obsTreeDelivered, obsTreeKept, obsTreeLanded, obsTreeFinishKept, obsTreeVisited,
	obsTreeIgnored, obsTreeUnreached, obsTreeMoved, obsTreeRemoved,
}

// halvesStarted reads where both halves of a fixture session started, empty
// while either has not written it yet.
func halvesStarted(cwdFile string) (session, terminal []byte) {
	session, _ = os.ReadFile(cwdFile + ".fixture")
	terminal, _ = os.ReadFile(cwdFile + ".terminal")
	if len(session) == 0 || len(terminal) == 0 {
		return nil, nil
	}
	return session, terminal
}

func playFixtureWorktree(t *testing.T, c *Case, iso *Isolation) []telemetryFinding {
	t.Helper()
	finding := func(observation string, held bool, detail string, args ...any) telemetryFinding {
		return telemetryFinding{observation: observation, held: held, judged: true, detail: fmt.Sprintf(detail, args...)}
	}
	unjudged := func(from int, detail string) []telemetryFinding {
		var out []telemetryFinding
		for _, observation := range fixtureWorktreeObservations[from:] {
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
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte(".env\n"), 0o600); err != nil {
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
	listTrees := func() []treeView { return listTreesFrom(rewake) }

	col := fixtureColumn.harness
	worker := startSessionIn(t, c, iso, nested, col, "tree", "--general", nil, []string{"--worktree=probe"},
		shimCwdFile+"="+cwdFile, shimInboxJSON+"=1")
	stopped := false
	defer func() {
		if !stopped {
			stopSession(t, c, worker)
		}
	}()

	var tree treeView
	var session, terminal []byte
	if !waitFor(c, 30*time.Second, func() bool {
		found := listTrees()
		session, terminal = halvesStarted(cwdFile)
		if len(found) != 1 || session == nil {
			return false
		}
		tree = found[0]
		return tree.Session != nil
	}) {
		return unjudged(0, fmt.Sprintf("no checkout with an owner and no started halves: worktrees %+v, session half %q, terminal %q", listTrees(), session, terminal))
	}
	registered := registeredCWD(rewake, worker.name)
	want := filepath.Join(tree.Path, "src", "nested")
	branch, _ := git(tree.Path, "symbolic-ref", "-q", "HEAD")
	checkedOut, _ := git(tree.Path, "rev-parse", "HEAD")
	sourceBranch, _ := git(repo, "symbolic-ref", "-q", "HEAD")
	entered := tree.Name == "probe" && strings.HasPrefix(tree.Path, trees+string(filepath.Separator)) &&
		tree.Commit == head && checkedOut == head && branch == "refs/heads/probe" && sourceBranch == "refs/heads/main" &&
		registered == want && string(session) == want && string(terminal) == want && tree.Session.Name == worker.name
	out := []telemetryFinding{finding(obsTreeEntered, entered,
		"checkout %s at %s (HEAD %s on %q, source HEAD %s on %q) owned by %v; want the work in %s: record %q, session half %q, terminal %q",
		tree.Name, tree.Path, checkedOut, branch, head, sourceBranch, tree.Session, want, registered, session, terminal)}

	// A session sends it, so the task has a run to report to.
	lead := startHarnessSession(t, c, iso, col, "lead", "--main",
		shimSendTo+"="+worker.name, shimSendText+"="+fixtureTreeTask, readinessSwitch(worker))
	defer stopSession(t, c, lead)
	var task reportView
	delivered := waitFor(c, 30*time.Second, func() bool {
		var ok bool
		task, ok = messageCarrying(worker, "fixture-worktree-probe")
		return ok && slices.Contains(worker.deliveredIDs(), task.ID)
	})
	out = append(out, finding(obsTreeDelivered, delivered, "the session read %q and was told of %v", task.ID, worker.deliveredIDs()))

	kept, keptErr := rewake("worktree", "rm", "probe")
	_, stillThere := os.Stat(tree.Path)
	refused := keptErr != nil && strings.Contains(keptErr.Error(), "still run in it: "+worker.name) && stillThere == nil
	out = append(out, finding(obsTreeKept, refused, "rm while running: %v, %q; checkout there: %v", keptErr, kept, stillThere == nil))

	commit := func(dir, file string) (string, error) {
		if err := os.WriteFile(filepath.Join(dir, file), []byte(file+"\n"), 0o600); err != nil {
			return "", err
		}
		if _, err := git(dir, "add", file); err != nil {
			return "", err
		}
		if _, err := git(dir, "commit", "-q", "-m", "Add "+file); err != nil {
			return "", err
		}
		return git(dir, "rev-parse", "HEAD")
	}
	out = append(out, midSession(worker, tree, repo, commit, git, rewake, finding)...)
	landedAt, _ := git(repo, "rev-parse", "main")

	stopped = true
	if err := worker.stop(c); err != nil {
		return append(out, unjudged(5, "the session did not end: "+err.Error())...)
	}
	stand := treeStand{t: t, c: c, iso: iso, repo: repo, rewake: rewake, git: git, trees: listTrees}
	out = append(out, stand.visited(tree))
	spares, err := stand.spares("ignored", "unreached", "moved")
	if err != nil {
		return append(out, unjudged(6, err.Error())...)
	}
	out = append(out, stand.ignored(spares["ignored"]), stand.unreached(spares["unreached"]), stand.moved(spares["moved"]))

	last, _ := git(tree.Path, "rev-parse", "HEAD")
	finished, finishErr := rewake("worktree", "finish", "probe")
	main, _ := git(repo, "rev-parse", "main")
	_, gone := os.Stat(tree.Path)
	listed, _ := git(repo, "worktree", "list", "--porcelain")
	branches, _ := git(repo, "branch", "--list", "probe")
	left := listTrees()
	removed := finishErr == nil && main == last && last != landedAt && os.IsNotExist(gone) && len(left) == 0 &&
		!strings.Contains(listed, tree.Path+"\n") && branches == ""
	return append(out, finding(obsTreeRemoved, removed,
		"finish after the end: %v, %q; main %s, the checkout's last commit %s, landed before %s; checkout gone: %v; records left %d; branch probe %q; git lists:\n%s",
		finishErr, finished, main, last, landedAt, os.IsNotExist(gone), len(left), branches, listed))
}

// midSession is the work in the checkout while its session runs: a commit
// landed mid-session, a second one, and a finish refused because the session
// runs.
func midSession(worker *scenarioSession, tree treeView, repo string,
	commit func(dir, file string) (string, error),
	git func(dir string, args ...string) (string, error),
	rewake func(args ...string) (string, error),
	finding func(observation string, held bool, detail string, args ...any) telemetryFinding,
) []telemetryFinding {
	first, err := commit(tree.Path, "first")
	if err != nil {
		return []telemetryFinding{
			finding(obsTreeLanded, false, "cannot commit in the checkout: %v", err),
			finding(obsTreeFinishKept, false, "no commit to finish with: %v", err),
		}
	}
	landed, landErr := rewake("worktree", "land", "probe")
	main, _ := git(repo, "rev-parse", "main")
	_, inSource := os.Stat(filepath.Join(repo, "first"))
	branch, _ := git(tree.Path, "symbolic-ref", "-q", "HEAD")
	head, _ := git(tree.Path, "rev-parse", "HEAD")
	running := false
	for _, view := range listTreesFrom(rewake) {
		running = running || view.Name == tree.Name && view.Running
	}
	held := landErr == nil && strings.Contains(landed, "landed 1 commit of probe into main") && main == first &&
		inSource == nil && branch == "refs/heads/probe" && head == first && running
	out := []telemetryFinding{finding(obsTreeLanded, held,
		"land mid-session: %v, %q; main %s, the commit %s, its file in the source: %v; checkout on %q at %s; still running: %v",
		landErr, landed, main, first, inSource == nil, branch, head, running)}

	second, err := commit(tree.Path, "second")
	if err != nil {
		return append(out, finding(obsTreeFinishKept, false, "cannot commit again in the checkout: %v", err))
	}
	refused, refuseErr := rewake("worktree", "finish", "probe")
	after, _ := git(repo, "rev-parse", "main")
	_, there := os.Stat(tree.Path)
	tip, _ := git(repo, "rev-parse", "refs/heads/probe")
	kept := refuseErr != nil && strings.Contains(refuseErr.Error(), "still run in it: "+worker.name) &&
		after == main && there == nil && tip == second
	return append(out, finding(obsTreeFinishKept, kept, "finish while running: %v, %q; main %s (was %s); checkout there: %v; probe at %s, want %s",
		refuseErr, refused, after, main, there == nil, tip, second))
}
