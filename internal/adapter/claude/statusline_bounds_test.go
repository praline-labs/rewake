package claude

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// withPersonalLine sets the workers' settings to run command as the person's
// status line, and the deadlines short.
func withPersonalLine(t *testing.T, command string) {
	t.Helper()
	folder := t.TempDir()
	settings := `{"statusLine":{"type":"command","command":` + strconv.Quote(command) + `}}`
	if err := os.WriteFile(filepath.Join(folder, "settings.json"), []byte(settings), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", folder)
	t.Setenv(modSocketEnv, "")
	t.Setenv(statusLineGuardEnv, "")
	shortenStatusLine(t, 400*time.Millisecond, 200*time.Millisecond)
}

func shortenStatusLine(t *testing.T, line, post time.Duration) {
	t.Helper()
	oldLine, oldPost := statusLineTimeout, postTimeout
	statusLineTimeout, postTimeout = line, post
	t.Cleanup(func() { statusLineTimeout, postTimeout = oldLine, oldPost })
}

// A personal command that outlives its deadline is killed with the children
// it started, and its late output is not printed: nothing it left behind
// holds the harness's output open.
func TestAPersonalLineThatOutlivesItsDeadlineLeavesNothingRunning(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child")
	withPersonalLine(t, "echo early; (sleep 30; echo late) & echo $! > "+pidFile+"; wait")
	var out bytes.Buffer
	started := time.Now()
	claudeHarness{}.StatusLine(context.Background(), []byte(`{}`), &out)
	if took := time.Since(started); took > 3*time.Second {
		t.Fatalf("the command held the line for %v", took)
	}
	if strings.Contains(out.String(), "late") {
		t.Fatalf("a late line was printed: %q", out.String())
	}
	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	for range 50 {
		if syscall.Kill(pid, 0) != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	t.Fatalf("the child %d is still running", pid)
}

// A host that takes the telemetry and never answers does not spend the
// person's line's time.
func TestAStalledPostDoesNotStarveThePersonsLine(t *testing.T) {
	withPersonalLine(t, "printf PERSONAL_LINE")
	socket := filepath.Join(t.TempDir(), "s")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			t.Cleanup(func() { _ = conn.Close() })
		}
	}()
	t.Setenv(modSocketEnv, socket)
	shortenStatusLine(t, 3*time.Second, 300*time.Millisecond)
	var out bytes.Buffer
	started := time.Now()
	claudeHarness{}.StatusLine(context.Background(), []byte(`{"prompt_cache":{"warm":true}}`), &out)
	if out.String() != "PERSONAL_LINE" || time.Since(started) > 2*time.Second {
		t.Fatalf("the person's line: %q after %v", out.String(), time.Since(started))
	}
}

// A person's line that is rewake's own statusline calls it again with the
// mark set; the second prints and posts nothing, so the first ends.
func TestAStatusLineCalledByThePersonsLineDoesNothing(t *testing.T) {
	withPersonalLine(t, "echo ran; env | grep -c "+statusLineGuardEnv+"=1")
	var out bytes.Buffer
	claudeHarness{}.StatusLine(context.Background(), []byte(`{}`), &out)
	if out.String() != "ran\n1\n" {
		t.Fatalf("the personal command's environment: %q", out.String())
	}
	t.Setenv(statusLineGuardEnv, "1")
	out.Reset()
	claudeHarness{}.StatusLine(context.Background(), []byte(`{}`), &out)
	if out.Len() != 0 {
		t.Fatalf("a nested status line printed %q", out.String())
	}
}
