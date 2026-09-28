package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/grant"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

// grantWorld is a main caller, a Codex worker in a workspace of its own, and a
// home with directories to grant.
type grantWorld struct {
	dir, home, workspace string
	peer                 registry.Session
}

func newGrantWorld(t *testing.T, senderRole, harnessID string) grantWorld {
	t.Helper()
	dir, self, peer := stateCaller(t, senderRole)
	grantingMain(t, dir, self, os.Getpid())
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	world := grantWorld{dir: dir, home: filepath.Join(base, "home"), workspace: filepath.Join(base, "home", "work", "worker")}
	for _, sub := range []string{"work/worker/src", "work/lib/pkg", "work/other", ".ssh", "wide"} {
		if err := os.MkdirAll(filepath.Join(world.home, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", world.home)
	t.Setenv("PATH", "/usr/bin:/bin")
	peer.Role, peer.Harness, peer.CWD = "write", harnessID, world.workspace
	if err := registry.Update(dir, peer); err != nil {
		t.Fatal(err)
	}
	world.peer = peer
	return world
}

func (w grantWorld) sent(t *testing.T) []inbox.Message {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(state.InboxPath(w.dir, w.peer.Name), "*.json"))
	var messages []inbox.Message
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var message inbox.Message
		if err := json.Unmarshal(raw, &message); err != nil {
			t.Fatal(err)
		}
		messages = append(messages, message)
	}
	return messages
}

func TestDirGrantIsCarriedResolvedAndCollapsed(t *testing.T) {
	w := newGrantWorld(t, "main", "codex")
	lib, pkg, wide := filepath.Join(w.home, "work/lib"), filepath.Join(w.home, "work/lib/pkg"), filepath.Join(w.home, "wide")
	t.Chdir(filepath.Join(w.home, "work"))
	code, out, stderr := run("send", w.peer.Name, "Bump the client", "--wait=0", "--json",
		"--grant-dir", pkg, "--grant-dir=lib", "--grant-dir", "worker/src", "--grant-dir-broad", wide)
	if code != ExitPending {
		t.Fatalf("send: %d %s %s", code, out, stderr)
	}
	messages := w.sent(t)
	if len(messages) != 1 {
		t.Fatalf("%d messages sent", len(messages))
	}
	message := messages[0]
	if !slices.Equal(message.GrantDirs, []string{lib, wide}) || !slices.Equal(message.GrantBroad, []string{wide}) {
		t.Fatalf("carried %v, broad %v", message.GrantDirs, message.GrantBroad)
	}
	var model sendModel
	if err := json.Unmarshal([]byte(out), &model); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(w.workspace, "src")
	if !slices.Equal(model.GrantDirs, message.GrantDirs) || !slices.Equal(model.AlreadyWritable, []string{src}) {
		t.Fatalf("send --json: %s", out)
	}

	// The reader is told in the text of the task what it may write.
	if err := state.EnsureSubdir(state.UnreadPath(w.dir, w.peer.Name)); err != nil {
		t.Fatal(err)
	}
	from := filepath.Join(state.InboxPath(w.dir, w.peer.Name), message.ID+".json")
	if err := os.Link(from, filepath.Join(state.UnreadPath(w.dir, w.peer.Name), message.ID+".json")); err != nil {
		t.Fatal(err)
	}
	t.Setenv(state.SessionEnv, w.peer.Name)
	t.Setenv(state.EpochEnv, w.peer.Epoch())
	code, out, stderr = run("inbox")
	if code != ExitOK || !strings.Contains(out, "grant: write "+lib+"\ngrant: write "+wide+"\nBump the client") {
		t.Fatalf("inbox: %d %s %s", code, out, stderr)
	}
	code, out, _ = run("inbox", "--owed")
	if code != ExitOK || !strings.Contains(out, "grant: write "+lib+" — unless a turn typed") {
		t.Fatalf("inbox --owed lost the grant line: %s", out)
	}
	// Once the journal says a directory is gone, --owed stops promising it.
	if err := grant.Save(w.dir, w.peer.Name, w.peer.Epoch(), []grant.Entry{{Path: lib, Message: message.ID, Outcome: grant.Dropped}}); err != nil {
		t.Fatal(err)
	}
	code, out, _ = run("inbox", "--owed")
	if code != ExitOK || !strings.Contains(out, "grant: dropped "+lib+" — no longer writable") || !strings.Contains(out, "grant: write "+wide) {
		t.Fatalf("inbox --owed after the grant was dropped: %s", out)
	}
}

// A Claude Code session takes a grant through its permission hook, which
// asks its wrapper (docs/grants.md#claude-code): the task goes out like one to
// Codex.
func TestAClaudeCodeSessionIsSentADirectoryGrant(t *testing.T) {
	w := newGrantWorld(t, "main", "claude")
	lib := filepath.Join(w.home, "work/lib")
	code, out, stderr := run("send", w.peer.Name, "Bump the client", "--wait=0", "--grant-dir", lib)
	if code != ExitPending {
		t.Fatalf("send: %d %s %s", code, out, stderr)
	}
	if messages := w.sent(t); len(messages) != 1 || !slices.Equal(messages[0].GrantDirs, []string{lib}) {
		t.Fatalf("sent %+v", messages)
	}
}

func TestDirGrantInWorkspaceIsNotCarried(t *testing.T) {
	w := newGrantWorld(t, "main", "codex")
	code, out, stderr := run("send", w.peer.Name, "Tidy src", "--wait=0", "--grant-dir", filepath.Join(w.workspace, "src"))
	if code != ExitPending || !strings.Contains(out, "already writable by "+w.peer.Name) {
		t.Fatalf("send: %d %s %s", code, out, stderr)
	}
	if messages := w.sent(t); len(messages) != 1 || messages[0].GrantDirs != nil {
		t.Fatalf("a directory the worker writes already was carried: %+v", messages)
	}
}

func TestDirGrantRefusals(t *testing.T) {
	cases := []struct {
		name         string
		sender, peer string
		args         func(w grantWorld) []string
		code         int
		says         string
	}{
		{name: "writer sends", sender: "write", args: func(w grantWorld) []string { return []string{"--grant-dir", w.home + "/work/lib"} }, code: ExitUsage, says: "only a verified current main"},
		{name: "general sends", sender: "general", args: func(w grantWorld) []string { return []string{"--grant-dir", w.home + "/work/lib"} }, code: ExitUsage, says: "only a verified current main"},
		{name: "heads-up", args: func(w grantWorld) []string { return []string{"--notify", "--grant-dir", w.home + "/work/lib"} }, code: ExitUsage, says: "only a task or a question"},
		{name: "addendum", args: func(w grantWorld) []string { return []string{"--to", "abc", "--grant-dir", w.home + "/work/lib"} }, code: ExitUsage, says: "--to excludes it"},
		{name: "missing", args: func(w grantWorld) []string { return []string{"--grant-dir", w.home + "/work/nope"} }, code: ExitFailed, says: "no such directory"},
		{name: "keys", args: func(w grantWorld) []string { return []string{"--grant-dir", w.home + "/.ssh"} }, code: ExitUsage, says: "where login keys are kept"},
		{name: "rewake state", args: func(w grantWorld) []string { return []string{"--grant-dir", w.dir} }, code: ExitUsage, says: "rewake's state directory"},
		{name: "broad unconfirmed", args: func(w grantWorld) []string { return []string{"--grant-dir", w.home + "/wide"} }, code: ExitUsage, says: "--grant-dir-broad " + "{home}/wide"},
		{name: "narrow confirmed", args: func(w grantWorld) []string { return []string{"--grant-dir-broad", w.home + "/work/lib"} }, code: ExitUsage, says: "pass it with --grant-dir"},
		{name: "nine", args: func(w grantWorld) []string {
			var args []string
			for range 9 {
				args = append(args, "--grant-dir", w.home+"/work/lib")
			}
			return args
		}, code: ExitUsage, says: "at most 8 directories"},
		{name: "empty", args: func(grantWorld) []string { return []string{"--grant-dir", ""} }, code: ExitUsage, says: "needs a directory"},
		{name: "a live session's directory", args: func(w grantWorld) []string { return []string{"--grant-dir", w.home + "/work"} }, code: ExitUsage, says: "live rewake session"},
		{name: "wait twice", args: func(grantWorld) []string { return []string{"--wait=1"} }, code: ExitUsage, says: "--wait is given twice"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sender, peer := c.sender, c.peer
			if sender == "" {
				sender = "main"
			}
			if peer == "" {
				peer = "codex"
			}
			w := newGrantWorld(t, sender, peer)
			args := append([]string{"send", w.peer.Name, "Write there", "--wait=0"}, c.args(w)...)
			code, out, stderr := run(args...)
			says := strings.ReplaceAll(c.says, "{home}", w.home)
			if code != c.code || !strings.Contains(stderr, says) {
				t.Fatalf("got %d %s %s, want %d saying %q", code, out, stderr, c.code, says)
			}
			if messages := w.sent(t); len(messages) != 0 {
				t.Fatalf("a refused grant sent %d messages", len(messages))
			}
		})
	}
}

func TestRepeatableFlagsCollectAndOthersRefuseRepeats(t *testing.T) {
	result, err := parse([]string{"send", "x", "text", "--grant-dir", "a", "--grant-dir=b", "--grant-dir-broad", "c"})
	if err != nil {
		t.Fatal(err)
	}
	call := result.Call
	if !slices.Equal(call.Lists["grant-dir"], []string{"a", "b"}) || !slices.Equal(call.Lists["grant-dir-broad"], []string{"c"}) {
		t.Fatalf("lists = %v", call.Lists)
	}
	for _, argv := range [][]string{
		{"send", "x", "text", "--wait", "1", "--wait=2"},
		{"send", "x", "text", "--json", "--json"},
		{"inbox", "--message", "a", "--message", "b"},
	} {
		if _, err := parse(argv); err == nil || !strings.Contains(err.Error(), "given twice") {
			t.Errorf("%v: got %v, want a refusal of the repeat", argv, err)
		}
	}
	// Every option the guide shows as repeatable really collects.
	for _, group := range Groups() {
		for _, command := range group.Commands {
			for _, option := range command.Options {
				if option.Repeatable && option.Value == "" {
					t.Errorf("%s %s repeats without a value", command.Name, option.Flag)
				}
			}
		}
	}
}

// The refusal comes before anything else of the launch: a program that does
// not exist would be refused next, with a text that names no grant.
func TestCodexLaunchRefusesAddDir(t *testing.T) {
	for _, args := range [][]string{
		{"--command", "/nonexistent/codex", "codex", "--add-dir", "/tmp"},
		{"--command", "/nonexistent/codex", "codex", "-c", "sandbox_workspace_write.writable_roots=[\"/tmp\"]"},
	} {
		code, out, stderr := run(args...)
		if code != ExitUsage || !strings.Contains(stderr, "rewake send --grant-dir") {
			t.Errorf("%q: got %d %s %s", args, code, out, stderr)
		}
	}
}

// An addition to a task whose grant waits for the worker to be idle would be
// read first, and worked on without the grant: --to refuses it and names
// rewake edit, which changes the task itself.
func TestAnAddendumToAWaitingGrantIsRefused(t *testing.T) {
	w := newGrantWorld(t, "main", "codex")
	code, out, stderr := run("send", w.peer.Name, "Write there", "--wait=0", "--grant-dir", w.home+"/work/lib")
	if code != ExitPending {
		t.Fatalf("send: %d %s %s", code, out, stderr)
	}
	task := w.sent(t)[0]
	code, out, stderr = run("send", w.peer.Name, "and this too", "--wait=0", "--to", task.ID)
	if code != ExitFailed || !strings.Contains(stderr, "carries a grant") || !strings.Contains(stderr, "rewake edit") {
		t.Fatalf("addendum: %d %s %s", code, out, stderr)
	}
	if messages := w.sent(t); len(messages) != 1 {
		t.Fatalf("the refused addendum was written: %d messages", len(messages))
	}
}
