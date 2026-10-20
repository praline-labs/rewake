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
	iso := Isolate(t, c, binary)
	repo := filepath.Join(iso.Home, "project")
	if err := os.MkdirAll(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "file"), []byte("one\n"), 0o600); err != nil {
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
	aliasDir := filepath.Join(iso.Home, ".config", "rewake")
	if err := os.MkdirAll(aliasDir, 0o700); err != nil {
		t.Fatal(err)
	}
	aliases := fmt.Sprintf("[alias.worker]\nharness = %q\nrole = \"write\"\nworktree = true\n", fixtureColumn.harness)
	if err := os.WriteFile(filepath.Join(aliasDir, "aliases.toml"), []byte(aliases), 0o600); err != nil {
		t.Fatal(err)
	}

	asks := newRequests(iso, "lead")
	lead := startSessionIn(t, c, iso, repo, fixtureColumn.harness, "lead", "--main", nil, nil, asks.env(), shimInboxJSON+"=1")
	defer stopSession(t, c, lead)
	if !waitFor(c, 20*time.Second, func() bool { _, err := os.Stat(lead.ready); return err == nil }) {
		c.Contradicted(obsSpaceTreeStatus, "main never became ready")
		return
	}
	worker, env := spaceWorker(iso, "api", fixtureColumn.harness)
	workerAsks := newRequests(iso, "api")
	env[0] = workerAsks.env()
	encoded, _ := json.Marshal(shimRequest{Args: []string{"space", "open", "api", "--alias", "worker", "--json"}, Env: env})
	code, out, answered := asks.send(c, encoded, "space open api")
	var opened struct {
		Session  string `json:"session"`
		Worktree bool   `json:"worktree"`
	}
	if !answered || code != 0 || json.Unmarshal([]byte(out), &opened) != nil || opened.Session != worker.name || !opened.Worktree {
		c.Contradicted(obsSpaceTreeStatus, "space open answered %d, %v: %s", code, answered, out)
		return
	}

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
	code, out, _ = workerAsks.ask(c, "worktree", "status", "--json")
	head, _ := git(repo, "rev-parse", "HEAD")
	trees := filepath.Join(iso.Home, ".local", "share", "rewake", "worktrees") + string(filepath.Separator)
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
