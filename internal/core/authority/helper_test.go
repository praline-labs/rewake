package authority

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
)

// A test process started again as a helper: another process for the
// protocol to face, where the check under test is about who the peer is.
const (
	// helperAsk sends a decision straight to the address, as a forger would.
	helperAsk = "REWAKE_AUTHORITY_ASK"
	// helperDecisions listens as a run's wrapper, taking decisions.
	helperDecisions = "REWAKE_AUTHORITY_DECISIONS"
)

// TestHelperProcess is not a test: it is what a helper runs.
func TestHelperProcess(t *testing.T) {
	if address := os.Getenv(helperAsk); address != "" {
		// The raw request, not Decide: a forger skips the client's own
		// checks, so only the wrapper's side is under test.
		conn, err := net.Dial("unix", address)
		if err != nil {
			fmt.Println("dial:", err)
			os.Exit(0)
		}
		_ = json.NewEncoder(conn).Encode(requestOf(ask(mail.ChoiceArrived)))
		line, _ := bufio.NewReader(io.LimitReader(conn, maxLine)).ReadBytes('\n')
		fmt.Print(string(line))
		os.Exit(0)
	}
	if address := os.Getenv(helperDecisions); address != "" {
		authority, err := Listen(address, os.Getpid())
		if err != nil {
			fmt.Println("listen:", err)
			os.Exit(0)
		}
		authority.HandleDecide(answerAll)
		fmt.Println("ready")
		go authority.Serve(t.Context())
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
}

// startHelper starts a helper that listens and waits until it does.
func startHelper(t *testing.T, env string) *exec.Cmd {
	t.Helper()
	command := helper(t, nil, env)
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = command.Process.Kill(); _ = command.Wait() })
	if line, _ := bufio.NewReader(stdout).ReadString('\n'); strings.TrimSpace(line) != "ready" {
		t.Fatalf("the helper did not listen: %q", line)
	}
	return command
}

func helper(t *testing.T, wrap []string, env string) *exec.Cmd {
	t.Helper()
	args := slices.Concat(wrap, []string{os.Args[0], "-test.run=^TestHelperProcess$"})
	command := exec.Command(args[0], args[1:]...)
	command.Env = append(os.Environ(), env)
	return command
}

// A command the session started inside a sandbox of its own — a sandboxed
// harness run from its shell, say — runs below the wrapper, and still is not
// answered a decision: nothing it does is the session's.
func TestADecisionFromOtherNamespacesIsRefused(t *testing.T) {
	if err := exec.Command("unshare", "-Ur", "--pid", "--fork", "true").Run(); err != nil {
		t.Skipf("no user namespaces here: %v", err)
	}
	_, path := listenDecisions(t, answerAll)
	out, err := helper(t, []string{"unshare", "-Ur", "--pid", "--fork", "--mount", "--mount-proc"}, helperAsk+"="+path).CombinedOutput()
	if err != nil {
		t.Fatalf("helper: %v: %s", err, out)
	}
	if !strings.Contains(string(out), "sandbox of its own") {
		t.Fatalf("decided for other namespaces: %s", out)
	}
	// The same helper in this process's namespaces is answered: the refusal
	// above is the namespaces', not the helper's.
	out, err = helper(t, nil, helperAsk+"="+path).CombinedOutput()
	if err != nil || !strings.Contains(string(out), `"id":"d1"`) {
		t.Fatalf("a helper beside this process: %v: %s", err, out)
	}
}
