package main

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// repo is a checkout on v2 with an origin to push to; git is real, go and
// bun are the test's.
type repo struct {
	t       *testing.T
	root    string
	origin  string
	install string
	// red names the checks that fail; calls records go and bun.
	red   map[string]bool
	calls []string
}

func newRepo(t *testing.T) *repo {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, name := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(name, "Owner")
	}
	for _, name := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(name, "owner@example.com")
	}
	r := &repo{t: t, root: t.TempDir(), origin: t.TempDir(), install: t.TempDir(), red: map[string]bool{}}
	r.git(r.origin, "init", "--bare", "-q")
	r.git(r.root, "init", "-q", "-b", branch)
	r.write("AGENTS.md", "## Commits\n\n```\nCo-Authored-By: One <one@example.com>\nCo-Authored-By: Two <two@example.com>\n```\n")
	r.write("shell/app.ts", "1\n")
	r.write(".gitignore", ".scratch/\n")
	r.commit("Start")
	r.git(r.root, "remote", "add", "origin", r.origin)
	r.git(r.root, "push", "-q", "origin", branch)
	if err := os.WriteFile(filepath.Join(r.install, "rewake"), []byte("old build"), 0o755); err != nil {
		t.Fatal(err)
	}
	return r
}

func (r *repo) git(dir string, args ...string) string {
	r.t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func (r *repo) write(name, text string) {
	r.t.Helper()
	path := filepath.Join(r.root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

func (r *repo) commit(subject string) string {
	r.git(r.root, "add", "-A")
	r.git(r.root, "commit", "-q", "-m", subject)
	return r.git(r.root, "rev-parse", "HEAD")
}

// branchOff makes a branch from v2 with one commit writing name, and comes
// back to v2.
func (r *repo) branchOff(work, name, text string) string {
	r.git(r.root, "switch", "-q", "-c", work)
	r.write(name, text)
	tip := r.commit("Work on " + name)
	r.git(r.root, "switch", "-q", branch)
	return tip
}

func (r *repo) run(ctx context.Context, c command, log io.Writer) result {
	if c.name == "git" {
		return execute(ctx, c, log)
	}
	line := strings.Join(append([]string{c.name}, c.args...), " ")
	r.calls = append(r.calls, line)
	switch {
	case strings.Contains(line, "-race"):
		if r.red["race"] {
			return result{code: 1, stdout: "--- FAIL: TestSomething (0.01s)\nFAIL\tgithub.com/praline-labs/rewake/internal/cli\t3.2s\n"}
		}
	case strings.Contains(line, "./test/workflow/"):
		if r.red["workflow"] {
			return result{code: 1, stdout: "--- FAIL: TestWorkflow/brief (2.00s)\n"}
		}
	case strings.HasPrefix(line, "go build"):
		out := c.args[slices.Index(c.args, "-o")+1]
		if err := os.WriteFile(out, []byte("new build"), 0o755); err != nil {
			return result{code: 1, err: err}
		}
	}
	return result{}
}

// land runs land in the repository and answers its exit code and output.
func (r *repo) land(args ...string) (int, string) {
	r.t.Helper()
	c, err := parseArgs(append(args, "--install", r.install))
	if err != nil {
		r.t.Fatal(err)
	}
	var out strings.Builder
	l := &lander{call: c, run: r.run, out: &out, root: r.root, processes: func() [][]string { return nil }}
	code := l.land(context.Background())
	return code, out.String()
}

func (r *repo) at(ref string) string { return r.git(r.root, "rev-parse", ref) }

// An accepted branch ahead of v2 lands as a fast-forward: checked, pushed,
// and its build put in place of the room's.
func TestAFastForwardLandsPushesAndInstalls(t *testing.T) {
	r := newRepo(t)
	tip := r.branchOff("work", "a.go", "package a\n")
	code, out := r.land("work")
	if code != exitDone {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if r.at(branch) != tip || r.git(r.origin, "rev-parse", branch) != tip {
		t.Errorf("v2 %s, origin %s, want both at %s\n%s", r.at(branch), r.git(r.origin, "rev-parse", branch), tip, out)
	}
	if build, _ := os.ReadFile(filepath.Join(r.install, "rewake")); string(build) != "new build" {
		t.Errorf("the install holds %q", build)
	}
	if _, err := os.Stat(filepath.Join(r.install, "rewake.next")); !os.IsNotExist(err) {
		t.Errorf("rewake.next was left beside the build: %v", err)
	}
	for _, want := range []string{"fast-forward", "ok    race", "ok    workflow", "skip  shell", "pushed v2", "log: "} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	for _, call := range r.calls {
		if strings.Contains(call, "-race") && !strings.Contains(call, "-p 4") {
			t.Errorf("the race run %q is not -p 4 with no other run going", call)
		}
	}
}

// A red check takes the merge back: v2 where it was, nothing pushed, the
// build untouched, and the failure named.
func TestARedCheckResetsAndPushesNothing(t *testing.T) {
	r := newRepo(t)
	old := r.at(branch)
	r.branchOff("work", "a.go", "package a\n")
	r.red["race"] = true
	code, out := r.land("work")
	if code != exitFailed {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if r.at(branch) != old || r.git(r.origin, "rev-parse", branch) != old {
		t.Errorf("v2 or origin moved\n%s", out)
	}
	if build, _ := os.ReadFile(filepath.Join(r.install, "rewake")); string(build) != "old build" {
		t.Errorf("the install was replaced: %q", build)
	}
	for _, want := range []string{"FAIL  race     --- FAIL: TestSomething", "ok    workflow", "red: race; v2 reset to"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

// v2 moved on: a merge commit needs its subject, and carries both trailers
// of AGENTS.md; shell/ changed, so the shell is checked.
func TestADivergedBranchNeedsASubject(t *testing.T) {
	r := newRepo(t)
	r.branchOff("work", "shell/app.ts", "2\n")
	r.write("b.go", "package b\n")
	old := r.commit("Meanwhile on v2")
	if code, out := r.land("work"); code != exitCall || r.at(branch) != old {
		t.Fatalf("exit %d, v2 %s; want 2 and v2 untouched\n%s", code, r.at(branch), out)
	}
	code, out := r.land("work", "--subject", "Merge work into v2")
	if code != exitDone {
		t.Fatalf("exit %d\n%s", code, out)
	}
	message := r.git(r.root, "log", "-1", "--format=%B")
	want := "Merge work into v2\n\nCo-Authored-By: One <one@example.com>\nCo-Authored-By: Two <two@example.com>"
	if message != want {
		t.Errorf("the merge commit says %q, want %q", message, want)
	}
	if !slices.Contains(r.calls, "bun run check") {
		t.Errorf("shell/ changed and the shell was not checked: %v", r.calls)
	}
}

// A conflict aborts the merge: v2 and the tree as they were, no check run.
func TestAConflictLeavesTheTreeAsItWas(t *testing.T) {
	r := newRepo(t)
	r.branchOff("work", "a.go", "package work\n")
	r.write("a.go", "package main\n")
	old := r.commit("The same file on v2")
	code, out := r.land("work", "--subject", "Merge work into v2")
	if code != exitFailed || !strings.Contains(out, "conflicts in a.go") {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if r.at(branch) != old || r.git(r.root, "status", "--porcelain") != "" {
		t.Errorf("v2 %s, status %q after a conflict", r.at(branch), r.git(r.root, "status", "--porcelain"))
	}
	if len(r.calls) > 0 {
		t.Errorf("checks ran after a conflict: %v", r.calls)
	}
}

// --no-push checks and changes nothing.
func TestNoPushIsADryRun(t *testing.T) {
	r := newRepo(t)
	old := r.at(branch)
	r.branchOff("work", "a.go", "package a\n")
	code, out := r.land("work", "--no-push")
	if code != exitDone || r.at(branch) != old || r.git(r.origin, "rev-parse", branch) != old {
		t.Fatalf("exit %d, v2 %s\n%s", code, r.at(branch), out)
	}
	if slices.ContainsFunc(r.calls, func(c string) bool { return strings.HasPrefix(c, "go build") }) {
		t.Errorf("a dry run built: %v", r.calls)
	}
}

// Only from v2 with a clean tree: a reset after a red check would take
// changes along.
func TestRefusedOffV2OrWithChanges(t *testing.T) {
	r := newRepo(t)
	r.branchOff("work", "a.go", "package a\n")
	r.write("AGENTS.md", "changed\n")
	if code, out := r.land("work"); code != exitFailed || !strings.Contains(out, "has changes") {
		t.Errorf("a dirty tree: exit %d\n%s", code, out)
	}
	r.git(r.root, "checkout", "-q", "--", "AGENTS.md")
	r.git(r.root, "switch", "-q", "work")
	if code, out := r.land("work"); code != exitFailed || !strings.Contains(out, "not v2") {
		t.Errorf("off v2: exit %d\n%s", code, out)
	}
	if len(r.calls) > 0 {
		t.Errorf("checks ran: %v", r.calls)
	}
}

func TestHeavy(t *testing.T) {
	for _, tc := range []struct {
		argv []string
		want bool
	}{
		{[]string{"/usr/local/go/bin/go", "test", "-race", "./internal/cli/"}, true},
		{[]string{"go", "test", "-count=1", "./test/workflow/..."}, true},
		{[]string{"go", "test", "./internal/core/mail/"}, false},
		{[]string{"go", "run", "./tools/land", "work"}, false},
		{[]string{"rewake", "test", "-race"}, false},
	} {
		if got := heavy([][]string{tc.argv}); got != tc.want {
			t.Errorf("%v: %v, want %v", tc.argv, got, tc.want)
		}
	}
}

func TestParseArgs(t *testing.T) {
	if c, err := parseArgs([]string{"work", "--subject=Merge work into v2", "--no-push"}); err != nil || c.target != "work" || c.subject != "Merge work into v2" || !c.noPush {
		t.Errorf("got %+v %v", c, err)
	}
	for _, tc := range []struct {
		args []string
		says string
	}{
		{[]string{"--no-push"}, "name the branch"},
		{[]string{"a", "b"}, "one branch or commit only"},
		{[]string{"a", "--force"}, "no flag --force"},
		{[]string{"a", "--subject"}, "needs a value"},
		{[]string{"a", "--subject", "x\ny"}, "one line"},
		{[]string{"a", "--install", "rel"}, "absolute path"},
		{[]string{"a", "--no-push", "--no-push"}, "given twice"},
	} {
		if _, err := parseArgs(tc.args); err == nil || !strings.Contains(err.Error(), tc.says) {
			t.Errorf("%v: got %v, want %q", tc.args, err, tc.says)
		}
	}
}
