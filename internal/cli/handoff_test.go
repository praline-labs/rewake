package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/infra/build"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// A command of a room that runs another build, and keeps a copy of it, is
// handed to that copy with its arguments and one line on stderr — a change and
// a view alike; the guide, help and the version stay this build's. An exec
// that fails leaves the call to this build: refused, or a view of another
// build's state.
func TestACommandOfAnOlderRoomIsHandedToItsCopy(t *testing.T) {
	dir := liveSession(t, "api")
	t.Setenv(state.SessionEnv, "api")
	copied := []byte("the room's build")
	sum := sha256.Sum256(copied)
	theirs := hex.EncodeToString(sum[:])
	path := build.Pinned(dir)
	_ = os.Remove(path)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, copied, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "build"), []byte(theirs+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var execs [][]string
	before := execImage
	execImage = func(program string, argv, _ []string) error {
		execs = append(execs, append([]string{program}, argv...))
		return errors.New("not in a test")
	}
	t.Cleanup(func() { execImage = before })

	for _, argv := range [][]string{{"send", "api", "hello"}, {"list", "--json"}} {
		execs = nil
		code, _, errOut := run(argv...)
		want := append([]string{path, path}, argv...)
		if len(execs) != 1 || !slices.Equal(execs[0], want) {
			t.Errorf("%q: exec %q, want %q", argv, execs, want)
		}
		if !strings.Contains(errOut, "runs build "+build.Short(theirs)) || !strings.Contains(errOut, "could not be started") {
			t.Errorf("%q: stderr %q", argv, errOut)
		}
		if argv[0] == "send" && code != ExitFailed {
			t.Errorf("a change whose hand-off failed: exit %d", code)
		}
	}
	for _, argv := range [][]string{{"--version"}, {"guide"}, {"send", "--help"}} {
		execs = nil
		if _, _, errOut := run(argv...); len(execs) != 0 || strings.Contains(errOut, "own copy") {
			t.Errorf("%q handed off: %q", argv, errOut)
		}
	}
}
