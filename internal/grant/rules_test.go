package grant

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// machine is a small home with every kind of protected directory in it, and
// the rules built from it. Only the system directories come from the real
// filesystem, which every rule table has.
type machine struct {
	root, home string
	rules      Rules
}

func newMachine(t *testing.T) machine {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	for _, dir := range []string{
		"home/.ssh/keys", "home/.codex/sessions", "home/.config/git", "home/.config/svc",
		"home/.config/plain", "home/code/proj/sub", "home/code/other", "state/rooms",
		"bin", "pathbin", "elsewhere",
	} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write(t, filepath.Join(root, "home/.config/svc/credentials"))
	write(t, filepath.Join(root, "home/code/notes.txt"))
	link(t, filepath.Join(home, "code/proj"), filepath.Join(root, "proj-link"))
	link(t, filepath.Join(home, ".ssh"), filepath.Join(root, "keys-link"))
	env := Env{
		Home:       home,
		PATH:       filepath.Join(root, "pathbin") + string(os.PathListSeparator) + "relative/bin",
		StateRoot:  filepath.Join(root, "state"),
		Executable: filepath.Join(root, "bin", "rewake"),
		Harness:    []string{filepath.Join(home, ".codex"), filepath.Join(home, ".claude")},
		Mounts:     []string{"/mnt/c", "/media/usb", "/boot/efi", "/srv/data"},
	}
	return machine{root: root, home: home, rules: env.Rules()}
}

func write(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func link(t *testing.T, target, path string) {
	t.Helper()
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
}

// grantOf runs what send runs: resolve from cwd, then check.
func (m machine) grantOf(given string, broad bool) (string, error) {
	path, err := Resolve(given, m.root)
	if err != nil {
		return "", err
	}
	return path, m.rules.Check(given, path, broad)
}

func TestDirectoriesAreResolvedAndChecked(t *testing.T) {
	m := newMachine(t)
	cases := []struct {
		name, given string
		broad       bool
		code        int
		// resolved is where an accepted grant lands, relative to the root.
		resolved string
		// says is part of a refusal's text.
		says string
	}{
		{name: "relative from cwd", given: "home/code/proj", resolved: "home/code/proj"},
		{name: "dot elements cleaned", given: "./home/code/other/../proj/", resolved: "home/code/proj"},
		{name: "symlink resolved", given: "proj-link/sub", resolved: "home/code/proj/sub"},
		{name: "plain config dir", given: "home/.config/plain", resolved: "home/.config/plain"},
		{name: "missing", given: "home/code/nope", code: 1, says: "no such directory"},
		{name: "file", given: "home/code/notes.txt", code: 1, says: "is a file"},
		{name: "keys", given: "home/.ssh", code: 2, says: "is the directory, which is where login keys"},
		{name: "keys through a link", given: "keys-link", code: 2, says: "(" + filepath.Join(m.home, ".ssh") + ")"},
		{name: "inside keys", given: "home/.ssh/keys", code: 2, says: "lies inside"},
		{name: "contains keys", given: ".", code: 2, says: "contains"},
		{name: "home contains keys", given: "home", code: 2, says: "contains"},
		{name: "harness config", given: "home/.codex/sessions", code: 2, says: "a harness's own configuration"},
		{name: "git config", given: "home/.config/git", code: 2, says: "runs or signs as the owner"},
		{name: "state", given: "state/rooms", code: 2, says: "rewake's state directory"},
		{name: "binary", given: "bin", code: 2, says: "the rewake binary"},
		{name: "path", given: "pathbin", code: 2, says: "a directory on PATH"},
		{name: "root", given: "/", code: 2, says: "/ contains"},
		{name: "system", given: "/etc", code: 2, says: "a system directory"},
		{name: "inside system", given: "/usr/share", code: 2, says: "lies inside /usr"},
		{name: "home child unconfirmed", given: "home/code", code: 2, says: "--grant-dir-broad home/code"},
		{name: "home child confirmed", given: "home/code", broad: true, resolved: "home/code"},
		{name: "credentials unconfirmed", given: "home/.config/svc", code: 2, says: "credentials file"},
		{name: "credentials confirmed", given: "home/.config/svc", broad: true, resolved: "home/.config/svc"},
		{name: "confirmation of a narrow one", given: "home/code/proj", broad: true, code: 2, says: "pass it with --grant-dir"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path, err := m.grantOf(c.given, c.broad)
			if c.code == 0 {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				if want := filepath.Join(m.root, c.resolved); path != want {
					t.Fatalf("resolved to %s, want %s", path, want)
				}
				return
			}
			var refusal *Refusal
			if !errors.As(err, &refusal) {
				t.Fatalf("got %v, want a refusal", err)
			}
			if refusal.Code != c.code || !strings.Contains(refusal.Message, c.says) {
				t.Fatalf("got code %d %q, want code %d saying %q", refusal.Code, refusal.Message, c.code, c.says)
			}
		})
	}
}

// Drives and mounts are compared by resolved string, as they are on a machine
// that has them: whether /mnt/c exists here does not matter to the rule.
func TestDrivesAndMountsAreBroadInAnyCase(t *testing.T) {
	m := newMachine(t)
	cases := []struct {
		path  string
		broad bool
		code  int
		says  string
	}{
		{path: "/mnt", code: 2, says: "every home or drive"},
		{path: "/mnt", broad: true},
		{path: "/mnt/c", code: 2, says: "a mounted drive"},
		{path: "/mnt/C", code: 2, says: "a mounted drive"},
		{path: "/mnt/C", broad: true},
		{path: "/mnt/C/Work", broad: true, code: 2, says: "pass it with --grant-dir"},
		{path: "/mnt/c/work"},
		{path: "/media/usb", code: 2, says: "a mounted drive"},
		{path: "/mnt/cd"},
		{path: "/boot/efi", broad: true, code: 2, says: "a system directory"},
	}
	for _, c := range cases {
		err := m.rules.Check(c.path, c.path, c.broad)
		var refusal *Refusal
		switch {
		case c.code == 0 && err != nil:
			t.Errorf("%s broad=%v: refused: %v", c.path, c.broad, err)
		case c.code != 0 && (!errors.As(err, &refusal) || refusal.Code != c.code || !strings.Contains(refusal.Message, c.says)):
			t.Errorf("%s broad=%v: got %v, want code %d saying %q", c.path, c.broad, err, c.code, c.says)
		}
	}
}

func TestDriveCaseDoesNotHideAProtectedDirectory(t *testing.T) {
	rules := Env{Home: "/mnt/c/Users/me"}.Rules()
	for _, path := range []string{"/mnt/C/users/ME/.SSH", "/mnt/c/USERS/me/.ssh/id", "/mnt/C/Users"} {
		if err := rules.Check(path, path, false); err == nil {
			t.Errorf("%s was accepted beside /mnt/c/Users/me/.ssh", path)
		}
	}
	// Outside a drive letter the case is the filesystem's own.
	rules = Env{Home: "/data/me"}.Rules()
	if err := rules.Check("/data/ME/.ssh", "/data/ME/.ssh", false); err != nil {
		t.Errorf("a different directory on a case-sensitive filesystem was refused: %v", err)
	}
}

func TestPathElementsNotPrefixes(t *testing.T) {
	for _, c := range []struct {
		path, dir string
		within    bool
	}{
		{"/a/b", "/a/b", true},
		{"/a/b/c", "/a/b", true},
		{"/a/bc", "/a/b", false},
		{"/a/b", "/a/bc", false},
		{"/a/..b", "/a", true},
		{"/a", "/a/b", false},
		{"/mnt/C/x", "/mnt/c", true},
	} {
		if got := Within(c.path, c.dir); got != c.within {
			t.Errorf("Within(%s, %s) = %v", c.path, c.dir, got)
		}
	}
}

func TestNestedGrantsCollapseToTheOutermost(t *testing.T) {
	got := Outermost([]string{"/w/a/b", "/w/a", "/w/c", "/w/a", "/w/ab", "/mnt/C/x", "/mnt/c"})
	want := []string{"/w/a", "/w/c", "/w/ab", "/mnt/c"}
	if !slices.Equal(got, want) {
		t.Fatalf("Outermost = %v, want %v", got, want)
	}
}

func TestRecheckCatchesAChangeSinceSending(t *testing.T) {
	m := newMachine(t)
	work := filepath.Join(m.root, "work")
	if err := os.MkdirAll(filepath.Join(work, "proj"), 0o755); err != nil {
		t.Fatal(err)
	}
	path, err := m.grantOf("work/proj", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.rules.Recheck(path, false); err != nil {
		t.Fatalf("an unchanged directory failed: %v", err)
	}
	// A directory on the way swapped for a link that leads elsewhere.
	if err := os.Rename(work, work+".old"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(m.home, ".ssh", "proj"), 0o755); err != nil {
		t.Fatal(err)
	}
	link(t, filepath.Join(m.home, ".ssh"), work)
	err = m.rules.Recheck(path, false)
	var refusal *Refusal
	if !errors.As(err, &refusal) || refusal.Code != 1 || !strings.Contains(refusal.Message, "a link changed") {
		t.Fatalf("got %v, want a changed link", err)
	}
	if err := os.Remove(work); err != nil {
		t.Fatal(err)
	}
	if err := m.rules.Recheck(path, false); err == nil || !strings.Contains(err.Error(), "is gone") {
		t.Fatalf("got %v, want gone", err)
	}
	if err := m.rules.Recheck("work/proj", false); err == nil {
		t.Fatal("a relative path passed a recheck")
	}
	if err := m.rules.Recheck(filepath.Join(m.root, "bin"), false); err == nil {
		t.Fatal("a protected directory passed a recheck")
	}
}

func TestProtectedDirectoryNotThereYetProtectsThroughLinks(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "real")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	link(t, target, filepath.Join(root, "home"))
	rules := Env{Home: filepath.Join(root, "home")}.Rules()
	// ~/.aws does not exist; its place is under the resolved home.
	aws := filepath.Join(target, ".aws")
	if err := rules.Check(aws, aws, false); err == nil {
		t.Fatal("the place of a missing ~/.aws was accepted")
	}
}

func TestMountPointsAreUnescaped(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mountinfo")
	content := "22 1 8:1 / / rw - ext4 /dev/sda1 rw\n" +
		"90 22 0:50 / /mnt/my\\040drive rw - 9p drvfs rw\n" +
		"91 22 0:51 / /media/a\\134b rw - vfat /dev/sdb1 rw\n" +
		"short line\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	got := mountPoints(path)
	want := []string{"/", "/mnt/my drive", `/media/a\b`}
	if !slices.Equal(got, want) {
		t.Fatalf("mountPoints = %q, want %q", got, want)
	}
	if mountPoints(filepath.Join(t.TempDir(), "none")) != nil {
		t.Fatal("an unreadable mountinfo gave mounts")
	}
}
