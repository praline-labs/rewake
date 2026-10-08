package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// What rm keeps once the session a checkout was made for has ended: work that
// is not in the checkout's changes, and sessions other than its own. Each is
// played on a checkout of its own, so a mutant that lets one rm through does
// not take the others' checkouts with it.

const (
	obsTreeVisited   = "after its session ends, rm still refuses the checkout while another rewake session started in it runs, naming that session"
	obsTreeIgnored   = "rm refuses a checkout holding a file git ignores and leaves the file; --force removes it"
	obsTreeUnreached = "rm refuses a checkout still at the commit it was made at once no ref holds that commit — its own branch left for a detached HEAD — and removes it once they hold it again"
	obsTreeMoved     = "rm refuses a checkout whose directory was moved away, pointing at git worktree repair; --force takes out its record and git's entry for it"
)

// treeStand is what the checks after the end share with the scenario.
type treeStand struct {
	t      *testing.T
	c      *Case
	iso    *Isolation
	repo   string
	rewake func(args ...string) (string, error)
	git    func(dir string, args ...string) (string, error)
	trees  func() []treeView
}

func (s treeStand) finding(observation string, held bool, detail string, args ...any) telemetryFinding {
	return telemetryFinding{observation: observation, held: held, judged: true, detail: fmt.Sprintf(detail, args...)}
}

// visited starts another session in the ended session's checkout and asks rm
// to remove it.
func (s treeStand) visited(tree treeView) telemetryFinding {
	if _, err := os.Stat(tree.Path); err != nil {
		return s.finding(obsTreeVisited, false, "the checkout is gone before the visitor came: %v", err)
	}
	cwdFile := filepath.Join(s.iso.Home, "visitor.cwd")
	visitor := startSessionIn(s.t, s.c, s.iso, tree.Path, "codex", "visitor", "--general", nil, nil, shimCwdFile+"="+cwdFile)
	defer stopSession(s.t, s.c, visitor)
	// Registered is not enough: both halves must have started in the
	// checkout, or a mutant's rm takes the directory before they do and the
	// visitor fails to start rather than losing its place.
	if !waitFor(s.c, 30*time.Second, func() bool {
		client, _ := os.ReadFile(cwdFile + ".client")
		server, _ := os.ReadFile(cwdFile + ".server")
		return len(client) > 0 && len(server) > 0 && registeredCWD(s.rewake, visitor.name) != ""
	}) {
		return s.finding(obsTreeVisited, false, "the visitor never started in the checkout")
	}
	out, err := s.rewake("worktree", "rm", tree.Name)
	_, there := os.Stat(tree.Path)
	refused := err != nil && strings.Contains(err.Error(), visitor.name) && there == nil
	return s.finding(obsTreeVisited, refused, "rm with %s working in the checkout: %v, %q; checkout there: %v", visitor.name, err, out, there == nil)
}

// spares launches one session per name, each with a checkout of that name,
// and ends them once each owns its checkout.
func (s treeStand) spares(names ...string) (map[string]treeView, error) {
	var sessions []*codexSession
	for _, name := range names {
		sessions = append(sessions, startSessionIn(s.t, s.c, s.iso, s.repo, "codex", name, "--general", nil, []string{"--worktree=" + name}))
	}
	byName := map[string]treeView{}
	owned := waitFor(s.c, 30*time.Second, func() bool {
		for _, tree := range s.trees() {
			if tree.Session != nil {
				byName[tree.Name] = tree
			}
		}
		for _, name := range names {
			if _, ok := byName[name]; !ok {
				return false
			}
		}
		return true
	})
	for _, session := range sessions {
		stopSession(s.t, s.c, session)
	}
	if !owned {
		return nil, fmt.Errorf("not every spare checkout was owned: %+v", s.trees())
	}
	return byName, nil
}

// ignored leaves a .env, which the repository ignores, in a checkout.
func (s treeStand) ignored(tree treeView) telemetryFinding {
	env := filepath.Join(tree.Path, ".env")
	if err := os.WriteFile(env, []byte("TOKEN=x\n"), 0o600); err != nil {
		return s.finding(obsTreeIgnored, false, "cannot write the .env: %v", err)
	}
	kept, keptErr := s.rewake("worktree", "rm", tree.Name)
	_, left := os.Stat(env)
	forced, forceErr := s.rewake("worktree", "rm", tree.Name, "--force")
	_, gone := os.Stat(tree.Path)
	held := keptErr != nil && strings.Contains(keptErr.Error(), "files git ignores") && left == nil && forceErr == nil && os.IsNotExist(gone)
	return s.finding(obsTreeIgnored, held, "rm with a .env: %v, %q, the .env left: %v; rm --force: %v, %q, gone: %v",
		keptErr, kept, left == nil, forceErr, forced, os.IsNotExist(gone))
}

// unreached leaves a checkout detached at the commit it was made at and takes
// away every ref holding that commit — its own branch, main and the other
// checkouts' branches — then puts them back as they were.
func (s treeStand) unreached(tree treeView) telemetryFinding {
	if _, err := s.git(tree.Path, "switch", "-q", "--detach"); err != nil {
		return s.finding(obsTreeUnreached, false, "cannot leave the branch: %v", err)
	}
	holders, err := s.git(s.repo, "for-each-ref", "--contains", tree.Commit, "--format=%(refname) %(objectname)", "refs/heads", "refs/tags", "refs/remotes")
	if err != nil {
		return s.finding(obsTreeUnreached, false, "cannot list the refs holding %s: %v", tree.Commit, err)
	}
	refs := strings.Fields(holders)
	for i := 0; i+1 < len(refs); i += 2 {
		if _, err := s.git(s.repo, "update-ref", "-d", refs[i]); err != nil {
			return s.finding(obsTreeUnreached, false, "cannot delete %s: %v", refs[i], err)
		}
	}
	kept, keptErr := s.rewake("worktree", "rm", tree.Name)
	_, there := os.Stat(tree.Path)
	for i := 0; i+1 < len(refs); i += 2 {
		if _, err := s.git(s.repo, "update-ref", refs[i], refs[i+1]); err != nil {
			return s.finding(obsTreeUnreached, false, "cannot restore %s: %v", refs[i], err)
		}
	}
	removed, removeErr := s.rewake("worktree", "rm", tree.Name)
	_, gone := os.Stat(tree.Path)
	held := keptErr != nil && strings.Contains(keptErr.Error(), "on no branch") && there == nil && removeErr == nil && os.IsNotExist(gone)
	return s.finding(obsTreeUnreached, held, "rm with %v deleted: %v, %q, checkout there: %v; rm with them back: %v, %q, gone: %v",
		refs, keptErr, kept, there == nil, removeErr, removed, os.IsNotExist(gone))
}

// moved takes a checkout's directory away from where the record says it is.
func (s treeStand) moved(tree treeView) telemetryFinding {
	away := tree.Path + ".away"
	if err := os.Rename(tree.Path, away); err != nil {
		return s.finding(obsTreeMoved, false, "cannot move the checkout: %v", err)
	}
	defer func() { _ = os.RemoveAll(away) }()
	kept, keptErr := s.rewake("worktree", "rm", tree.Name)
	listedKept, _ := s.git(s.repo, "worktree", "list", "--porcelain")
	forced, forceErr := s.rewake("worktree", "rm", tree.Name, "--force")
	listed, _ := s.git(s.repo, "worktree", "list", "--porcelain")
	entry := "worktree " + tree.Path + "\n"
	recorded := false
	for _, view := range s.trees() {
		recorded = recorded || view.Name == tree.Name
	}
	held := keptErr != nil && strings.Contains(keptErr.Error(), "git worktree repair") && strings.Contains(listedKept+"\n", entry) &&
		forceErr == nil && !strings.Contains(listed+"\n", entry) && !recorded
	return s.finding(obsTreeMoved, held, "rm of the moved checkout: %v, %q; rm --force: %v, %q; record left: %v; git lists:\n%s",
		keptErr, kept, forceErr, forced, recorded, listed)
}

// registeredCWD is the directory a session registered as its own, or "".
func registeredCWD(rewake func(args ...string) (string, error), name string) string {
	out, err := rewake("list", "--json")
	if err != nil {
		return ""
	}
	var listing struct {
		Sessions []struct {
			Name string `json:"name"`
			CWD  string `json:"cwd"`
		} `json:"sessions"`
	}
	if json.Unmarshal([]byte(out), &listing) != nil {
		return ""
	}
	for _, session := range listing.Sessions {
		if session.Name == name {
			return session.CWD
		}
	}
	return ""
}

// The controls of what rm keeps.

// rm that sees only the session a checkout was made for: a session started in
// it since loses its directory.
var mutantWorktreeVisitorIgnored = mutation{
	name:  "worktree-visitor-ignored",
	file:  "internal/cli/worktree_keep.go",
	edits: []edit{{"(made || worktree.Within(session.CWD, record.Path))", "(made || false)"}},
}

// Ignored files taken for nothing: a .env is deleted with the checkout.
var mutantWorktreeIgnoredUnseen = mutation{
	name:  "worktree-ignored-unseen",
	file:  "internal/worktree/git.go",
	edits: []edit{{"\t\t\tcheck.Ignored = true\n", "\t\t\tcheck.Ignored = false\n"}},
}

// Reachability asked only after the HEAD moved: the commit a checkout was made
// at is taken as safe after its branch is gone.
var mutantWorktreeBranchTrusted = mutation{
	name: "worktree-branch-trusted",
	file: "internal/worktree/git.go",
	edits: []edit{{
		"\tif check.Unreachable, err = unreachable(record.CommonDir, head); err != nil {\n\t\treturn Check{}, err\n\t}\n\tentry, _, err := listed(record)",
		"\tif check.Unreachable, err = unreachable(record.CommonDir, head); err != nil {\n\t\treturn Check{}, err\n\t}\n\tcheck.Unreachable = check.Unreachable && head != record.Commit\n\tentry, _, err := listed(record)",
	}},
}

// A missing directory taken for a removed one: rm without --force goes on,
// and the work moved with the directory loses its entry, which git worktree
// repair needed to reconnect it. (That the entry goes alone, where git
// worktree prune would take every missing checkout's, is the unit test's.)
var mutantWorktreeMovedUnrefused = mutation{
	name:  "worktree-moved-unrefused",
	file:  "internal/cli/worktree_keep.go",
	edits: []edit{{"\tif check.Missing && !check.Forgotten {\n", "\tif false {\n"}},
}

func TestARemovalThatIgnoresAVisitorFails(t *testing.T) {
	runFindingsControlOn(t, codexColumn.harness, "codex-worktree", playCodexWorktree, mutantWorktreeVisitorIgnored, obsTreeVisited, obsTreeRemoved)
}

func TestARemovalThatDeletesIgnoredFilesFails(t *testing.T) {
	runFindingsControlOn(t, codexColumn.harness, "codex-worktree", playCodexWorktree, mutantWorktreeIgnoredUnseen, obsTreeIgnored)
}

func TestARemovalThatTrustsTheMadeCommitFails(t *testing.T) {
	runFindingsControlOn(t, codexColumn.harness, "codex-worktree", playCodexWorktree, mutantWorktreeBranchTrusted, obsTreeUnreached)
}

func TestARemovalOfAMovedCheckoutFails(t *testing.T) {
	runFindingsControlOn(t, codexColumn.harness, "codex-worktree", playCodexWorktree, mutantWorktreeMovedUnrefused, obsTreeMoved)
}
