package legacy_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/praline-labs/rewake/internal/infra/legacy"
	"github.com/praline-labs/rewake/internal/infra/proc"
)

// leftID is the id of a candidate a clearer that died left behind.
const leftID = "1.2.0123456789abcdef"

// Admission refuses a live wrapper, a 1.x lock held and every check it cannot
// make, in rooms/ and in a candidate alike, naming what it found; and a
// refused launch removes nothing.
func TestLegacyAdmissionRejectsLiveBusyAndUnknown(t *testing.T) {
	cases := []struct {
		name  string
		setup func(s *scene, room string)
		want  []string
	}{
		{
			"a live wrapper", func(s *scene, room string) { s.liveSession(room, "web") },
			[]string{"a rewake 1.x session runs: web in room old", "does not read the mail of rewake 1.x, so end those sessions"},
		},
		{
			"a launch lock held", func(s *scene, room string) { holdLock(s.t, filepath.Join(room, ".launch.lock")) },
			[]string{"a rewake 1.x command runs in room old", ".launch.lock"},
		},
		{
			"a name lock held", func(s *scene, room string) { holdLock(s.t, filepath.Join(room, "sessions", ".web.lock")) },
			[]string{"a rewake 1.x command runs in room old", "sessions/.web.lock"},
		},
		{
			"a mailbox lock held", func(s *scene, room string) { holdLock(s.t, filepath.Join(room, "inbox", "web", ".lock")) },
			[]string{"a rewake 1.x command runs in room old", "inbox/web/.lock"},
		},
		{
			"a record that does not parse", func(s *scene, room string) { mustWrite(s.t, filepath.Join(room, "sessions", "web.json"), "{") },
			[]string{"web.json cannot be checked", "you may remove it once no rewake 1.x session runs"},
		},
		{
			"a record with no start", func(s *scene, room string) { s.session(room, "web", os.Getpid(), 0, proc.Namespace()) },
			[]string{"web.json cannot be checked", "names no wrapper"},
		},
		{
			"a record of another namespace", func(s *scene, room string) { s.session(room, "web", 1, 1, "pid:[1]") },
			[]string{"web.json cannot be checked", "pid namespace pid:[1]"},
		},
		{"a directory that cannot be listed", func(s *scene, room string) {
			if os.Geteuid() == 0 {
				s.t.Skip("root lists any directory")
			}
			sessions := filepath.Join(room, "sessions")
			if err := os.Chmod(sessions, 0); err != nil {
				s.t.Fatal(err)
			}
			s.t.Cleanup(func() { _ = os.Chmod(sessions, 0o700) })
		}, []string{"sessions cannot be checked"}},
		{"a room that is a link", func(s *scene, room string) {
			_ = os.RemoveAll(room)
			if err := os.Symlink(s.t.TempDir(), room); err != nil {
				s.t.Fatal(err)
			}
		}, []string{"old cannot be checked"}},
	}
	for _, tree := range []string{"rooms", "candidate"} {
		for _, c := range cases {
			t.Run(tree+"/"+c.name, func(t *testing.T) {
				s := newScene(t)
				path := s.rooms()
				if tree == "candidate" {
					path = s.candidate(leftID)
				}
				room := s.room(path, "old")
				s.letter(room, "web", "m1")
				c.setup(s, room)
				before := s.unrelated()
				s.refused(c.want...)
				s.wantTrees(tree)
				if !exists(filepath.Join(path, "old", "inbox", "web", "m1.json")) && c.name != "a room that is a link" {
					t.Fatal("a refused launch lost a letter of 1.x")
				}
				if s.unrelated() != before {
					t.Fatal("a refused launch changed what lies beside the trees")
				}
			})
		}
	}
}

// Whether a wrapper runs is read from /proc and nothing else: no /proc/<pid>
// is proof it is gone, another start time is another process and a zombie
// does not run — the tree is cleared; a stat that cannot be read or parsed,
// or a namespace this launch cannot read, is unknown — the launch refuses. A
// running wrapper refuses as itself.
func TestLegacyMissingProcessIsDeadButReadFailureUnknown(t *testing.T) {
	const pid, start, ns = 4242, 777, "pid:[4026531836]"
	stat := func(state string, start int) string {
		return strconv.Itoa(pid) + " (rewake (1.x)) " + state + " 1 1 1 0 -1 0 0 0 0 0 0 0 0 0 20 0 1 0 " + strconv.Itoa(start) + " 0 0\n"
	}
	cases := []struct {
		name    string
		proc    func(root string)
		cleared bool
		want    string
	}{
		{"no /proc/<pid>", func(string) {}, true, ""},
		{"another start time", func(root string) { mustWrite(t, filepath.Join(root, "4242", "stat"), stat("S", start+1)) }, true, ""},
		{"a zombie", func(root string) { mustWrite(t, filepath.Join(root, "4242", "stat"), stat("Z", start)) }, true, ""},
		{"running", func(root string) { mustWrite(t, filepath.Join(root, "4242", "stat"), stat("S", start)) }, false, "a rewake 1.x session runs: web in room old"},
		{"a stat that does not parse", func(root string) { mustWrite(t, filepath.Join(root, "4242", "stat"), "4242 (x) S") }, false, "/proc/4242/stat cannot be read"},
		{"a stat that cannot be read", func(root string) {
			if os.Geteuid() == 0 {
				t.Skip("root reads any file")
			}
			path := filepath.Join(root, "4242", "stat")
			mustWrite(t, path, stat("S", start))
			if err := os.Chmod(path, 0); err != nil {
				t.Fatal(err)
			}
		}, false, "/proc/4242/stat cannot be read"},
		{"no namespace of its own", func(root string) { _ = os.Remove(filepath.Join(root, "self", "ns", "pid")) }, false, "cannot read its own pid namespace"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, "self", "ns"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(ns, filepath.Join(root, "self", "ns", "pid")); err != nil {
				t.Fatal(err)
			}
			c.proc(root)
			s := newScene(t)
			s.session(s.room(s.rooms(), "old"), "web", pid, start, ns)
			legacy.SetProc(t, root)
			if c.cleared {
				s.went()
				s.wantTrees()
				return
			}
			s.refused(c.want)
			s.wantTrees("rooms")
		})
	}
	t.Run("a process that exited", func(t *testing.T) {
		child := exec.Command("/bin/true")
		if err := child.Start(); err != nil {
			t.Fatal(err)
		}
		pid := child.Process.Pid
		started, err := proc.StartTime(pid)
		_ = child.Wait()
		if err != nil {
			t.Skip("the child ended before its start time was read")
		}
		s := newScene(t)
		s.session(s.room(s.rooms(), "old"), "web", pid, started, proc.Namespace())
		s.went()
		s.wantTrees()
	})
}
