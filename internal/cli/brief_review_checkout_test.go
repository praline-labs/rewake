package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/taskbrief"
	"github.com/praline-labs/rewake/internal/host/worktree"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// send --brief --review --commit gives the reviewer a detached checkout of
// the commit main named, in the repository main runs in: the letter and
// brief show name its path, and the brief's close removes it.
func TestAReviewerGetsACheckoutOfTheNamedCommit(t *testing.T) {
	dir, web, api := briefSender(t)
	for _, name := range []string{"GIT_CONFIG_GLOBAL", "GIT_CONFIG_SYSTEM"} {
		t.Setenv(name, os.DevNull)
	}
	for _, name := range []string{"GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(name, "test")
	}
	t.Setenv(worktree.RootEnv, t.TempDir())
	repo := t.TempDir()
	git := func(args ...string) string {
		out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q", "-b", "v2")
	git("commit", "-q", "--allow-empty", "-m", "First")
	git("switch", "-q", "-c", "work")
	git("commit", "-q", "--allow-empty", "-m", "Reviewed")
	commit := git("rev-parse", "HEAD")
	git("switch", "-q", "v2")
	record, err := registry.Load(dir, web.Name)
	if err != nil {
		t.Fatal(err)
	}
	record.CWD = repo
	encoded, _ := json.MarshalIndent(record, "", "  ")
	if err := os.WriteFile(state.SessionPath(dir, web.Name), encoded, 0o600); err != nil {
		t.Fatal(err)
	}

	if code, _, errOut := run("send", "api", "review it", "--brief", "1", "--commit", "work"); code != ExitUsage || !strings.Contains(errOut, "--brief <id> --review") {
		t.Fatalf("--commit without --review: %d %q", code, errOut)
	}
	if code, _, errOut := run("send", "api", "review it", "--brief", "1", "--review", "--commit", "no-such"); code != ExitUsage || !strings.Contains(errOut, "names no commit") {
		t.Fatalf("--commit of nothing: %d %q", code, errOut)
	}
	id := sendOK(t, "api", "review it", "--brief", "1", "--review", "--commit", "work")
	brief, err := taskbrief.Load(taskbrief.Home{Room: dir}, 1)
	binding, _ := brief.BindingOf(taskbrief.Author{Name: api.Name, Epoch: api.Epoch()})
	if err != nil || binding.Kind != taskbrief.Review || binding.Commit != commit || binding.Checkout == "" {
		t.Fatalf("bound = %+v, %v", brief.Bound, err)
	}
	checkout := binding.Checkout
	if head, err := exec.Command("git", "-C", checkout, "rev-parse", "HEAD").Output(); err != nil || strings.TrimSpace(string(head)) != commit {
		t.Fatalf("the checkout's HEAD = %s, %v", head, err)
	}
	leaveUnread(t, dir, mail.Message{ID: id, From: web.Name, FromEpoch: web.Epoch(), To: api.Name, ToEpoch: api.Epoch(), Kind: mail.Task, Text: "review it", Brief: "1"})

	as(t, api)
	if _, out, _ := run("inbox"); !strings.Contains(out, "Rewake: review in "+checkout+", a checkout of its own detached at "+commit[:7]) {
		t.Fatalf("the reviewer's inbox = %q", out)
	}
	if _, out, _ := run("brief", "show", "1"); !strings.Contains(out, "review in "+checkout) {
		t.Fatalf("the reviewer's show = %q", out)
	}

	as(t, web)
	if code, out, errOut := run("brief", "close", "1"); code != ExitOK || strings.Contains(out, "stays") {
		t.Fatalf("close: %d %q %q", code, out, errOut)
	}
	if _, err := os.Stat(checkout); !os.IsNotExist(err) {
		t.Errorf("the reviewer's checkout outlived the brief: %v", err)
	}
	if records, _ := filepath.Glob(filepath.Join(os.Getenv(worktree.RootEnv), "*", "*.json")); len(records) != 0 {
		t.Errorf("records left: %v", records)
	}
}
