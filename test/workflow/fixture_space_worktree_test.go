package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestFixtureSpaceWorktree is a worker opened from a template with
// worktree = true, on the fixture: main opens it in a repository, the worker
// works in a checkout of its own and asks rewake worktree status, which names
// that checkout and forecasts its removal; main's space close then removes
// the clean checkout and its branch, as forecast.
func TestFixtureSpaceWorktree(t *testing.T) {
	binary := enterScenario(t, "fixture-space-worktree")
	c := Start(t, Spec{
		Name:         "fixture-space-worktree",
		Harness:      fixtureColumn.harness,
		Observations: fixtureSpaceWorktreeObservations,
		Deadline:     90 * time.Second,
	})
	room, ok := openTreeWorker(t, c, binary, "worktree = true", obsSpaceTreeStatus)
	if !ok {
		return
	}
	defer stopSession(t, c, room.lead)
	if !room.opened.Worktree {
		c.Contradicted(obsSpaceTreeStatus, "space open made no worktree")
		return
	}
	asks, workerAsks, repo, git := room.asks, room.workerAsks, room.repo, room.git

	var status struct {
		Space    string `json:"space"`
		Checkout *struct {
			Path, Branch, Base string
		} `json:"checkout"`
		Close struct {
			Outcome string   `json:"outcome"`
			Reasons []string `json:"reasons"`
		} `json:"close"`
	}
	code, out, _ := workerAsks.ask(c, "worktree", "status", "--json")
	head, _ := git(repo, "rev-parse", "HEAD")
	trees := room.trees
	if code != 0 || json.Unmarshal([]byte(out), &status) != nil || status.Checkout == nil || status.Space != "api" ||
		!strings.HasPrefix(status.Checkout.Path, trees) || status.Checkout.Base != head || status.Close.Outcome != "remove" {
		c.Contradicted(obsSpaceTreeStatus, "the worker's status answered %d: %s", code, out)
		return
	}
	c.Observed(obsSpaceTreeStatus, fmt.Sprintf("the worker works in %s on %s, and its close is forecast to remove it", status.Checkout.Path, status.Checkout.Branch))

	code, out, _ = asks.ask(c, "space", "close", "api", "--json")
	var closed struct{ State, Why string }
	_, statErr := os.Stat(status.Checkout.Path)
	branch, _ := git(repo, "branch", "--list", status.Checkout.Branch)
	if code != 0 || json.Unmarshal([]byte(out), &closed) != nil || closed.State != "closed" || !os.IsNotExist(statErr) || branch != "" {
		c.Contradicted(obsSpaceTreeRemoved, "space close answered %d: %s; checkout: %v; branch %q", code, out, statErr, branch)
		return
	}
	c.Observed(obsSpaceTreeRemoved, "the close removed the clean checkout and its branch: "+closed.Why)
}

const (
	obsSpaceTreeStatus  = "a worker opened from a template with worktree = true works in a checkout of its own, which its rewake worktree status names, forecasting removal at close"
	obsSpaceTreeRemoved = "main's space close removes that clean checkout and its branch, as forecast"
)

var fixtureSpaceWorktreeObservations = []string{obsSpaceTreeStatus, obsSpaceTreeRemoved}

// treeRoom is main ready in a repository of its own, and a worker it opened
// from the alias worker, whose last line is given.
type treeRoom struct {
	repo, trees string
	git         func(dir string, args ...string) (string, error)
	lead        *scenarioSession
	asks        *requests
	workerAsks  *requests
	opened      struct {
		Session  string `json:"session"`
		Worktree bool   `json:"worktree"`
	}
}

func openTreeWorker(t *testing.T, c *Case, binary, line, obs string) (*treeRoom, bool) {
	t.Helper()
	iso := Isolate(t, c, binary)
	room := &treeRoom{
		repo:  filepath.Join(iso.Home, "project"),
		trees: filepath.Join(iso.Home, ".local", "share", "rewake", "worktrees") + string(filepath.Separator),
	}
	if err := os.MkdirAll(room.repo, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(room.repo, "file"), []byte("one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	room.git = func(dir string, args ...string) (string, error) {
		command := exec.Command("git", append([]string{"-C", dir}, args...)...)
		command.Env = append(iso.Env(), "GIT_CONFIG_NOSYSTEM=1",
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.invalid",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.invalid")
		out, err := c.OutputAllowingFailure(command)
		return strings.TrimSpace(string(out)), err
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "."}, {"commit", "-q", "-m", "First"}} {
		if _, err := room.git(room.repo, args...); err != nil {
			t.Fatalf("preparing the repository: git %q: %v", args, err)
		}
	}
	aliasDir := filepath.Join(iso.Home, ".config", "rewake")
	if err := os.MkdirAll(aliasDir, 0o700); err != nil {
		t.Fatal(err)
	}
	aliases := fmt.Sprintf("[alias.worker]\nharness = %q\nrole = \"write\"\n%s\n", fixtureColumn.harness, line)
	if err := os.WriteFile(filepath.Join(aliasDir, "aliases.toml"), []byte(aliases), 0o600); err != nil {
		t.Fatal(err)
	}

	room.asks = newRequests(iso, "lead")
	room.lead = startSessionIn(t, c, iso, room.repo, fixtureColumn.harness, "lead", "--main", nil, nil, room.asks.env(), shimInboxJSON+"=1")
	if !waitFor(c, 20*time.Second, func() bool { _, err := os.Stat(room.lead.ready); return err == nil }) {
		c.Contradicted(obs, "main never became ready")
		stopSession(t, c, room.lead)
		return room, false
	}
	worker, env := spaceWorker(iso, "api", fixtureColumn.harness)
	room.workerAsks = newRequests(iso, "api")
	env[0] = room.workerAsks.env()
	encoded, _ := json.Marshal(shimRequest{Args: []string{"space", "open", "api", "--alias", "worker", "--json"}, Env: env})
	code, out, answered := room.asks.send(c, encoded, "space open api")
	if !answered || code != 0 || json.Unmarshal([]byte(out), &room.opened) != nil || room.opened.Session != worker.name {
		c.Contradicted(obs, "space open answered %d, %v: %s", code, answered, out)
		stopSession(t, c, room.lead)
		return room, false
	}
	return room, true
}
