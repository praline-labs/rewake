package codex

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/proc"
	"github.com/iiiokojiadbi/rewake/internal/registry"
)

// codexHome writes a Codex home with the given config.toml content.
func codexHome(t *testing.T, config string) string {
	t.Helper()
	home := t.TempDir()
	if config != "" {
		if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(config), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
	}
	t.Setenv("CODEX_HOME", home)
	return home
}

func configValue(args []string, key string) (string, bool) {
	for index, arg := range args {
		if arg == configFlag && index+1 < len(args) && strings.HasPrefix(args[index+1], key+"=") {
			return strings.TrimPrefix(args[index+1], key+"="), true
		}
	}
	return "", false
}

func TestIntroIsPassedForOneLaunch(t *testing.T) {
	codexHome(t, "")

	plan, err := New().Launch(harness.LaunchRequest{Name: "web", Dir: t.TempDir(), Intro: true})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}

	value, ok := configValue(plan.Args, introKey)
	if !ok {
		t.Fatalf("the briefing was not passed: %v", plan.Args)
	}
	if !strings.Contains(value, `web`) || !strings.Contains(value, "rewake send") {
		t.Errorf("briefing = %s, want the session name and how to answer", value)
	}
}

// The key replaces the user's value rather than adding to it, so a briefing that
// ignored what is configured would quietly drop the user's own instructions.
func TestIntroKeepsTheUserInstructions(t *testing.T) {
	codexHome(t, "developer_instructions = \"Always answer in French.\"\n")

	plan, err := New().Launch(harness.LaunchRequest{Name: "web", Dir: t.TempDir(), Intro: true})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}

	value, _ := configValue(plan.Args, introKey)
	if !strings.Contains(value, "Always answer in French.") {
		t.Errorf("the user's instructions were dropped: %s", value)
	}
	if !strings.Contains(value, "rewake") {
		t.Errorf("the briefing was dropped: %s", value)
	}
}

func TestIntroKeepsAMultilineUserValue(t *testing.T) {
	codexHome(t, "developer_instructions = \"\"\"\nfirst line\nsecond line\n\"\"\"\n")

	plan, err := New().Launch(harness.LaunchRequest{Name: "web", Dir: t.TempDir(), Intro: true})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}

	value, _ := configValue(plan.Args, introKey)
	for _, want := range []string{"first line", "second line"} {
		if !strings.Contains(value, want) {
			t.Errorf("multi-line value lost %q: %s", want, value)
		}
	}
}

func TestIntroIsSkippedWhenNotWanted(t *testing.T) {
	codexHome(t, "")

	plan, err := New().Launch(harness.LaunchRequest{Name: "web", Dir: t.TempDir(), Intro: false})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if _, ok := configValue(plan.Args, introKey); ok {
		t.Errorf("--no-intro still sent a briefing: %v", plan.Args)
	}
}

func TestCallerConfigWins(t *testing.T) {
	codexHome(t, "")

	plan, err := New().Launch(harness.LaunchRequest{
		Name:  "web",
		Dir:   t.TempDir(),
		Intro: true,
		Args:  []string{configFlag, introKey + `="mine"`},
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if count := strings.Count(strings.Join(plan.Args, " "), introKey+"="); count != 1 {
		t.Errorf("the key was set %d times, want the caller's only: %v", count, plan.Args)
	}
}

// With /tmp excluded from the sandbox, an agent cannot write a message file at
// all — and that is the only way it can answer anybody.
func TestStateDirectoryIsMadeWritableWhenTmpIsExcluded(t *testing.T) {
	codexHome(t, "[sandbox_workspace_write]\nexclude_slash_tmp = true\nwritable_roots = [\"/var/data\"]\n")
	dir := t.TempDir()

	plan, err := New().Launch(harness.LaunchRequest{Name: "web", Dir: dir, Intro: false})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}

	value, ok := configValue(plan.Args, "sandbox_workspace_write.writable_roots")
	if !ok {
		t.Fatalf("the state directory was not made writable: %v", plan.Args)
	}
	if !strings.Contains(value, dir) {
		t.Errorf("roots = %s, want the state directory", value)
	}
	if !strings.Contains(value, "/var/data") {
		t.Errorf("roots = %s, want the configured roots kept", value)
	}
}

func TestWritableRootsAreLeftAloneByDefault(t *testing.T) {
	codexHome(t, "[sandbox_workspace_write]\nnetwork_access = true\n")

	plan, err := New().Launch(harness.LaunchRequest{Name: "web", Dir: t.TempDir()})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if _, ok := configValue(plan.Args, "sandbox_workspace_write.writable_roots"); ok {
		t.Errorf("the sandbox configuration was changed for no reason: %v", plan.Args)
	}
}

// threadFixture builds a /proc-shaped tree whose process holds lock files open.
func threadFixture(t *testing.T, home string, threads map[string]time.Time) int {
	t.Helper()
	root := t.TempDir()
	const pid = 7

	statDir := filepath.Join(root, "7")
	if err := os.MkdirAll(filepath.Join(statDir, "fd"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	line := "7 (codex) S 1"
	for field := 5; field <= 21; field++ {
		line += " 0"
	}
	line += " 100 0 0\n"
	if err := os.WriteFile(filepath.Join(statDir, "stat"), []byte(line), 0o644); err != nil {
		t.Fatalf("write stat: %v", err)
	}

	locks := filepath.Join(home, lockDir)
	if err := os.MkdirAll(locks, 0o700); err != nil {
		t.Fatalf("mkdir locks: %v", err)
	}
	descriptor := 3
	for thread, touched := range threads {
		path := filepath.Join(locks, thread+".lock")
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatalf("write lock: %v", err)
		}
		if err := os.Chtimes(path, touched, touched); err != nil {
			t.Fatalf("chtimes: %v", err)
		}
		if err := os.Symlink(path, filepath.Join(statDir, "fd", itoa(descriptor))); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		descriptor++
	}

	swapProcRoot(t, root)
	return pid
}

// swapProcRoot points the process reader at a fixture for the test's lifetime.
func swapProcRoot(t *testing.T, root string) {
	t.Helper()
	previous := proc.Default
	proc.Default = proc.Reader{Root: root}
	t.Cleanup(func() { proc.Default = previous })
}

// writeProcess adds one process to a /proc fixture, holding one lock file open.
func writeProcess(t *testing.T, root string, pid, parent int, lock string, touched time.Time) {
	t.Helper()
	dir := filepath.Join(root, strconv.Itoa(pid))
	if err := os.MkdirAll(filepath.Join(dir, "fd"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	line := strconv.Itoa(pid) + " (codex) S " + strconv.Itoa(parent)
	for field := 5; field <= 21; field++ {
		line += " 0"
	}
	line += " 100 0 0\n"
	if err := os.WriteFile(filepath.Join(dir, "stat"), []byte(line), 0o644); err != nil {
		t.Fatalf("write stat: %v", err)
	}
	if err := os.WriteFile(lock, nil, 0o600); err != nil {
		t.Fatalf("write lock: %v", err)
	}
	if err := os.Chtimes(lock, touched, touched); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	if err := os.Symlink(lock, filepath.Join(dir, "fd", "3")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
}

func itoa(value int) string { return strconv.Itoa(value) }

// After /new the process holds both lock files open. The newer one is the thread
// the session is actually writing to; answering with the other sends the message
// into a conversation nobody is looking at.
func TestCurrentThreadTakesTheNewestLock(t *testing.T) {
	home := t.TempDir()
	older := time.Now().Add(-time.Hour)
	newer := time.Now().Add(-time.Minute)
	pid := threadFixture(t, home, map[string]time.Time{
		"01a0a6e5-f5cc-70a2-97c4-bc0df35734c6": older,
		"01a0a6e6-276b-7bf0-b74a-e3442e6a53dc": newer,
	})

	thread, err := CurrentThread(pid, home)
	if err != nil {
		t.Fatalf("CurrentThread: %v", err)
	}
	if thread != "01a0a6e6-276b-7bf0-b74a-e3442e6a53dc" {
		t.Errorf("thread = %q, want the newest lock", thread)
	}
}

func TestCurrentThreadWithoutALockSaysSo(t *testing.T) {
	home := t.TempDir()
	pid := threadFixture(t, home, nil)

	if _, err := CurrentThread(pid, home); err == nil {
		t.Fatal("a session with no open thread reported one")
	}
}

func TestDeliveryWithoutAThreadIsPending(t *testing.T) {
	home := t.TempDir()
	pid := threadFixture(t, home, nil)

	result := New().Deliver(context.Background(), registry.Session{HarnessPID: pid, CodexHome: home}, inbox.Message{ID: "1789-aa", From: "api", Text: "hello"})
	if result.State != inbox.Pending {
		t.Fatalf("result = %+v, want pending until codex opens a thread", result)
	}
}

func TestDeliverySaysTheQueueIsNotInstant(t *testing.T) {
	home := t.TempDir()
	pid := threadFixture(t, home, map[string]time.Time{"01a0-thread": time.Now()})

	var sawThread, sawText string
	previous := queue
	queue = func(_ context.Context, _, thread, text string) (string, error) {
		sawThread, sawText = thread, text
		return "", nil
	}
	t.Cleanup(func() { queue = previous })

	result := New().Deliver(context.Background(), registry.Session{HarnessPID: pid, CodexHome: home},
		inbox.Message{ID: "1789-aa", From: "api", Text: "pull and rerun the smoke"})

	if result.State != inbox.Delivered || result.Via != "codex queue" {
		t.Fatalf("result = %+v, want delivered via the queue", result)
	}
	if result.Detail == "" {
		t.Error("the result does not warn that delivery is not instant")
	}
	if sawThread != "01a0-thread" {
		t.Errorf("thread = %q, want the open one", sawThread)
	}
	if !strings.Contains(sawText, "pull and rerun the smoke") || !strings.Contains(sawText, "rewake send api") {
		t.Errorf("queued text = %q, want the message and how to answer", sawText)
	}
}

// The three answers codex queue gives mean three different things to a sender.
func TestQueueFailuresAreClassified(t *testing.T) {
	cases := []struct {
		name   string
		output string
		want   inbox.State
		says   string
	}{
		{
			name:   "no conversation yet",
			output: "Error: failed to queue session message: thread/queue/add failed: failed to read thread: invalid thread-store request: no rollout found for thread id 01a0 (code -32603)",
			want:   inbox.Pending,
			says:   "first turn",
		},
		{
			name:   "thread gone",
			output: "Error: No active session found matching '01a0'.",
			want:   inbox.Failed,
			says:   "no longer knows",
		},
		{
			name:   "anything else",
			output: "Error: database is locked",
			want:   inbox.Failed,
			says:   "database is locked",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			result := classify(testCase.output, errors.New("exit status 1"))
			if result.State != testCase.want {
				t.Errorf("state = %q, want %q", result.State, testCase.want)
			}
			if !strings.Contains(result.Detail, testCase.says) {
				t.Errorf("detail = %q, want it to mention %q", result.Detail, testCase.says)
			}
		})
	}
}
