package state_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/state"
)

// The lock has to hold between processes, which is the whole reason it is a
// file lock rather than a mutex. A test that only runs goroutines would pass
// with a plain sync.Mutex in its place, so this one re-runs the test binary as a
// second process and has the two compete for the same name.
const helperEnv = "REWAKE_TEST_LOCK_HELPER"

func TestMain(m *testing.M) {
	if dir := os.Getenv(helperEnv); dir != "" {
		os.Exit(holdLock(dir))
	}
	os.Exit(m.Run())
}

// holdLock takes the name lock, writes a marker while inside, and releases it.
func holdLock(dir string) int {
	code := 0
	err := state.WithNameLock(dir, "api", func() error {
		marker := filepath.Join(dir, "inside")
		if _, err := os.Stat(marker); err == nil {
			// Somebody else is inside the lock at the same time.
			code = 3
			return nil
		}
		if err := os.WriteFile(marker, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
			return err
		}
		time.Sleep(700 * time.Millisecond)
		return os.Remove(marker)
	})
	if err != nil {
		return 4
	}
	return code
}

func TestNameLockHoldsBetweenProcesses(t *testing.T) {
	base := filepath.Join(t.TempDir(), "state")
	t.Setenv(state.DirEnv, base)
	dir, err := state.Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("executable: %v", err)
	}

	first := exec.Command(executable)
	first.Env = append(os.Environ(), helperEnv+"="+dir)
	if err := first.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	// Let it get inside the lock before the second one tries.
	time.Sleep(200 * time.Millisecond)

	second := exec.Command(executable)
	second.Env = append(os.Environ(), helperEnv+"="+dir)
	if err := second.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	for name, command := range map[string]*exec.Cmd{"first": first, "second": second} {
		if err := command.Wait(); err != nil {
			t.Fatalf("%s process: %v", name, err)
		}
		if code := command.ProcessState.ExitCode(); code == 3 {
			t.Fatalf("%s process was inside the lock while the other held it", name)
		} else if code != 0 {
			t.Fatalf("%s process failed with code %d", name, code)
		}
	}
}
