package claude

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/grant"
)

// A file tool's path is judged where it lands, links resolved and a relative
// one taken from the working directory: a link inside a grant leading out of
// it, or into its Git metadata, writes there and not in the grant; a link
// elsewhere leading into the grant writes in it.
func TestAGrantJudgesAWriteWhereItLands(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	work, granted, other := filepath.Join(root, "work"), filepath.Join(root, "grant"), filepath.Join(root, "other")
	for _, dir := range []string{work, granted, other, filepath.Join(granted, ".git")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for link, target := range map[string]string{
		filepath.Join(granted, "out"):  other,
		filepath.Join(granted, "meta"): filepath.Join(granted, ".git"),
		filepath.Join(work, "in"):      granted,
	} {
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
	}
	journal := []grant.Entry{{Path: granted, Message: "m1", Outcome: grant.Granted}}
	for _, c := range []struct {
		name, cwd, path string
		allowed         bool
	}{
		{name: "a link in the grant leading out of it", cwd: work, path: filepath.Join(granted, "out", "a.txt")},
		{name: "a link in the grant leading into its metadata", cwd: work, path: filepath.Join(granted, "meta", "config")},
		{name: "a link elsewhere leading into the grant", cwd: work, path: filepath.Join(work, "in", "a.txt"), allowed: true},
		{name: "a relative path from inside the grant", cwd: granted, path: "a.txt", allowed: true},
		{name: "a relative path from elsewhere", cwd: work, path: "a.txt"},
	} {
		t.Run(c.name, func(t *testing.T) {
			payload := hookPayload(permissionRequest, "Write", "acceptEdits", c.cwd, map[string]string{"file_path": c.path, "content": "probe\n"}, granted)
			call, _ := GrantCall(payload, true)
			decided := DecideGrant(call, journal)
			if allowed := strings.Contains(string(decided.Output), `"behavior":"allow"`); allowed != c.allowed {
				t.Fatalf("allowed %v, want %v: %s", allowed, c.allowed, decided.Output)
			}
			if c.allowed && !slices.Equal(decided.Added, []string{granted}) {
				t.Fatalf("added %v", decided.Added)
			}
		})
	}
}

// A read is forced into the question that takes a grant back only where it
// would run unasked anyway: in the working directory, where it lands. A read
// through a link out of it would be asked about by the harness itself, and a
// search naming no path searches the working directory.
func TestOnlyAReadLandingInTheWorkingDirectoryIsForced(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	work, old, other := filepath.Join(root, "work"), filepath.Join(root, "old"), filepath.Join(root, "other")
	for _, dir := range []string{work, old, other} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(other, filepath.Join(work, "out")); err != nil {
		t.Fatal(err)
	}
	journal := []grant.Entry{{Path: old, Message: "m0", Outcome: grant.Revoking}}
	for _, c := range []struct {
		name, tool string
		input      map[string]string
		forced     bool
	}{
		{name: "a read through a link out of the working directory", tool: "Read", input: map[string]string{"file_path": filepath.Join(work, "out", "README")}},
		{name: "a relative read in it", tool: "Read", input: map[string]string{"file_path": "README"}, forced: true},
		{name: "a search naming no path", tool: "Grep", input: map[string]string{"pattern": "x"}, forced: true},
	} {
		t.Run(c.name, func(t *testing.T) {
			call, _ := GrantCall(hookPayload(preToolUse, c.tool, "acceptEdits", work, c.input), true)
			decided := DecideGrant(call, journal)
			if forced := strings.Contains(string(decided.Output), `"permissionDecision":"ask"`); forced != c.forced {
				t.Fatalf("forced %v, want %v: %s", forced, c.forced, decided.Output)
			}
		})
	}
}
