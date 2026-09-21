package alias

// Reading the files, and refusing what they must not be allowed to say.
//
// Split from alias_test.go by subject when that file passed the project's
// 400-line limit: what an alias *file* may contain and what a bad one does is
// one question, and what an alias expands into is another.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A file carried by a directory may not choose the role of the session
// launching there: the role decides what a session sees and may ask for.
func TestAProjectAliasMayNotSetARole(t *testing.T) {
	s := set(t, "[alias.x]\nharness = \"codex\"\nrewake = [\"--main\"]\n")
	_, err := s.Expand([]string{"x"}, []string{"codex"}, valued, []string{"main", "write", "general"}, codexLike)
	if err == nil {
		t.Fatal("a project alias set a role")
	}
	if !strings.Contains(err.Error(), "role belongs in the user alias file") {
		t.Fatalf("refused for the wrong reason: %v", err)
	}
}

// The same alias is fine in the user's own file.
func TestAUserAliasMaySetARole(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, userFile)
	if err := os.WriteFile(path, []byte("[alias.x]\nharness = \"codex\"\nrewake = [\"--main\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := newSet()
	s.read(path, "the user alias file", true)
	if _, err := s.Expand([]string{"x"}, []string{"codex"}, valued, []string{"main"}, codexLike); err != nil {
		t.Fatalf("a user alias could not set a role: %v", err)
	}
	if len(s.Notes) != 0 {
		t.Fatalf("a 0600 file produced notes: %v", s.Notes)
	}
}

// Permissions are said out loud for the user file: it can name a role, which
// is more than the settings file beside it can do.
func TestAUserFileOpenToOthersIsReported(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, userFile)
	if err := os.WriteFile(path, []byte("[alias.x]\nharness = \"codex\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := newSet()
	s.read(path, "the user alias file", true)
	if len(s.Notes) == 0 {
		t.Fatal("a world-readable user alias file produced no note")
	}
	if _, ok := s.aliases["x"]; !ok {
		t.Fatal("the warning also dropped the alias; it is a note, not a refusal")
	}
}

func TestAnUnknownNameIsLeftAlone(t *testing.T) {
	s := set(t, wcodex)
	got, err := s.Expand([]string{"inbox"}, []string{"codex"}, valued, nil, codexLike)
	if err != nil || len(got) != 1 || got[0] != "inbox" {
		t.Fatalf("got %v, %v", got, err)
	}
}

// An alias that expands into something unusable is refused, with what it
// expanded to — not passed down to fail three layers later about a flag the
// person never typed.
func TestAnAliasThatMakesNoSenseIsRefused(t *testing.T) {
	for _, tc := range []struct{ name, file, want string }{
		{"no-harness", "[alias.x]\nrewake = [\"--write\"]\n", "names no harness"},
		{"unknown-harness", "[alias.x]\nharness = \"nothing\"\n", "which is not a harness"},
		{"a-command-in-rewake", "[alias.x]\nharness = \"codex\"\nrewake = [\"send\"]\n", "only takes flags"},
		{"an-empty-argument", "[alias.x]\nharness = \"codex\"\nargs = [\"\"]\n", "empty argument"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := set(t, tc.file)
			_, err := s.Expand([]string{"x"}, []string{"codex", "claude"}, valued, nil, codexLike)
			if err == nil {
				t.Fatal("an unusable alias was accepted")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("refused for the wrong reason: %v", err)
			}
		})
	}
}

// A file that is present and broken says so. Silence here would mean a launch
// that quietly runs something other than what the person asked for.
func TestABrokenFileIsReported(t *testing.T) {
	s := set(t, "[alias.x\nharness =")
	if len(s.Notes) == 0 {
		t.Fatal("a broken alias file produced no note")
	}
	if len(s.Names()) != 0 {
		t.Fatalf("a broken file produced aliases: %v", s.Names())
	}
}

func TestNamesAreListedForARefusal(t *testing.T) {
	s := set(t, wcodex+"\n[alias.claude-main]\nharness = \"claude\"\n")
	if got := strings.Join(s.Names(), ","); got != "claude-main,wcodex" {
		t.Fatalf("got %q", got)
	}
}
