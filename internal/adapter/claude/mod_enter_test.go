package claude

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/infra/proxyenv"
)

// enterProbe calls EnterWorktree from session.start as worktree_enter does,
// as the plugin rewake, writes what it answered and drops the prompt, so no
// model is asked and no sign-in is needed.
const enterProbe = `import type { Register } from "claude-code";
import { serveWorktreeEnter } from "./enter.ts";
export const register: Register = (on) => {
  serveWorktreeEnter(on);
  on("session.start", async ($, e, next) => {
    const started = await next(e);
    let answer: unknown;
    try { answer = await $.tool.call({ tool: "EnterWorktree", path: "/nonexistent/rewake-enter-probe" }); }
    catch (error) { answer = { error: String(error) }; }
    const output = await $.env.get("ENTER_PROBE_OUTPUT");
    if (output) await $.fs.write(output, JSON.stringify(answer));
    return started;
  });
  on("prompt.submit", () => ({ drop: "no model turn in this probe" }));
};
`

// enterOnce runs the installed harness once over the mod's own enter.ts in a
// private home, with the settings the case gives, $DENY standing for a
// PreToolUse hook that denies, and returns what rewake's call answered.
func enterOnce(t *testing.T, program, settings string) string {
	t.Helper()
	base := t.TempDir()
	plugin := filepath.Join(base, "plugin")
	files := map[string]string{
		".claude-plugin/plugin.json": `{"name": "rewake", "version": "0.0.0", "description": "worktree entry probe"}`,
		"hooks/hooks.json":           `{"modules": ["./probe.ts"]}`,
		"hooks/probe.ts":             enterProbe,
	}
	for _, name := range []string{"enter.ts", "worktree.ts"} {
		data, err := modFiles.ReadFile("mod/hooks/" + name)
		if err != nil {
			t.Fatal(err)
		}
		files["hooks/"+name] = string(data)
	}
	files["../deny.sh"] = "#!/bin/sh\nprintf '%s\\n' '{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"policy refuses worktree relocation\"}}'\n"
	files["../settings.json"] = strings.ReplaceAll(settings, "$DENY", filepath.Join(base, "deny.sh"))
	for name, text := range files {
		path := filepath.Join(plugin, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	work := filepath.Join(base, "work")
	if err := os.MkdirAll(work, 0o700); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(base, "answer.json")
	command := exec.Command(program, "-p", "probe", "--plugin-dir", plugin,
		"--settings", filepath.Join(base, "settings.json"), "--setting-sources", "")
	command.Dir = work
	// The machine's proxy, its only way out, and nothing else of this
	// process's environment.
	command.Env = append([]string{
		"HOME=" + base, "CLAUDE_CONFIG_DIR=" + filepath.Join(base, "config"), "PATH=" + os.Getenv("PATH"),
		"REWAKE_WORKTREE_ENTER=1", "ENTER_PROBE_OUTPUT=" + output,
	}, proxyenv.Of(os.Environ())...)
	out := runBounded(t, command, time.Minute, filepath.Join(base, "output.txt"))
	answer, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("the probe wrote no answer: %v\n%s", err, out)
	}
	return string(answer)
}

// runBounded runs the command in a group of its own, its output in the file
// log, and fails the test with that output once limit passes. It never waits
// on the harness past the limit and a few seconds: a process with a thread
// stuck in the kernel cannot be reaped, so Wait, WaitDelay or not, would hang
// the suite (October 21, 2026: claude -p stuck in rtnl_dumpit while the
// machine's network device failed to come up; ip addr hung the same).
func runBounded(t *testing.T, command *exec.Cmd, limit time.Duration, log string) []byte {
	t.Helper()
	file, err := os.Create(log)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	// A file, not a pipe: no copying goroutine waits for a child that keeps
	// it open.
	command.Stdout, command.Stderr = file, file
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	// What the harness started goes with it, on time or not.
	defer func() { _ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL) }()
	done := make(chan struct{})
	go func() {
		_ = command.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(limit):
		_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
		out, _ := os.ReadFile(log)
		t.Fatalf("%s ran past %s and was killed; output:\n%s", command.Path, limit, out)
	}
	out, _ := os.ReadFile(log)
	return out
}

// rewake's own EnterWorktree gets past the ask Claude Code puts to a person
// before a move outside .claude/worktrees, with the allow rule a worker's
// launch writes, and the person's policy still holds: a PreToolUse deny, an
// ask rule (found in review, October 20, 2026).
func TestWorktreeEnterKeepsThePersonsPolicy(t *testing.T) {
	program := claudeProgram(t)
	// The tool itself refuses the made-up path; what matters is that it ran.
	allowed := `{"permissions": {"allow": ["EnterWorktree"]}}`
	if answer := enterOnce(t, program, allowed); strings.Contains(answer, `"deny"`) || !strings.Contains(answer, "Cannot enter") {
		t.Fatalf("with the launch's allow rule, rewake's call did not reach the tool: %s", answer)
	}
	denied := `{"permissions": {"allow": ["EnterWorktree"]}, "hooks": {"PreToolUse": [{"matcher": "EnterWorktree", "hooks": [{"type": "command", "command": "$DENY"}]}]}}`
	if answer := enterOnce(t, program, denied); !strings.Contains(answer, `"deny"`) || !strings.Contains(answer, "policy refuses worktree relocation") {
		t.Fatalf("the PreToolUse deny did not hold: %s", answer)
	}
	asked := `{"permissions": {"ask": ["EnterWorktree"]}}`
	if answer := enterOnce(t, program, asked); !strings.Contains(answer, `"deny"`) {
		t.Fatalf("the ask rule was answered for the person: %s", answer)
	}
}
