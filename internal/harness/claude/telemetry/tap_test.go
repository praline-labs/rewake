package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The tap replaces its own process, so it is tested as a process: the test
// binary runs itself as the tap when this variable names the socket.
const (
	tapSocketEnv  = "REWAKE_TEST_TAP_SOCKET"
	tapCommandEnv = "REWAKE_TEST_TAP_COMMAND"
	tapPipeEnv    = "REWAKE_TEST_TAP_MAX_PIPE"
	tapSourcesEnv = "REWAKE_TEST_TAP_SOURCES"
)

func TestMain(m *testing.M) {
	if socket, ok := os.LookupEnv(tapSocketEnv); ok {
		if limit := os.Getenv(tapPipeEnv); limit != "" {
			maxTapPipe, _ = strconv.Atoi(limit)
		}
		// The command arrives the way a caller's --settings hands it on, and
		// no settings file is read unless a test names the sources: the tap
		// must never find the status line of the person running the tests.
		caller := ""
		if command := os.Getenv(tapCommandEnv); command != "" {
			encoded, _ := json.Marshal(map[string]string{"type": "command", "command": command})
			caller = string(encoded)
		}
		sources, named := os.LookupEnv(tapSourcesEnv)
		if !named {
			sources = NoSources
		}
		os.Exit(RunTap(socket, sources, caller))
	}
	os.Exit(m.Run())
}

type tapRun struct {
	stdout, stderr string
	code           int
}

func runTap(t *testing.T, socket, command string, stdin []byte, env ...string) tapRun {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tap := exec.CommandContext(ctx, os.Args[0])
	tap.Env = append(os.Environ(), append([]string{tapSocketEnv + "=" + socket, tapCommandEnv + "=" + command}, env...)...)
	tap.Stdin = bytes.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	tap.Stdout, tap.Stderr = &stdout, &stderr
	err := tap.Run()
	code := 0
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatalf("running the tap: %v", err)
	}
	return tapRun{stdout.String(), stderr.String(), code}
}

const statusJSON = `{"session_id":"s1","model":{"id":"m","display_name":"M"},"effort":{"level":"high"},` +
	`"context_window":{"total_input_tokens":34727,"context_window_size":200000,"current_usage":{"input_tokens":10},"used_percentage":17}}` + "\n"

// The person's command gets exactly what the harness wrote, and its output,
// its error output and its exit code come back unchanged — on both paths, the
// exec and the child the tap falls back to.
func TestTheTapPassesEverythingThrough(t *testing.T) {
	dir := socketDir(t)
	copied := filepath.Join(dir, "stdin")
	command := "cat > " + copied + "; printf 'line one\\n\\nline three'; echo oops >&2; exit 3"
	for _, path := range []struct {
		name string
		env  []string
	}{{"exec", nil}, {"child", []string{tapPipeEnv + "=1"}}} {
		t.Run(path.name, func(t *testing.T) {
			run := runTap(t, filepath.Join(dir, "missing.obs"), command, []byte(statusJSON), path.env...)
			if run.stdout != "line one\n\nline three" || run.stderr != "oops\n" || run.code != 3 {
				t.Errorf("got stdout %q stderr %q code %d", run.stdout, run.stderr, run.code)
			}
			if got, _ := os.ReadFile(copied); string(got) != statusJSON {
				t.Errorf("the command read %q, want the harness's payload unchanged", got)
			}
		})
	}
}

// The command runs in the tap's directory and environment — which are the
// ones the harness gave the status line.
func TestTheTapKeepsDirectoryAndEnvironment(t *testing.T) {
	dir := socketDir(t)
	tap := exec.Command(os.Args[0])
	tap.Dir = dir
	tap.Env = append(os.Environ(), tapSocketEnv+"=", tapCommandEnv+`=echo "$PWD $CLAUDE_PROJECT_DIR"`, "CLAUDE_PROJECT_DIR=/p")
	out, err := tap.Output()
	if err != nil {
		t.Fatal(err)
	}
	if want := dir + " /p\n"; string(out) != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// No configured status line: nothing printed, success — what the harness
// shows without one.
func TestTheTapWithoutACommandPrintsNothing(t *testing.T) {
	run := runTap(t, "", "", []byte(statusJSON))
	if run.stdout != "" || run.stderr != "" || run.code != 0 {
		t.Errorf("got %+v, want nothing", run)
	}
}

// Nothing of rewake's stands in the way: input it cannot parse, input in a
// shape it does not know, and a wrapper that is gone all leave the person's
// command running as before.
func TestTheTapPassesThroughWhateverRewakeCannotDo(t *testing.T) {
	dir := socketDir(t)
	for name, input := range map[string]string{
		"garbage":       "\x00\x01 not json at all",
		"changed shape": `{"model":"m","context_window":[1,2,3],"effort":"high"}`,
		"empty":         "",
	} {
		t.Run(name, func(t *testing.T) {
			run := runTap(t, filepath.Join(dir, "gone.obs"), "cat; echo; echo shown", []byte(input))
			if run.stdout != input+"\nshown\n" || run.code != 0 {
				t.Errorf("got %+v", run)
			}
		})
	}
}

// The tap reports what the status line says to a live collector.
func TestTheTapReportsToTheWrapper(t *testing.T) {
	path := filepath.Join(socketDir(t), "s.obs")
	collector := NewCollector(path)
	if err := collector.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer collector.Close()
	if run := runTap(t, path, "echo shown", []byte(statusJSON)); run.stdout != "shown\n" {
		t.Fatalf("got %+v", run)
	}
	waitFor(t, "the status", func() bool { return collector.SessionState().ContextUsed != nil })
	snapshot := collector.SessionState()
	if *snapshot.Model != "m" || *snapshot.Effort != "high" || *snapshot.ContextUsed != 34727 || *snapshot.FilledPercent != 17 {
		t.Errorf("snapshot = %+v", snapshot)
	}
}

// A terminating signal reaches the person's command on both paths: after the
// exec it is the command; on the fallback the tap passes it on.
func TestATimedOutStatusLineIsStopped(t *testing.T) {
	for _, path := range []struct {
		name string
		env  []string
	}{{"exec", nil}, {"child", []string{tapPipeEnv + "=1"}}} {
		t.Run(path.name, func(t *testing.T) {
			tap := exec.Command(os.Args[0])
			tap.Env = append(os.Environ(), append([]string{tapSocketEnv + "=", tapCommandEnv + "=echo started; exec sleep 30"}, path.env...)...)
			tap.Stdin = strings.NewReader(statusJSON)
			stdout, err := tap.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := tap.Start(); err != nil {
				t.Fatal(err)
			}
			buffer := make([]byte, 8)
			if _, err := stdout.Read(buffer); err != nil {
				t.Fatal(err)
			}
			_ = tap.Process.Signal(syscall.SIGTERM)
			done := make(chan error, 1)
			go func() { done <- tap.Wait() }()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				_ = tap.Process.Kill()
				t.Fatal("the status line outlived its termination")
			}
		})
	}
}

// The person's status line is found when the tap runs, from the tap's own
// environment: a configuration directory set only there — by a script that
// starts the harness, out of the rewake wrapper's sight — is the one read.
func TestTheTapFindsTheStatusLineFromItsOwnEnvironment(t *testing.T) {
	dir := socketDir(t)
	config := filepath.Join(dir, "worker-config")
	project := filepath.Join(dir, "project")
	writeSettings(t, filepath.Join(config, "settings.json"), `{"statusLine":{"type":"command","command":"echo from-config-dir"}}`)
	writeSettings(t, filepath.Join(dir, "home", ".claude", "settings.json"), `{"statusLine":{"type":"command","command":"echo from-home"}}`)
	env := []string{tapSourcesEnv + "=" + AllSources, "HOME=" + filepath.Join(dir, "home"), "CLAUDE_PROJECT_DIR=" + project}

	// Cleared, so a configuration directory of whoever runs the tests is not read.
	if run := runTap(t, "", "", []byte(statusJSON), append(env, "CLAUDE_CONFIG_DIR=")...); run.stdout != "from-home\n" {
		t.Errorf("without a config dir: %+v", run)
	}
	if run := runTap(t, "", "", []byte(statusJSON), append(env, "CLAUDE_CONFIG_DIR="+config)...); run.stdout != "from-config-dir\n" {
		t.Errorf("with a config dir set only for the tap: %+v", run)
	}
	writeSettings(t, filepath.Join(project, ".claude", "settings.local.json"), `{"statusLine":{"command":"echo from-local"}}`)
	if run := runTap(t, "", "", []byte(statusJSON), append(env, "CLAUDE_CONFIG_DIR="+config)...); run.stdout != "from-local\n" {
		t.Errorf("a local layer over the user's: %+v", run)
	}
}

func writeSettings(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
