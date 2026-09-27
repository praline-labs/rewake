package worktree

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A line that makes no expression is named and left out, and the lines around
// it still apply: the file is a repository's content, and a stranger's line
// must not stop a launch.
func TestIncludeRulesSkipALineThatMakesNoExpression(t *testing.T) {
	for _, line := range []string{"[z-a]", "a[b-a]c/", "[[:nope:]]"} {
		rules, unread := parseIgnoreRules("*.env\n" + line + "\n!keep.env\n")
		if len(rules) != 2 || len(unread) != 1 || !strings.Contains(unread[0], line) {
			t.Errorf("%q: %d rules, unread %v", line, len(rules), unread)
			continue
		}
		if !rules.matches("a.env", false) || rules.matches("keep.env", false) {
			t.Errorf("%q: the other lines stopped applying", line)
		}
	}
}

// A checkout whose .worktreeinclude holds such a line is made, gets what the
// other lines name, and says which line it could not read.
func TestCreateSkipsAnUnreadableIncludeLine(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	write(t, filepath.Join(source, ".gitignore"), ".env\n")
	write(t, filepath.Join(source, IncludeFile), "[z-a]\n.env\n")
	must(t, source, "add", ".gitignore", IncludeFile)
	must(t, source, "commit", "-q", "-m", "Ignore")
	write(t, filepath.Join(source, ".env"), "TOKEN=x\n")
	record, err := Create(t.TempDir(), source, "work")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(record.Included, []string{".env"}) || len(record.Skipped) != 1 || !strings.Contains(record.Skipped[0], "[z-a]") {
		t.Errorf("included %v, skipped %v", record.Included, record.Skipped)
	}
	if _, err := os.Stat(filepath.Join(record.Path, ".env")); err != nil {
		t.Errorf(".env not copied: %v", err)
	}
}

// No text makes the parser or the matcher panic.
func FuzzParseIgnoreRules(f *testing.F) {
	for _, seed := range []string{"[z-a]", "a[b-a]c/", "[[:alpha:]]", "[![:digit:]x]", "[]", "[!]]", `[\]`, `\`, "**/[", "a/**/b", "[[:", "[a-\\]"} {
		f.Add(seed)
	}
	f.Fuzz(func(_ *testing.T, text string) {
		rules, _ := parseIgnoreRules(text)
		rules.matches("a/b/c", false)
		rules.opens("a")
	})
}

// A copy is compared a block at a time: a difference in its last byte counts,
// and a source file that is now a pipe is no copy and is not opened, which
// would wait for a writer forever.
func TestSameCopyStreams(t *testing.T) {
	checkout, source := t.TempDir(), t.TempDir()
	record := Record{Path: checkout, Source: source, Included: []string{"same", "last", "short", "pipe"}}
	big := strings.Repeat("0123456789abcdef", 20000)
	for file, text := range map[string]string{"same": big, "last": big[:len(big)-1] + "X", "short": big[:len(big)-1]} {
		write(t, filepath.Join(checkout, file), big)
		write(t, filepath.Join(source, file), text)
	}
	for file, want := range map[string]bool{"same": true, "last": false, "short": false} {
		if got := sameCopy(record, file); got != want {
			t.Errorf("%s: %v, want %v", file, got, want)
		}
	}
	write(t, filepath.Join(checkout, "pipe"), big)
	if err := syscall.Mkfifo(filepath.Join(source, "pipe"), 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan bool, 1)
	go func() { done <- sameCopy(record, "pipe") }()
	select {
	case got := <-done:
		if got {
			t.Error("a pipe was taken for a copy")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("comparing with a pipe waited for a writer")
	}
}
