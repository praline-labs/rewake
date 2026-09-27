package cli

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// outsideAnySession removes both session markers for the rest of the test, as
// a shell outside any session has them: set, even to nothing, they refuse a
// launch.
func outsideAnySession(t *testing.T) {
	t.Helper()
	for _, key := range []string{state.SessionEnv, state.EpochEnv} {
		t.Setenv(key, "") // restores the value when the test ends
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
}

// A shell inside a session starts no harness: a worker whose rewake commands
// run unasked would otherwise start a session without its limits. Either
// marker is enough, set even to nothing, and neither needs a live record
// behind it.
func TestALaunchFromInsideASessionIsRefused(t *testing.T) {
	for _, h := range harness.All() {
		for _, marker := range []string{state.SessionEnv, state.EpochEnv} {
			for _, value := range []string{"worker-claude", ""} {
				t.Run(h.ID()+"/"+marker+"="+value, func(t *testing.T) {
					dir := t.TempDir()
					if err := os.Chmod(dir, 0o700); err != nil {
						t.Fatal(err)
					}
					t.Setenv(state.DirEnv, dir)
					t.Setenv(state.RoomEnv, "isolated")
					outsideAnySession(t)
					t.Setenv(marker, value)
					parsed, err := parse([]string{h.ID(), "-p", "x"})
					if err != nil {
						t.Fatal(err)
					}
					probe := &roleLaunchProbe{Harness: h}
					err = handleLaunch(probe)(&Context{Stdout: io.Discard, Stderr: io.Discard}, parsed.Call)
					var usage *UsageError
					if !errors.As(err, &usage) || !strings.Contains(usage.Message, "does not start other sessions") {
						t.Fatalf("launched from inside a session: %v", err)
					}
					if probe.request.Name != "" {
						t.Fatalf("the harness was asked to launch: %+v", probe.request)
					}
				})
			}
		}
	}
}
