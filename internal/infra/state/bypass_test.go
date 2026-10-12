package state

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// bypass is one exception: a file allowed an effect, how many times, and
// why. A file and effect found more often than the entry says, or less, fails:
// the table holds what it excuses and nothing it no longer does.
type bypass struct {
	file, effect string
	count        int
	reason       string
}

// TestStateWritesHaveNoBypass reads every production file under internal/,
// whatever build tags it carries, so the four build variants are all covered
// at once, and fails on a write that does not go through this package.
func TestStateWritesHaveNoBypass(t *testing.T) {
	root := filepath.Join(moduleDir(t), "internal")
	found := map[[2]string]int{}
	var where []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "testdata" || strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(filepath.Dir(root), path)
		rel = filepath.ToSlash(rel)
		if filepath.ToSlash(filepath.Dir(rel)) == "internal/infra/state" {
			return nil
		}
		for _, ref := range bypassRefs(t, path) {
			found[[2]string{rel, ref.effect}]++
			where = append(where, fmt.Sprintf("%s:%d: %s", rel, ref.line, ref.effect))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, problem := range bypassProblems(found, bypassExceptions) {
		t.Error(problem)
	}
	if t.Failed() {
		slices.Sort(where)
		t.Logf("every write found:\n%s", strings.Join(where, "\n"))
	}
}

// bypassProblems compares what was found with the exceptions: an effect no
// entry excuses, one found more or fewer times than its entry counts, an
// entry without a reason.
func bypassProblems(found map[[2]string]int, exceptions []bypass) []string {
	var problems []string
	excused := map[[2]string]bool{}
	for _, entry := range exceptions {
		key := [2]string{entry.file, entry.effect}
		excused[key] = true
		switch {
		case entry.reason == "":
			problems = append(problems, fmt.Sprintf("%s: %s is excused without a reason", entry.file, entry.effect))
		case found[key] == 0:
			problems = append(problems, fmt.Sprintf("%s: %s is excused and no longer made: a stale entry", entry.file, entry.effect))
		case found[key] != entry.count:
			problems = append(problems, fmt.Sprintf("%s: %s is made %d times, the entry excuses %d", entry.file, entry.effect, found[key], entry.count))
		}
	}
	for key, n := range found {
		if !excused[key] {
			problems = append(problems, fmt.Sprintf("%s: %s, %d times, bypasses the state's seam", key[0], key[1], n))
		}
	}
	slices.Sort(problems)
	return problems
}

// The check counts every way a write is reached, and only writes: a value
// kept and called later, an argument, a second import name, a dot import, a
// reference in a selector's receiver; an open whose flag is not spelled from
// read flags alone, a writable mapping, a socket on a path. A read-only open,
// a read-only mapping, a network socket and an abstract one are no writes.
func TestTheBypassCheckSeesEveryWrite(t *testing.T) {
	writes := map[string]string{
		"a function value":      "import \"os\"\nvar rm = os.Remove",
		"an argument":           "import \"os\"\nfunc f() { run(os.RemoveAll) }",
		"a second name":         "import o \"os\"\nfunc f() { _ = o.WriteFile(\"p\", nil, 0) }",
		"a dot import":          "import . \"os\"\nfunc f() { _ = Rename(\"a\", \"b\") }",
		"a selector's receiver": "import \"os\"\nvar f = (struct{ F any }{os.Chmod}).F",
		"a write flag":          "import \"os\"\nfunc f() { _, _ = os.OpenFile(\"p\", os.O_RDONLY|os.O_CREATE, 0) }",
		"a flag in a variable":  "import \"os\"\nfunc f(flag int) { _, _ = os.OpenFile(\"p\", flag, 0) }",
		"a flag as a number":    "import \"os\"\nfunc f() { _, _ = os.OpenFile(\"p\", 1, 0) }",
		"a raw open":            "import \"syscall\"\nfunc f() { _, _ = syscall.Open(\"p\", syscall.O_WRONLY, 0) }",
		"a writable mapping":    "import \"syscall\"\nfunc f() { _, _ = syscall.Mmap(0, 0, 8, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED) }",
		"a lock kept as value":  "import s \"syscall\"\nvar lock = s.Flock",
		"a socket on a path":    "import \"net\"\nfunc f(p string) { _, _ = net.Listen(\"unix\", p) }",
		"a socket by address":   "import \"net\"\nfunc f(p string) { _, _ = net.ListenUnix(\"unix\", &net.UnixAddr{Name: p, Net: \"unix\"}) }",
		"a listener as value":   "import \"net\"\nvar listen = net.Listen",
	}
	for name, body := range writes {
		if found := bypassRefs(t, writeSource(t, body)); len(found) == 0 {
			t.Errorf("the check missed a write reached through %s", name)
		}
	}
	reads := map[string]string{
		"a read-only open":    "import (\n\"os\"\n\"syscall\"\n)\nfunc f() { _, _ = os.OpenFile(\"p\", os.O_RDONLY|syscall.O_NOFOLLOW, 0) }",
		"a raw read":          "import \"syscall\"\nfunc f() { _, _ = syscall.Open(\"p\", syscall.O_RDONLY|syscall.O_CLOEXEC, 0) }",
		"a read-only mapping": "import \"syscall\"\nfunc f() { _, _ = syscall.Mmap(0, 0, 8, syscall.PROT_READ, syscall.MAP_SHARED) }",
		"a network socket":    "import \"net\"\nfunc f() { _, _ = net.Listen(\"tcp\", \"127.0.0.1:0\") }",
		"an abstract socket":  "import \"net\"\nfunc f() { _, _ = net.ListenUnix(\"unix\", &net.UnixAddr{Name: \"@a\", Net: \"unix\"}) }",
	}
	for name, body := range reads {
		if found := bypassRefs(t, writeSource(t, body)); len(found) != 0 {
			t.Errorf("the check took %s for a write: %v", name, found)
		}
	}
}

// A descriptor changes its file's mode or owner whatever it was opened for: a
// read-only open gives one that can chmod. Its Chmod and Chown count wherever
// they are reached, called, kept or named by method expression, and under a
// name that shadows an import; a function of the same name in another package
// is that package's, and a content write needs a writable descriptor, which
// only a counted or guarded open gives.
func TestTheBypassCheckSeesADescriptorsMetadataChange(t *testing.T) {
	writes := map[string]string{
		"a read-opened chmod":               "import \"os\"\nfunc f(path string) error { file, err := os.Open(path); if err != nil { return err }; defer file.Close(); return file.Chmod(0o600) }",
		"a chown":                           "import \"os\"\nfunc f(file *os.File) { _ = file.Chown(0, 0) }",
		"a method value":                    "import \"os\"\nfunc f(file *os.File) func(os.FileMode) error { return file.Chmod }",
		"a method expression":               "import \"os\"\nvar chmod = (*os.File).Chmod",
		"a nested receiver":                 "import \"os\"\nfunc f(s struct{ F *os.File }) { _ = s.F.Chmod(0) }",
		"a descriptor shadowing an import":  "import (\"os\"; \"io\")\nvar _ io.Reader\nfunc f(path string) error { io, err := os.Open(path); if err != nil { return err }; defer io.Close(); return io.Chmod(0o600) }",
		"a parameter shadowing an import":   "import (\"os\"; \"io\")\nvar _ io.Reader\nfunc f(io *os.File) { _ = io.Chown(0, 0) }",
		"a range value shadowing an import": "import (\"os\"; \"io\")\nvar _ io.Reader\nfunc f(files []*os.File) { for _, io := range files { _ = io.Chmod(0) } }",
		"a var shadowing an alias":          "import (\"os\"; fs \"io/fs\")\nvar _ fs.FileMode\nfunc f(file *os.File) { var fs = file; _ = fs.Chmod(0) }",
	}
	for name, body := range writes {
		if found := bypassRefs(t, writeSource(t, body)); len(found) == 0 {
			t.Errorf("the check missed %s", name)
		}
	}
	others := map[string]string{
		"another package's function": "import \"example.com/state\"\nfunc f() { _ = state.Chmod(\"p\", 0) }",
		"a content write":            "import \"os\"\nfunc f(file *os.File) { _, _ = file.Write(nil) }",
	}
	for name, body := range others {
		if found := bypassRefs(t, writeSource(t, body)); len(found) != 0 {
			t.Errorf("the check took %s for a write: %v", name, found)
		}
	}
}

// Only an address whose value is proven abstract, "@" first, makes no file: a
// literal, a concatenation that begins with one, a UnixAddr naming one. Any
// other expression may name a path, a call or a variable among them, whatever
// literal it holds inside.
func TestTheBypassCheckExcusesOnlyAProvenAbstractAddress(t *testing.T) {
	paths := map[string]string{
		"a computed address":     "import (\"net\"; \"strings\")\nfunc f(path string) { _, _ = net.Listen(\"unix\", strings.TrimPrefix(\"@\" + path, \"@\")) }",
		"an address in a var":    "import \"net\"\nfunc f() { a := \"@x\"; _, _ = net.Listen(\"unix\", a) }",
		"a concatenation after":  "import \"net\"\nfunc f(p string) { _, _ = net.Listen(\"unix\", p + \"@\") }",
		"a UnixAddr of a call":   "import (\"net\"; \"strings\")\nfunc f(p string) { _, _ = net.ListenUnix(\"unix\", &net.UnixAddr{Name: strings.TrimPrefix(\"@\" + p, \"@\"), Net: \"unix\"}) }",
		"a UnixAddr's other key": "import \"net\"\nfunc f(p string) { _, _ = net.ListenUnix(\"unix\", &net.UnixAddr{Name: p, Net: \"@\"}) }",
	}
	for name, body := range paths {
		if found := bypassRefs(t, writeSource(t, body)); len(found) == 0 {
			t.Errorf("the check excused %s as abstract", name)
		}
	}
	abstract := map[string]string{
		"a literal":          "import \"net\"\nfunc f() { _, _ = net.Listen(\"unix\", \"@a\") }",
		"a concatenation":    "import \"net\"\nfunc f(p string) { _, _ = net.Listen(\"unix\", \"@\" + p) }",
		"a UnixAddr":         "import \"net\"\nfunc f(p string) { _, _ = net.ListenUnix(\"unix\", &net.UnixAddr{Name: \"@\" + p, Net: \"unix\"}) }",
		"a positional field": "import \"net\"\nfunc f() { _, _ = net.ListenUnix(\"unix\", &net.UnixAddr{\"@a\", \"unix\"}) }",
	}
	for name, body := range abstract {
		if found := bypassRefs(t, writeSource(t, body)); len(found) != 0 {
			t.Errorf("the check took %s for a file: %v", name, found)
		}
	}
}

// The table fails on what it no longer excuses and on what it excuses
// otherwise than found, as on a write it does not excuse.
func TestTheBypassTableFailsOnAStaleEntry(t *testing.T) {
	found := map[[2]string]int{{"a.go", "os.Remove"}: 2, {"b.go", "os.Link"}: 1}
	cases := map[string][]bypass{
		"a stale entry":         {{"a.go", "os.Remove", 2, "why"}, {"b.go", "os.Link", 1, "why"}, {"c.go", "os.Chmod", 1, "why"}},
		"a count too low":       {{"a.go", "os.Remove", 1, "why"}, {"b.go", "os.Link", 1, "why"}},
		"a count too high":      {{"a.go", "os.Remove", 3, "why"}, {"b.go", "os.Link", 1, "why"}},
		"an entry with no why":  {{"a.go", "os.Remove", 2, ""}, {"b.go", "os.Link", 1, "why"}},
		"a write not excused":   {{"a.go", "os.Remove", 2, "why"}},
		"an entry of an effect": {{"a.go", "os.Rename", 2, "why"}, {"b.go", "os.Link", 1, "why"}},
	}
	for name, table := range cases {
		if problems := bypassProblems(found, table); len(problems) == 0 {
			t.Errorf("the table passed with %s", name)
		}
	}
	exact := []bypass{{"a.go", "os.Remove", 2, "why"}, {"b.go", "os.Link", 1, "why"}}
	if problems := bypassProblems(found, exact); len(problems) != 0 {
		t.Errorf("an exact table failed: %v", problems)
	}
}

// writeSource writes a file of a package outside the state with body.
func writeSource(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "outside.go")
	if err := os.WriteFile(path, []byte("package outside\n"+body+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func moduleDir(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the test")
		}
		dir = parent
	}
}
