package launch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/core/worktype"
)

// typesRoom is a private home and working directory; files maps a path under
// one of them, "home/…" or "work/…", to its contents.
func typesRoom(t *testing.T, files map[string]string) *Set {
	t.Helper()
	home, work := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(work)
	for name, contents := range files {
		base, rest, _ := strings.Cut(name, "/")
		root := home
		if base == "work" {
			root = work
		}
		path := filepath.Join(root, rest)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return Load()
}

func typeFile(description, text string) string {
	return "+++\ndescription = \"" + description + "\"\n+++\n" + text + "\n"
}

// The nearest type of one name wins: the project's over the person's over
// rewake's own; a template names its default and keeps it unresolved.
func TestTheNearestTypeWins(t *testing.T) {
	set := typesRoom(t, map[string]string{
		"home/.config/rewake/types/code.md":     typeFile("mine", "the person's code"),
		"home/.config/rewake/types/study.md":    typeFile("mine", "the person's study"),
		"work/.rewake/types/study.md":           typeFile("ours", "the project's study"),
		"work/.rewake/types/notes.txt":          "not a type",
		"work/.rewake/aliases.toml":             "[alias.w]\nharness = \"agent\"\ntype = \"study\"\n",
		"home/.config/rewake/aliases.toml":      "[alias.u]\nharness = \"agent\"\n",
		"home/.config/rewake/types/.hidden.md":  "skipped",
		"home/.config/rewake/types/review.md.x": "skipped",
	})
	if err := set.Err(); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"code": "the person's code", "study": "the project's study", "review": worktype.BuiltIns()["review"].Text} {
		got, err := set.Type(name)
		if err != nil || got.Text != want {
			t.Fatalf("%s: %+v %v", name, got, err)
		}
	}
	if study, _ := set.Type("study"); study.Source != worktype.Project {
		t.Fatalf("study from %s", study.Source)
	}
	var names []string
	for _, value := range set.Types() {
		names = append(names, value.Name)
	}
	if strings.Join(names, ",") != "analysis,code,process,review,study" {
		t.Fatalf("types: %v", names)
	}
	if _, err := set.Type("missing"); err == nil || !strings.Contains(err.Error(), "available: analysis, code") {
		t.Fatalf("an unknown type: %v", err)
	}
	if template, err := set.Template("w", catalog); err != nil || template.Type != "study" {
		t.Fatalf("the template: %+v %v", template, err)
	}
}

// [types] off in both alias files is joined: a built-in type off in either is
// gone, and refused saying where; a file of that name still stands; a name
// that is no built-in type refuses the file, as an unknown field does.
func TestBuiltInTypesTurnedOff(t *testing.T) {
	set := typesRoom(t, map[string]string{
		"home/.config/rewake/aliases.toml":      "[types]\noff = [\"process\", \"analysis\"]\n",
		"work/.rewake/aliases.toml":             "[types]\noff = [\"process\", \"review\"]\n[alias.w]\nharness = \"agent\"\ntype = \"review\"\n",
		"home/.config/rewake/types/analysis.md": typeFile("mine", "the person's analysis"),
	})
	if err := set.Err(); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, value := range set.Types() {
		names = append(names, value.Name)
	}
	if strings.Join(names, ",") != "analysis,code" {
		t.Fatalf("types: %v", names)
	}
	if got, err := set.Type("analysis"); err != nil || got.Source != worktype.User {
		t.Fatalf("the person's own analysis: %+v %v", got, err)
	}
	_, err := set.Type("process")
	if err == nil || !strings.Contains(err.Error(), "turned off in the user alias file and the project alias file") {
		t.Fatalf("process: %v", err)
	}
	if _, err := set.Type("review"); err == nil || !strings.Contains(err.Error(), "turned off in the project alias file") {
		t.Fatalf("review: %v", err)
	}

	set = typesRoom(t, map[string]string{"home/.config/rewake/aliases.toml": "[types]\noff = [\"nosuch\"]\n"})
	if err := set.Err(); err == nil || !strings.Contains(err.Error(), `turns off "nosuch"`) {
		t.Fatalf("an unknown name in off: %v", err)
	}
	set = typesRoom(t, map[string]string{"home/.config/rewake/aliases.toml": "[types]\nhidden = [\"code\"]\n"})
	if err := set.Err(); err == nil || !strings.Contains(err.Error(), "types.hidden") {
		t.Fatalf("an unknown field of [types]: %v", err)
	}
}

// A project's aliases moved into .rewake/ on October 21, 2026: the old file
// refuses the catalog, naming the new place; a type file that does not read
// refuses it too.
func TestTheOldProjectFileAndABrokenTypeAreRefused(t *testing.T) {
	set := typesRoom(t, map[string]string{"work/.rewake.toml": "[alias.w]\nharness = \"agent\"\n"})
	if err := set.Err(); err == nil || !strings.Contains(err.Error(), filepath.Join(".rewake", "aliases.toml")) {
		t.Fatalf("the old file: %v", err)
	}
	set = typesRoom(t, map[string]string{"work/.rewake/types/study.md": "no front matter"})
	if err := set.Err(); err == nil || !strings.Contains(err.Error(), "study.md") {
		t.Fatalf("a broken type: %v", err)
	}
}
