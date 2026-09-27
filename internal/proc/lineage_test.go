package proc

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestDescendsWalksParentsToTheAncestor(t *testing.T) {
	// 1 init; 10 the wrapper; 20 its harness; 30 a command; 40 a stranger.
	r := fixture(t, map[int][2]int{1: {0, 1}, 10: {1, 100}, 20: {10, 200}, 30: {20, 300}, 40: {1, 150}})
	for _, pid := range []int{10, 20, 30} {
		if err := r.Descends(pid, 10); err != nil {
			t.Errorf("%d: %v", pid, err)
		}
	}
	for _, pid := range []int{1, 40, 99} {
		if err := r.Descends(pid, 10); !errors.Is(err, ErrNotDescendant) {
			t.Errorf("%d descends: %v", pid, err)
		}
	}
}

// A pid taken again on the way up is a process that started after its
// supposed child: the walk stops there rather than follow a stranger.
func TestDescendsRefusesAReusedParent(t *testing.T) {
	r := fixture(t, map[int][2]int{1: {0, 1}, 10: {1, 500}, 30: {10, 300}})
	if err := r.Descends(30, 10); !errors.Is(err, ErrNotDescendant) {
		t.Fatalf("followed a reused pid: %v", err)
	}
}

func TestDescendsStopsOnALoop(t *testing.T) {
	r := fixture(t, map[int][2]int{5: {6, 100}, 6: {5, 100}})
	if err := r.Descends(5, 10); !errors.Is(err, ErrNotDescendant) {
		t.Fatalf("a loop: %v", err)
	}
}

func TestParentReadsBothFieldsOfOneLine(t *testing.T) {
	r := fixture(t, map[int][2]int{30: {20, 300}})
	parent, start, err := r.Parent(30)
	if err != nil || parent != 20 || start != 300 {
		t.Fatalf("Parent = %d, %d, %v", parent, start, err)
	}
}

func TestNamespacesReadsThreeLinks(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "7", "ns")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"mnt", "user", "pid"} {
		if err := os.Symlink(kind+":[1]", filepath.Join(dir, kind)); err != nil {
			t.Fatal(err)
		}
	}
	r := Reader{Root: root}
	got, err := r.Namespaces("7")
	if err != nil || got != "mnt:[1] user:[1] pid:[1]" {
		t.Fatalf("Namespaces = %q, %v", got, err)
	}
	if _, err := r.Namespaces("8"); err == nil {
		t.Fatal("a process with no links named namespaces")
	}
	// This process against itself: the real tree answers the same both times.
	self := strconv.Itoa(os.Getpid())
	first, err := Default.Namespaces(self)
	if err != nil {
		t.Fatal(err)
	}
	if second, _ := Default.Namespaces(self); first != second {
		t.Fatalf("%q then %q", first, second)
	}
}
