package grantauth

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"
)

// A test process started again as a helper: another process for the
// protocol to face, where the check under test is about who the peer is.
const (
	helperRegister = "REWAKE_GRANTAUTH_REGISTER"
	helperListen   = "REWAKE_GRANTAUTH_LISTEN"
)

// TestHelperProcess is not a test: it is what a helper runs.
func TestHelperProcess(t *testing.T) {
	if address := os.Getenv(helperRegister); address != "" {
		// The raw request, not Register: a forger skips the client's own
		// checks, so only the wrapper's side is under test.
		conn, err := net.Dial("unix", address)
		if err != nil {
			fmt.Println("dial:", err)
			os.Exit(0)
		}
		_ = json.NewEncoder(conn).Encode(request{Op: opRegister, Grant: lib})
		line, _ := bufio.NewReader(io.LimitReader(conn, maxLine)).ReadBytes('\n')
		fmt.Print(string(line))
		os.Exit(0)
	}
	if address := os.Getenv(helperListen); address != "" {
		authority, err := Listen(address, os.Getpid(), time.Minute)
		if err != nil {
			fmt.Println("listen:", err)
			os.Exit(0)
		}
		fmt.Println("ready")
		go authority.Serve(t.Context())
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
}

func helper(t *testing.T, wrap []string, env string) *exec.Cmd {
	t.Helper()
	args := slices.Concat(wrap, []string{os.Args[0], "-test.run=^TestHelperProcess$"})
	command := exec.Command(args[0], args[1:]...)
	command.Env = append(os.Environ(), env)
	return command
}

// A command main started inside a sandbox of its own — Codex run from
// main's shell, say — runs below main's wrapper, and still does not register
// a grant: nothing it does is main's.
func TestARegistrationFromOtherNamespacesIsRefused(t *testing.T) {
	if err := exec.Command("unshare", "-Ur", "--pid", "--fork", "true").Run(); err != nil {
		t.Skipf("no user namespaces here: %v", err)
	}
	_, path := listen(t, os.Getpid(), time.Minute)
	out, err := helper(t, []string{"unshare", "-Ur", "--pid", "--fork", "--mount", "--mount-proc"}, helperRegister+"="+path).CombinedOutput()
	if err != nil {
		t.Fatalf("helper: %v: %s", err, out)
	}
	if !strings.Contains(string(out), "sandbox of its own") {
		t.Fatalf("registered from other namespaces: %s", out)
	}
	// The same helper in this process's namespaces is taken: the refusal
	// above is the namespaces', not the helper's.
	out, err = helper(t, nil, helperRegister+"="+path).CombinedOutput()
	if err != nil || !strings.HasPrefix(strings.TrimSpace(string(out)), "{}") {
		t.Fatalf("a helper beside this process: %v: %s", err, out)
	}
}

// A listener that is not above the caller — one that bound the address
// before main's wrapper did — does not take its registration: send would
// report a grant nobody can confirm.
func TestARegistrationGoesOnlyToTheCallersWrapper(t *testing.T) {
	path := "@rewake-test/" + t.Name() + "/" + fmt.Sprint(os.Getpid())
	command := helper(t, nil, helperListen+"="+path)
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
	err = Register(path, lib)
	if !errors.Is(err, ErrNotConfirmed) || !strings.Contains(err.Error(), "not this session's wrapper") {
		t.Fatalf("registered with a listener beside the caller: %v", err)
	}
}
