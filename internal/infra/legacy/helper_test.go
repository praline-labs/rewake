package legacy_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/infra/build"
	"github.com/praline-labs/rewake/internal/infra/legacy"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// scene is a base with the state of 1.x beside it, launched into the way a
// launch does: build.Launch, its lease and the clearing under it.
type scene struct {
	t    *testing.T
	base string
	said bytes.Buffer
}

func newScene(t *testing.T) *scene {
	t.Helper()
	base := t.TempDir()
	if err := os.Chmod(base, 0o700); err != nil {
		t.Fatal(err)
	}
	legacy.SetWaits(t, 200*time.Millisecond, 50*time.Millisecond)
	// Unrelated files beside the trees, which nothing may touch.
	for _, path := range []string{"notes/kept.txt", "worktrees/x/file"} {
		mustWrite(t, filepath.Join(base, path), "kept")
	}
	return &scene{t: t, base: base}
}

func (s *scene) rooms() string              { return filepath.Join(s.base, "rooms") }
func (s *scene) candidate(id string) string { return filepath.Join(s.base, ".rooms-1x-candidate-"+id) }

func (s *scene) validated(id string) string { return filepath.Join(s.base, ".rooms-1x-validated-"+id) }

func (s *scene) summary(id string) string {
	return filepath.Join(s.base, "v2", ".cleared-1x-"+id+".json")
}

// room makes a 1.x room in a tree, as a 1.x launch does by MkdirAll.
func (s *scene) room(tree, name string) string {
	s.t.Helper()
	dir := filepath.Join(tree, name)
	for _, sub := range []string{"sessions", "inbox", "sock"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o700); err != nil {
			s.t.Fatal(err)
		}
	}
	return dir
}

// session publishes a 1.x session record naming a wrapper.
func (s *scene) session(room, name string, pid int, start uint64, namespace string) string {
	s.t.Helper()
	path := filepath.Join(room, "sessions", name+".json")
	data, _ := json.Marshal(map[string]any{"name": name, "servicePid": pid, "serviceStart": start, "pidNamespace": namespace})
	mustWrite(s.t, path, string(data))
	return path
}

// liveSession publishes a record naming this very test process.
func (s *scene) liveSession(room, name string) string {
	s.t.Helper()
	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		s.t.Fatal(err)
	}
	return s.session(room, name, os.Getpid(), start, proc.Namespace())
}

// letter writes a waiting letter into a mailbox of a room.
func (s *scene) letter(room, to, id string) {
	s.t.Helper()
	mustWrite(s.t, filepath.Join(room, "inbox", to, id+".json"), `{"id":"`+id+`","to":"`+to+`","text":"hi"}`)
}

// launch runs a launch's acquisition and clearing, releasing the lease after.
func (s *scene) launch() error {
	s.t.Helper()
	lease, err := build.Launch(s.base, state.DefaultRoom, &s.said)
	if err == nil {
		lease.Release()
	}
	return err
}

// refused requires the launch to be refused by the state of 1.x, its
// message holding each of want.
func (s *scene) refused(want ...string) {
	s.t.Helper()
	err := s.launch()
	var refusal *legacy.Refusal
	if !errors.As(err, &refusal) {
		s.t.Fatalf("the launch was not refused by the state of 1.x: %v", err)
	}
	for _, text := range want {
		if !strings.Contains(err.Error(), text) {
			s.t.Fatalf("the refusal does not say %q:\n%v", text, err)
		}
	}
}

// went requires the launch to go on.
func (s *scene) went() {
	s.t.Helper()
	if err := s.launch(); err != nil {
		s.t.Fatalf("the launch was refused: %v", err)
	}
}

// trees names the trees of 1.x in the base, as "rooms", "candidate" and
// "validated" in order.
func (s *scene) trees() []string {
	s.t.Helper()
	entries, err := os.ReadDir(s.base)
	if err != nil {
		s.t.Fatal(err)
	}
	var kinds []string
	for _, entry := range entries {
		switch name := entry.Name(); {
		case name == "rooms":
			kinds = append(kinds, "rooms")
		case strings.HasPrefix(name, ".rooms-1x-candidate-"):
			kinds = append(kinds, "candidate")
		case strings.HasPrefix(name, ".rooms-1x-validated-"):
			kinds = append(kinds, "validated")
		}
	}
	sort.Strings(kinds)
	return kinds
}

func (s *scene) wantTrees(want ...string) {
	s.t.Helper()
	sort.Strings(want)
	if got := s.trees(); strings.Join(got, ",") != strings.Join(want, ",") {
		s.t.Fatalf("trees %v, want %v", got, want)
	}
}

// only returns the one tree of a kind's path.
func (s *scene) only(kind string) string {
	s.t.Helper()
	matches, _ := filepath.Glob(filepath.Join(s.base, ".rooms-1x-"+kind+"-*"))
	if len(matches) != 1 {
		s.t.Fatalf("%d %s trees: %v", len(matches), kind, matches)
	}
	return matches[0]
}

// unrelated hashes what lies in the base outside the trees of 1.x and v2/:
// what a clearing must leave as it found it.
func (s *scene) unrelated() string {
	s.t.Helper()
	sum := sha256.New()
	err := filepath.WalkDir(s.base, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(s.base, path)
		first, _, _ := strings.Cut(rel, string(filepath.Separator))
		if first == "rooms" || first == "v2" || strings.HasPrefix(first, ".rooms-1x-") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		data := []byte{}
		if info.Mode().IsRegular() {
			if data, err = os.ReadFile(path); err != nil {
				return err
			}
		}
		_, _ = fmt.Fprintf(sum, "%s %v %x\n", rel, info.Mode(), sha256.Sum256(data))
		return nil
	})
	if err != nil {
		s.t.Fatal(err)
	}
	return hex.EncodeToString(sum.Sum(nil))
}

// onStep runs act when the clearing reaches the named step, for one test.
func onStep(t *testing.T, step string, act func()) {
	t.Helper()
	faults(t, func(op, path string) error {
		if op == state.OpStep && path == step {
			act()
		}
		return nil
	})
}

// faults sets the fault seam for one test.
func faults(t *testing.T, plan func(op, path string) error) {
	t.Helper()
	state.Fault = plan
	t.Cleanup(func() { state.Fault = nil })
}

// holdLock takes a flock on path, creating it, until the test ends: another
// command of 1.x holding it.
func holdLock(t *testing.T, path string) func() {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	release := func() { _ = file.Close() }
	t.Cleanup(release)
	return release
}

func mustWrite(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
