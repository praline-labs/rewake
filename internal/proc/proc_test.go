package proc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixture writes a /proc-shaped tree: pid -> (parent, start time). The command
// name contains a space and a parenthesis on purpose, because that is what
// breaks a parser that splits the stat line on spaces.
func fixture(t *testing.T, processes map[int][2]int) Reader {
	t.Helper()
	root := t.TempDir()
	for pid, values := range processes {
		dir := filepath.Join(root, itoa(pid))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		// Fields: pid, name, state, ppid, then filler up to field 21, then the
		// start time as field 22.
		line := itoa(pid) + " (my app (2)) S " + itoa(values[0])
		for field := 5; field <= 21; field++ {
			line += " 0"
		}
		line += " " + itoa(values[1]) + " 0 0\n"
		if err := os.WriteFile(filepath.Join(dir, "stat"), []byte(line), 0o644); err != nil {
			t.Fatalf("write stat: %v", err)
		}
	}
	return Reader{Root: root}
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := ""
	for value > 0 {
		digits = string(rune('0'+value%10)) + digits
		value /= 10
	}
	return digits
}

func TestStartTimeSurvivesASpacedCommandName(t *testing.T) {
	reader := fixture(t, map[int][2]int{7: {1, 4242}})

	start, err := reader.StartTime(7)
	if err != nil {
		t.Fatalf("StartTime: %v", err)
	}
	if start != 4242 {
		t.Errorf("start = %d, want 4242", start)
	}
}

func TestAliveComparesStartTime(t *testing.T) {
	reader := fixture(t, map[int][2]int{7: {1, 4242}})

	if !reader.Alive(7, 4242) {
		t.Error("the running process was reported dead")
	}
	if reader.Alive(7, 9999) {
		t.Error("a reused pid was reported alive")
	}
	if reader.Alive(8, 4242) {
		t.Error("a missing process was reported alive")
	}
}

func TestDescendantsWalksTheTree(t *testing.T) {
	// 10 -> 11 -> 12, plus an unrelated 20.
	reader := fixture(t, map[int][2]int{
		10: {1, 100},
		11: {10, 110},
		12: {11, 120},
		20: {1, 200},
	})

	found, err := reader.Descendants(10)
	if err != nil {
		t.Fatalf("Descendants: %v", err)
	}
	seen := map[int]bool{}
	for _, pid := range found {
		seen[pid] = true
	}
	for _, want := range []int{10, 11, 12} {
		if !seen[want] {
			t.Errorf("pid %d missing from the tree", want)
		}
	}
	if seen[20] {
		t.Error("an unrelated process was included")
	}
}

func TestOpenFilesResolvesLinks(t *testing.T) {
	reader := fixture(t, map[int][2]int{7: {1, 100}})
	fdDir := filepath.Join(reader.Root, "7", "fd")
	if err := os.MkdirAll(fdDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Two descriptors, because the caller looks for the newest of several lock
	// files: stopping at the first one would answer with the wrong thread.
	base := t.TempDir()
	first := filepath.Join(base, "first.lock")
	second := filepath.Join(base, "second.lock")
	for index, target := range []string{first, second} {
		if err := os.WriteFile(target, nil, 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		if err := os.Symlink(target, filepath.Join(fdDir, itoa(3+index))); err != nil {
			t.Fatalf("symlink: %v", err)
		}
	}

	files, err := reader.OpenFiles(7)
	if err != nil {
		t.Fatalf("OpenFiles: %v", err)
	}
	found := map[string]bool{}
	for _, file := range files {
		found[file] = true
	}
	if !found[first] || !found[second] {
		t.Errorf("files = %v, want both descriptors", files)
	}
}

func TestZombieIsNotAlive(t *testing.T) {
	reader := fixture(t, map[int][2]int{7: {1, 100}})
	// A zombie keeps its entry and its start time until the parent reaps it.
	stat := filepath.Join(reader.Root, "7", "stat")
	raw, err := os.ReadFile(stat)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if err := os.WriteFile(stat, []byte(strings.Replace(string(raw), ") S ", ") Z ", 1)), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if reader.Alive(7, 100) {
		t.Error("a zombie was reported alive; its session would keep accepting messages")
	}
}

func TestOpenFilesOfAMissingProcessIsEmpty(t *testing.T) {
	reader := fixture(t, map[int][2]int{7: {1, 100}})
	files, err := reader.OpenFiles(999)
	if err != nil {
		t.Fatalf("OpenFiles: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("files = %v, want none", files)
	}
}
