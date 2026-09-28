package grant

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// What the owner's shell, session manager and tools read and run is in the
// hard tier: a worker writing there runs as the owner the next time any of
// them starts, outside every sandbox.
func TestWhatRunsAsTheOwnerIsNeverGranted(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(root, "home")
	protected := []string{
		".config/fish", ".config/gh", ".local/share/systemd/user", ".local/share/applications",
		"go/pkg/mod", ".cache/go-build", "cache/mod", ".local/go/src", ".nvm/versions/node/v22/lib/node_modules",
	}
	for _, dir := range append(protected, "code/project", ".local/share/notes", ".local/bin") {
		if err := os.MkdirAll(filepath.Join(home, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, dir := range []string{".local/go/bin", ".nvm/versions/node/v22/bin"} {
		if err := os.MkdirAll(filepath.Join(home, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	path := strings.Join([]string{
		filepath.Join(home, ".local/bin"), filepath.Join(home, ".local/go/bin"), filepath.Join(home, ".nvm/versions/node/v22/bin"),
	}, string(os.PathListSeparator))
	rules := Env{Home: home, PATH: path, Caches: []string{filepath.Join(home, "cache/mod")}}.Rules()
	for _, dir := range protected {
		full := filepath.Join(home, dir)
		var refusal *Refusal
		if err := rules.Check(full, full, false); !errors.As(err, &refusal) || refusal.Code != 2 || !strings.Contains(refusal.Message, "never grants") {
			t.Errorf("%s: %v", dir, err)
		}
	}
	// ~/.local holds ~/.local/bin, and is no toolchain of its own: a
	// directory beside the protected ones in it is granted.
	for _, dir := range []string{"code/project", ".local/share/notes"} {
		full := filepath.Join(home, dir)
		if err := rules.Check(full, full, false); err != nil {
			t.Errorf("%s: %v", dir, err)
		}
	}
}

func TestAToolchainIsTheParentOfItsBin(t *testing.T) {
	home := "/home/someone"
	for dir, want := range map[string]string{
		"/home/someone/.local/go/bin":             "/home/someone/.local/go",
		"/home/someone/go/bin":                    "/home/someone/go",
		"/home/someone/.nvm/versions/node/v1/bin": "/home/someone/.nvm/versions/node/v1",
		"/home/someone/.local/bin":                "",
		"/home/someone/bin":                       "",
		"/bin":                                    "",
		"/home/someone/.cargo/shims":              "",
		"relative/bin":                            "",
	} {
		if got := toolchainOf(dir, home); got != want {
			t.Errorf("%s: %q, want %q", dir, got, want)
		}
	}
}

// A path through a shared temporary directory is refused wherever it leads:
// a sandboxed worker could have put the link there for main to name.
func TestAPathThroughATemporaryDirectoryIsRefused(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	temp, target := filepath.Join(root, "tmp"), filepath.Join(root, "home", "notes")
	for _, dir := range []string{temp, target} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(temp, "out")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	rules := Env{Temp: []string{temp}}.Rules()
	for _, given := range []string{link, "out"} {
		resolved, err := Resolve(given, temp)
		if err != nil || resolved != target {
			t.Fatalf("%s resolved to %s, %v", given, resolved, err)
		}
		var refusal *Refusal
		if err := rules.Named(given, temp, resolved); !errors.As(err, &refusal) || refusal.Code != 2 || !strings.Contains(refusal.Message, target) {
			t.Errorf("%s: %v", given, err)
		}
	}
	if err := rules.Named(target, root, target); err != nil {
		t.Errorf("the directory named by itself: %v", err)
	}
	// A link outside that passes through the worker's link on its way, and
	// one whose own target climbs into the temporary directory and out again.
	work := filepath.Join(root, "home", "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, via := range map[string]string{"link": link, "climb": "../../tmp/../home/notes"} {
		outside := filepath.Join(work, name)
		if err := os.Symlink(via, outside); err != nil {
			t.Fatal(err)
		}
		resolved, err := Resolve(outside, root)
		if err != nil || resolved != target {
			t.Fatalf("%s resolved to %s, %v", outside, resolved, err)
		}
		var refusal *Refusal
		err = rules.Named(outside, root, resolved)
		if name == "link" && (!errors.As(err, &refusal) || refusal.Code != 2 || !strings.Contains(refusal.Message, temp)) {
			t.Errorf("a link leading through %s: %v", link, err)
		}
		if name == "climb" && !errors.As(err, &refusal) {
			t.Errorf("a link climbing through %s: %v", temp, err)
		}
	}
	plain := filepath.Join(work, "plain")
	if err := os.Symlink(target, plain); err != nil {
		t.Fatal(err)
	}
	if err := rules.Named(plain, root, target); err != nil {
		t.Errorf("a link that never passes the temporary directory: %v", err)
	}
}

// A path past the links the walk follows is refused, not taken as checked:
// EvalSymlinks follows more, and whatever lay beyond — a hop through /tmp —
// would go unseen.
func TestAPathPastTheLinksFollowedIsRefused(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	temp, target, chains := filepath.Join(root, "tmp"), filepath.Join(root, "home", "notes"), filepath.Join(root, "home", "chains")
	for _, dir := range []string{temp, target, chains} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(temp, "out")
	if err := os.Symlink(target, out); err != nil {
		t.Fatal(err)
	}
	rules := Env{Temp: []string{temp}}.Rules()
	// chain makes count links, each leading to the next, the last to end.
	chain := func(name string, count int, end string) string {
		next := end
		for index := count; index > 0; index-- {
			link := filepath.Join(chains, fmt.Sprintf("%s-%d", name, index))
			if err := os.Symlink(next, link); err != nil {
				t.Fatal(err)
			}
			next = link
		}
		return next
	}
	cases := []struct {
		name    string
		given   string
		refused string
	}{
		{"at the limit", chain("forty", maxLinks, target), ""},
		{"past the limit", chain("fortyone", maxLinks+1, target), "more than"},
		{"through /tmp just past the limit", chain("hop", maxLinks+1, out), "more than"},
		{"through /tmp within the limit", chain("near", maxLinks-1, out), temp},
	}
	for _, c := range cases {
		resolved, err := Resolve(c.given, root)
		if err != nil || resolved != target {
			t.Fatalf("%s: resolved to %s, %v", c.name, resolved, err)
		}
		err = rules.Named(c.given, root, resolved)
		var refusal *Refusal
		switch {
		case c.refused == "" && err != nil:
			t.Errorf("%s: %v", c.name, err)
		case c.refused != "" && (!errors.As(err, &refusal) || refusal.Code != 2 || !strings.Contains(refusal.Message, c.refused)):
			t.Errorf("%s: %v", c.name, err)
		}
	}
}
