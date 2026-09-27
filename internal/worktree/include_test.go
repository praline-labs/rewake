package worktree

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

// The patterns of .worktreeinclude mean what they would in a .gitignore: git
// itself is the judge, over a tree holding a file for each case.
func TestIncludeRulesMatchAsGitDoes(t *testing.T) {
	isolate(t)
	patterns := strings.Join([]string{
		"# a comment",
		"*.env",
		"!keep.env",
		"/config/local.json",
		"secrets/",
		"a/**/b.txt",
		"**/deep",
		"logs/**",
		"[ab].key",
		"[!c]x.bin",
		`\#hash`,
		"trailing   ",
		"docs/*.md",
		"?y",
		"nested/only/",
		"[[:digit:]]x.cfg",
		"[![:alpha:]]q.cfg",
		"[]a]r.cfg",
		`[\]b]s.cfg`,
		"",
	}, "\n")
	files := []string{
		"root.env", "sub/dir/x.env", "keep.env", "sub/keep.env",
		"config/local.json", "sub/config/local.json",
		"secrets/token", "sub/secrets/token", "secrets.txt",
		"a/b.txt", "a/x/y/b.txt", "c/a/b.txt",
		"deep", "x/deep", "y/deep/file",
		"logs/one", "logs/two/three", "sub/logs/one",
		"a.key", "b.key", "c.key",
		"dx.bin", "cx.bin",
		"#hash", "trailing", "docs/one.md", "docs/sub/two.md",
		"zy", "zzy", "nested/only/file", "nested/onlyfile",
		"1x.cfg", "ax.cfg", "1q.cfg", "aq.cfg", "]r.cfg", "ar.cfg", "cr.cfg", "]s.cfg", "bs.cfg", "cs.cfg",
	}
	top := t.TempDir()
	for _, file := range files {
		write(t, filepath.Join(top, filepath.FromSlash(file)), "x\n")
	}
	write(t, filepath.Join(top, ".gitignore"), patterns)
	must(t, top, "init", "-q", "-b", "main")
	want, err := ignoredEntries(top)
	if err != nil {
		t.Fatal(err)
	}
	rules, unread := parseIgnoreRules(patterns)
	if len(unread) != 0 {
		t.Errorf("unread lines: %v", unread)
	}
	var got []string
	for _, file := range files {
		if rules.matches(file, false) {
			got = append(got, file)
		}
	}
	sort.Strings(want)
	sort.Strings(got)
	if !slices.Equal(got, want) {
		t.Errorf("matched %v\ngit ignores %v", got, want)
	}
}

// A new checkout gets a copy of what .worktreeinclude names among the files git
// ignores — a tracked file it names is the commit's, and a symbolic link is
// skipped — including a file in a wholly ignored directory a pattern names,
// while an ignored directory nothing names is not walked.
func TestCreateCopiesWhatWorktreeIncludeNames(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	write(t, filepath.Join(source, ".gitignore"), ".env\n*.local\nbuild/\nconfig/secrets/\nlinked.env\n")
	write(t, filepath.Join(source, IncludeFile), ".env\n*.local\nconfig/secrets/api.key\nlinked.env\nsrc/nested/file\n")
	must(t, source, "add", ".gitignore", IncludeFile)
	must(t, source, "commit", "-q", "-m", "Ignore")
	write(t, filepath.Join(source, ".env"), "TOKEN=x\n")
	write(t, filepath.Join(source, "src", "app.local"), "local\n")
	write(t, filepath.Join(source, "build", "big.local"), "ignored dir\n")
	write(t, filepath.Join(source, "config", "secrets", "api.key"), "key\n")
	write(t, filepath.Join(source, "config", "secrets", "other"), "other\n")
	write(t, filepath.Join(source, "src", "nested", "file"), "changed in the source\n")
	if err := os.Symlink(filepath.Join(source, ".env"), filepath.Join(source, "linked.env")); err != nil {
		t.Fatal(err)
	}
	record, err := Create(t.TempDir(), source, "work")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(record.Included)
	if want := []string{".env", "config/secrets/api.key", "src/app.local"}; !slices.Equal(record.Included, want) || len(record.Skipped) != 0 {
		t.Errorf("included %v, skipped %v; want %v", record.Included, record.Skipped, want)
	}
	for file, text := range map[string]string{".env": "TOKEN=x\n", "config/secrets/api.key": "key\n", "src/nested/file": "one\n"} {
		if got, err := os.ReadFile(filepath.Join(record.Path, file)); err != nil || string(got) != text {
			t.Errorf("%s: %q, %v", file, got, err)
		}
	}
	for _, absent := range []string{"linked.env", "build/big.local", "config/secrets/other"} {
		if _, err := os.Lstat(filepath.Join(record.Path, absent)); !os.IsNotExist(err) {
			t.Errorf("%s was copied: %v", absent, err)
		}
	}
	if info, err := os.Lstat(filepath.Join(record.Path, ".env")); err != nil || !info.Mode().IsRegular() {
		t.Errorf(".env is not a copy: %v", err)
	}

	// The copies are no work of the checkout's while they equal the source's.
	if check, err := Inspect(record); err != nil || check.Dirty() {
		t.Errorf("fresh copies: %+v, %v", check, err)
	}
	write(t, filepath.Join(record.Path, ".env"), "TOKEN=changed\n")
	if check, _ := Inspect(record); !check.Ignored {
		t.Errorf("a changed copy: %+v", check)
	}
	write(t, filepath.Join(record.Path, ".env"), "TOKEN=x\n")
	write(t, filepath.Join(record.Path, "config", "secrets", "new"), "mine\n")
	if check, _ := Inspect(record); !check.Ignored {
		t.Errorf("a new file beside the copies: %+v", check)
	}
}

// Without the file nothing is copied, not even what git ignores.
func TestNoWorktreeIncludeCopiesNothing(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	write(t, filepath.Join(source, ".gitignore"), ".env\n")
	must(t, source, "add", ".gitignore")
	must(t, source, "commit", "-q", "-m", "Ignore")
	write(t, filepath.Join(source, ".env"), "TOKEN=x\n")
	record, err := Create(t.TempDir(), source, "work")
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Included) != 0 || len(record.Skipped) != 0 {
		t.Errorf("included %v, skipped %v", record.Included, record.Skipped)
	}
	if _, err := os.Stat(filepath.Join(record.Path, ".env")); !os.IsNotExist(err) {
		t.Errorf(".env copied: %v", err)
	}
}

// A link the commit holds leads the copy nowhere: a file whose directory in the
// checkout is such a link is skipped, and nothing is made where it points.
func TestIncludeStaysInsideTheCheckout(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(source, "out")); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(source, ".gitignore"), "out/deep/\n")
	write(t, filepath.Join(source, IncludeFile), "out/deep/secret\n")
	must(t, source, "add", "out", ".gitignore", IncludeFile)
	must(t, source, "commit", "-q", "-m", "Link")
	if err := os.Remove(filepath.Join(source, "out")); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(source, "out", "deep", "secret"), "key\n")
	record, err := Create(t.TempDir(), source, "work")
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Included) != 0 || len(record.Skipped) != 1 || !strings.Contains(record.Skipped[0], "not a directory") {
		t.Errorf("included %v, skipped %v", record.Included, record.Skipped)
	}
	if _, err := os.Lstat(filepath.Join(outside, "deep")); !os.IsNotExist(err) {
		t.Errorf("made a directory through the link: %v", err)
	}
}
