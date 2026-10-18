package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/infra/build"
	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// --version --json carries the build id, the SHA-256 of the executable; the
// human line stays as it was.
func TestVersionJSONCarriesBuildID(t *testing.T) {
	id, err := build.ID()
	if err != nil {
		t.Fatal(err)
	}
	code, out, _ := run("--version", "--json")
	var got Build
	if err := json.Unmarshal([]byte(out), &got); code != ExitOK || err != nil || got.ID != id {
		t.Fatalf("exit %d, %v: build %q, want %q, from %s", code, err, got.ID, id, out)
	}
	code, out, _ = run("--version")
	if code != ExitOK || strings.Contains(out, build.Short(id)) || out != thisBuild().Line()+"\n" {
		t.Fatalf("the line: %d %q", code, out)
	}
}

// A change form on a base no launch has made refuses — no build was ever
// established there — and makes nothing: no base, no room.
func TestChangeFormsOnAnEmptyBaseCreateNothing(t *testing.T) {
	base := filepath.Join(t.TempDir(), "base")
	t.Setenv(state.DirEnv, base)
	t.Setenv(state.RoomEnv, "")
	t.Setenv(state.SessionEnv, "api")
	t.Setenv(epochEnv, "1.1")
	for _, argv := range [][]string{
		{"send", "web", "hello", "--wait", "0"},
		{"inbox"},
		{"inbox", "--peek"},
		{"pending", "the review"},
		{"retry", "0123456789abcdef01234567"},
		{"withdraw", "1789590000000000000-aabbccddeeff"},
		{"worktree", "rm", "nothing"},
	} {
		code, _, errOut := run(argv...)
		if code != ExitFailed || !strings.Contains(errOut, "has no build yet") {
			t.Errorf("%v: exit %d, %q", argv, code, errOut)
		}
		if _, err := os.Lstat(base); !os.IsNotExist(err) {
			t.Fatalf("%v made the base: %v", argv, err)
		}
	}
}

// A change takes the room's lease before anything else: in a room of another
// build it is refused, and no message is written.
func TestAChangeRefusesAnotherBuildsRoom(t *testing.T) {
	dir, _, web := peerSession(t)
	foreign := strings.Repeat("ab", 32)
	if err := os.WriteFile(filepath.Join(dir, "build"), []byte(foreign+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := run("send", "web", "hello", "--wait", "0")
	if code != ExitFailed || !strings.Contains(errOut, "belongs to build "+build.Short(foreign)) {
		t.Fatalf("the shell: exit %d, %q", code, errOut)
	}
	if files, _ := filepath.Glob(filepath.Join(state.InboxPath(dir, web.Name), "*.json")); len(files) != 0 {
		t.Fatalf("a message was written: %v", files)
	}
}

// A change whose lease could not be taken is refused before its scope and
// tries no write in the room it could not acquire, though the receipts are
// writable — the lock that refused it is unreadable, in a room of this build
// or another's.
func TestAFailedLeaseWritesNothing(t *testing.T) {
	for _, foreign := range []bool{false, true} {
		// Held, the test's own lease would let a stray write land; released,
		// enforcement refuses it, and the count of refusals shows the attempt.
		for _, held := range []bool{true, false} {
			dir := liveSession(t, "api")
			epoch := epochOf(t, dir, "api")
			t.Setenv(state.SessionEnv, "api")
			t.Setenv(state.EpochEnv, epoch)
			if foreign {
				if err := os.WriteFile(filepath.Join(dir, "build"), []byte(strings.Repeat("ab", 32)+"\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if !held {
				buildtest.Release(dir)
			}
			lock := filepath.Join(dir, "build.lock")
			if err := os.Chmod(lock, 0); err != nil {
				t.Fatal(err)
			}
			before := state.NoLeaseRefusals()
			code, _, errOut := run("pending", "the review")
			_ = os.Chmod(lock, 0o600)
			if code != ExitFailed || !strings.Contains(errOut, "could not open the build lease") {
				t.Fatalf("foreign %v, held %v: the lease was not refused: %d %s", foreign, held, code, errOut)
			}
			if refused := state.NoLeaseRefusals() - before; refused != 0 {
				t.Fatalf("foreign %v, held %v: the refused call tried %d writes", foreign, held, refused)
			}
		}
	}
}
