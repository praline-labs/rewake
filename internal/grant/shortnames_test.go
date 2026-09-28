package grant

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// standDrive makes a temporary directory answer as a Windows drive does: a
// link with a short name stands in for the drive's second name of a
// directory, which the drive does not list.
func standDrive(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	onDrive = func(path string) bool { return Within(path, root) }
	t.Cleanup(func() { onDrive = windowsDrive })
	return root
}

func TestShortNamesAreTheGeneratedShape(t *testing.T) {
	for element, want := range map[string]bool{
		"PROGRA~1": true, "PYTHON~1": true, "MICRO~12": true, "DOCUME~1.TXT": true, "PY1A2B~1": true,
		"Program Files": false, "a~b": false, "backup~": false, "LONGERNAME~1": false, "FILE~1.TOOLONG": false, "~1": true,
	} {
		if got := shortName(element); got != want {
			t.Errorf("%q: %v, want %v", element, got, want)
		}
	}
}

// A directory named by its short name is resolved to its long one, so a
// protected directory on PATH named the long way still protects it; one named
// the short way on PATH protects the long name too.
func TestAShortNameIsComparedByItsLongName(t *testing.T) {
	root := standDrive(t)
	python := filepath.Join(root, "Programs", "Python313")
	if err := os.MkdirAll(filepath.Join(python, "Lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	short := filepath.Join(root, "Programs", "PYTHON~1")
	if err := os.Symlink("Python313", short); err != nil {
		t.Fatal(err)
	}
	for name, env := range map[string]Env{
		"long on PATH":  {PATH: python},
		"short on PATH": {PATH: short},
	} {
		rules := env.Rules()
		for _, given := range []string{python, short, filepath.Join(short, "Lib")} {
			resolved := longNames(given)
			if strings.Contains(resolved, "~") {
				t.Fatalf("%s: %s resolved to %s", name, given, resolved)
			}
			var refusal *Refusal
			if err := rules.Check(given, resolved, false); !errors.As(err, &refusal) || refusal.Code != 2 || !strings.Contains(refusal.Message, "PATH") {
				t.Errorf("%s: %s granted: %v", name, given, err)
			}
		}
	}
}

// A short name rewake cannot match to a long one is refused: the checks could
// not tell what it names.
func TestAShortNameLeftUnmatchedIsRefused(t *testing.T) {
	root := standDrive(t)
	unmatched := filepath.Join(root, "WORK~1")
	if err := os.Mkdir(unmatched, 0o755); err != nil {
		t.Fatal(err)
	}
	err := Env{}.Rules().Check(unmatched, longNames(unmatched), false)
	var refusal *Refusal
	if !errors.As(err, &refusal) || refusal.Code != 2 || !strings.Contains(refusal.Message, "short name") {
		t.Fatalf("granted %s: %v", unmatched, err)
	}
	if err := (Env{}).Rules().Recheck(unmatched, false); err == nil {
		t.Fatalf("rechecked %s", unmatched)
	}
}

// On the real drive, when there is one: its short name leads to its long one.
func TestADrivesShortNameLeadsToItsLongName(t *testing.T) {
	info, err := os.Stat("/mnt/c/PROGRA~1")
	if err != nil || !info.IsDir() {
		t.Skip("no Windows drive with short names here")
	}
	if got := longNames("/mnt/c/PROGRA~1"); got != "/mnt/c/Program Files" {
		t.Fatalf("resolved to %s", got)
	}
}
