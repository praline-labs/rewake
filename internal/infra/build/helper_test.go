package build

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// holderEnv makes the test binary a holder of a room's lease instead of a
// test run: another process, as a lease is only ever contended between
// processes. Its value is "writer <base> <room>", which takes the lease as
// any writer does, or "shared <lock path>", which holds the file shared with
// no build checked — a writer of another build — or "exclusive <lock path>",
// a launch that stalled while it switched the build. Either prints "held" once it
// holds, or "refused <why>", and holds until its stdin closes. "id" prints
// the process's build id once a line arrives on its stdin.
const holderEnv = "REWAKE_BUILD_TEST_HOLDER"

func TestMain(m *testing.M) {
	if spec := os.Getenv(holderEnv); spec != "" {
		os.Exit(runHolder(strings.Fields(spec)))
	}
	os.Exit(m.Run())
}

func runHolder(spec []string) int {
	switch spec[0] {
	case "id":
		// Hash only once told to: the test replaces the file first.
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
		own, err := ID()
		if err != nil {
			fmt.Println("refused", err)
			return 3
		}
		fmt.Println(own)
		return 0
	case "writer":
		shareWait = time.Second
		lease, err := Acquire(spec[1], spec[2])
		if err != nil {
			fmt.Println("refused", strings.ReplaceAll(err.Error(), "\n", " "))
			return 3
		}
		defer lease.Release()
	case "shared", "exclusive":
		how := syscall.LOCK_SH
		if spec[0] == "exclusive" {
			how = syscall.LOCK_EX
		}
		file, err := os.OpenFile(spec[1], os.O_RDWR|os.O_CREATE, 0o600)
		if err != nil {
			fmt.Println("refused", err)
			return 3
		}
		if err := syscall.Flock(int(file.Fd()), how); err != nil {
			fmt.Println("refused", err)
			return 3
		}
	}
	fmt.Println("held")
	_, _ = io.Copy(io.Discard, os.Stdin)
	return 0
}

// holder is a holder process the test started.
type holder struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	// answer is the first line it printed.
	answer string
	once   sync.Once
}

// startHolder starts a holder and waits for its first line.
func startHolder(t *testing.T, spec ...string) *holder {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), holderEnv+"="+strings.Join(spec, " "))
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	h := &holder{cmd: cmd, stdin: stdin}
	t.Cleanup(h.release)
	lines := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(stdout).ReadString('\n')
		lines <- strings.TrimSpace(line)
	}()
	select {
	case h.answer = <-lines:
	case <-time.After(20 * time.Second):
		t.Fatalf("holder %v said nothing", spec)
	}
	return h
}

// release lets the holder go and waits for it to exit.
func (h *holder) release() {
	h.once.Do(func() {
		_ = h.stdin.Close()
		_ = h.cmd.Wait()
	})
}

func (h *holder) held() bool { return h.answer == "held" }

// actAs makes this process another build for the rest of the test: its id
// is that of a file whose content differs from the test binary's.
func actAs(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "image")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	setImage(t, path)
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// setImage points the id at another file, read again on first use.
func setImage(t *testing.T, path string) {
	t.Helper()
	before, beforeID := image, id
	image = path
	id = sync.OnceValues(func() (string, error) { return hashFile(image) })
	t.Cleanup(func() { image, id = before, beforeID })
}

// newBase is an empty base directory: the path, not yet made.
func newBase(t *testing.T) string {
	t.Helper()
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(parent, "base")
}

// roomOf is a room's directory on the layout Bootstrap makes.
func roomOf(base, room string) string { return filepath.Join(base, "v2", "rooms", room) }

// record reads a room's build record, its raw bytes or "missing".
func record(t *testing.T, room string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(room, "build"))
	if os.IsNotExist(err) {
		return "missing"
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// writeRecord sets a room's build record as a fixture: raw bytes, or nothing
// for "missing".
func writeRecord(t *testing.T, room, content string) {
	t.Helper()
	path := filepath.Join(room, "build")
	if content == "missing" {
		_ = os.Remove(path)
		return
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// established makes a room whose build is this process's, as a launch does.
func established(t *testing.T, base, room string) string {
	t.Helper()
	lease, err := AcquireLaunch(base, room)
	if err != nil {
		t.Fatalf("launch into an empty base: %v", err)
	}
	lease.Release()
	return roomOf(base, room)
}

// leaseLockPath is the room's build.lock.
func leaseLockPath(room string) string { return filepath.Join(room, "build.lock") }
