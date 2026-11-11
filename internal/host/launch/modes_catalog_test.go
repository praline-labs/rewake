package launch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/core/workmode"
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

// The nearest mode of one name wins: the project's over the person's over
// rewake's own; a file of a built-in's name replaces its text and keeps its
// built-in place in the cycle.
func TestTheNearestModeWins(t *testing.T) {
	set := typesRoom(t, map[string]string{
		"home/.config/rewake/modes/build.md":    typeFile("mine", "the person's build"),
		"home/.config/rewake/modes/audit.md":    typeFile("mine", "the person's audit"),
		"work/.rewake/modes/audit.md":           typeFile("ours", "the project's audit"),
		"work/.rewake/modes/notes.txt":          "not a mode",
		"work/.rewake/aliases.toml":             "[alias.w]\nharness = \"agent\"\n[alias.w.pi]\nstart_mode = \"audit\"\n",
		"home/.config/rewake/aliases.toml":      "[alias.u]\nharness = \"agent\"\n",
		"home/.config/rewake/modes/.hidden.md":  "skipped",
		"home/.config/rewake/modes/review.md.x": "skipped",
	})
	if err := set.Err(); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"build": "the person's build", "audit": "the project's audit", "review": workmode.BuiltIns()["review"].Text} {
		got, err := set.Mode(name)
		if err != nil || got.Text != want {
			t.Fatalf("%s: %+v %v", name, got, err)
		}
	}
	if audit, _ := set.Mode("audit"); audit.Source != workmode.Project {
		t.Fatalf("audit from %s", audit.Source)
	}
	var names []string
	for _, value := range set.ModeList() {
		names = append(names, value.Name)
	}
	if strings.Join(names, ",") != "advisor,audit,build,process,review" {
		t.Fatalf("modes: %v", names)
	}
	if _, err := set.Mode("missing"); err == nil || !strings.Contains(err.Error(), "available: advisor") {
		t.Fatalf("an unknown mode: %v", err)
	}
	if template, err := set.Template("w", catalog); err != nil || set.Modes(template.Name).Start() != "audit" {
		t.Fatalf("the template: %+v %v", template, err)
	}
	// An overridden built-in keeps its built-in position in the cycle.
	var order []string
	for _, value := range set.Catalog() {
		order = append(order, value.Name)
	}
	if strings.Join(order, ",") != "review,advisor,build,process,audit" {
		t.Fatalf("cycle: %v", order)
	}
}

// The types of before this change are refused, each naming where its file
// goes, rather than read as if the mode were its type.
func TestTheOldTypeSyntaxIsRefusedWithWhereItGoes(t *testing.T) {
	for _, c := range []struct {
		files   map[string]string
		refused []string
	}{
		{
			map[string]string{"home/.config/rewake/types/code.md": typeFile("mine", "text")},
			[]string{"types", "modes", "code is now build"},
		},
		{
			map[string]string{"home/.config/rewake/aliases.toml": "[types]\noff = [\"process\"]\n"},
			[]string{"types (line 1)", "[types] section is no longer read", "modes/<name>.md"},
		},
		{
			map[string]string{
				"work/.rewake/modes/study.md": typeFile("ours", "text"),
				"work/.rewake/aliases.toml":   "[alias.w]\nharness = \"agent\"\ntype = \"study\"\n",
			},
			[]string{"alias.w.type", "start_mode", "--work-mode"},
		},
		{
			map[string]string{"home/.config/rewake/aliases.toml": "[pi.modes.nowhere]\neffort = \"high\"\n"},
			[]string{"[pi]", `"nowhere"`, "catalog does not hold"},
		},
		{
			map[string]string{"home/.config/rewake/aliases.toml": "[alias.w]\nharness = \"agent\"\n[alias.w.pi.modes.nothing]\neffort = \"low\"\n"},
			[]string{"alias \"w\"", "nothing"},
		},
	} {
		set := typesRoom(t, c.files)
		err := set.Err()
		if err == nil {
			t.Fatalf("accepted %v", c.files)
		}
		for _, want := range c.refused {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%v: %v, want %q", c.files, err, want)
			}
		}
	}
}

// A mode file that does not read refuses the catalog; the old project alias
// file still does.
func TestTheOldProjectFileAndABrokenModeAreRefused(t *testing.T) {
	set := typesRoom(t, map[string]string{"work/.rewake.toml": "[alias.w]\nharness = \"agent\"\n"})
	if err := set.Err(); err == nil || !strings.Contains(err.Error(), filepath.Join(".rewake", "aliases.toml")) {
		t.Fatalf("the old file: %v", err)
	}
	set = typesRoom(t, map[string]string{"work/.rewake/modes/study.md": "no front matter"})
	if err := set.Err(); err == nil || !strings.Contains(err.Error(), "study.md") {
		t.Fatalf("a broken mode: %v", err)
	}
}
