package proc

import (
	"os"
	"path/filepath"
	"testing"
)

// A group is alive while any member is not a zombie, its leader gone or
// not; a tree that cannot be listed holds one.
func TestAGroupLivesWhileAnyMemberIsNotAZombie(t *testing.T) {
	stat := func(pid, group, state string) string {
		return pid + " (sh (x)) " + state + " 1 " + group + " " + group + " 0\n"
	}
	cases := []struct {
		name    string
		members map[string]string
		alive   bool
	}{
		{"none", map[string]string{"7": stat("7", "8", "S")}, false},
		{"a child outlived its leader", map[string]string{"11": stat("11", "10", "S")}, true},
		{"only zombies", map[string]string{"10": stat("10", "10", "Z"), "11": stat("11", "10", "Z")}, false},
		{"a zombie leader and a sleeping child", map[string]string{"10": stat("10", "10", "Z"), "11": stat("11", "10", "S")}, true},
		{"a group whose id is a prefix of another's", map[string]string{"12": stat("12", "100", "S")}, false},
	}
	for _, c := range cases {
		root := t.TempDir()
		for pid, line := range c.members {
			if err := os.MkdirAll(filepath.Join(root, pid), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, pid, "stat"), []byte(line), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if got := (Reader{Root: root}).GroupAlive(10); got != c.alive {
			t.Errorf("%s: alive %v, want %v", c.name, got, c.alive)
		}
	}
	if !(Reader{Root: filepath.Join(t.TempDir(), "missing")}).GroupAlive(10) {
		t.Error("a tree that cannot be listed was taken for an ended group")
	}
}
