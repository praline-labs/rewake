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

// The types of before this change are skipped with a migration notice naming
// where each goes, rather than refusing a catalog that still carries one: main
// must start on what is valid and learn what to fix (docs/work-modes.md#the-old-worker-types).
func TestTheOldTypeSyntaxIsSkippedWithWhereItGoes(t *testing.T) {
	for _, c := range []struct {
		files   map[string]string
		skipped []string
		names   string
	}{
		{
			map[string]string{"home/.config/rewake/types/code.md": typeFile("mine", "text")},
			[]string{"types", "no longer read and is skipped", "modes", "code is now build"},
			"",
		},
		{
			map[string]string{"work/.rewake/types/study.md": typeFile("ours", "text")},
			[]string{filepath.Join(".rewake", "types"), "modes", "review and process keep theirs"},
			"",
		},
		{
			map[string]string{"home/.config/rewake/aliases.toml": "[types]\noff = [\"process\"]\n"},
			[]string{"types", "[types] section is no longer read", "modes/<name>.md"},
			"",
		},
		{
			map[string]string{
				"work/.rewake/modes/study.md": typeFile("ours", "text"),
				"work/.rewake/aliases.toml":   "[alias.w]\nharness = \"agent\"\ntype = \"study\"\n[alias.kept]\nharness = \"agent\"\n",
			},
			[]string{"alias.w", "obsolete type field", "left out until it is fixed", "start_mode", "aliases.toml:3"},
			"kept",
		},
	} {
		set := typesRoom(t, c.files)
		if err := set.Err(); err != nil {
			t.Fatalf("refused %v: %v", c.files, err)
		}
		if got := strings.Join(set.Names(), ","); got != c.names {
			t.Errorf("%v: names %q, want %q", c.files, got, c.names)
		}
		notices := strings.Join(set.Migrations, "\n")
		for _, want := range c.skipped {
			if !strings.Contains(notices, want) {
				t.Errorf("%v: notices %q lack %q", c.files, notices, want)
			}
		}
	}
}

// A template whose obsolete type is skipped is left out rather than started
// under a default, and space open refuses it with what to write.
func TestAnOmittedTemplateIsRefusedWithWhatToWrite(t *testing.T) {
	set := typesRoom(t, map[string]string{
		"work/.rewake/aliases.toml": "[alias.w]\nharness = \"agent\"\ntype = \"code\"\n",
	})
	if err := set.Err(); err != nil {
		t.Fatal(err)
	}
	_, err := set.Template("w", catalog)
	if err == nil || !strings.Contains(err.Error(), `the alias "w" is unavailable`) || !strings.Contains(err.Error(), "start_mode") {
		t.Fatalf("the omission: %v", err)
	}
}

// What rewake cannot recognize still refuses the file: a malformed value and a
// field no alias has, beside the obsolete settings it does recognize.
func TestTheObsoleteSettingsDoNotSoftenTheOtherRefusals(t *testing.T) {
	for _, c := range []struct {
		files   map[string]string
		refused []string
	}{
		{
			map[string]string{"home/.config/rewake/aliases.toml": "[pi.modes.nowhere]\neffort = \"high\"\n[types]\noff = [\"process\"]\n"},
			[]string{"[pi]", `"nowhere"`, "catalog does not hold"},
		},
		{
			map[string]string{"home/.config/rewake/aliases.toml": "[alias.w]\nharness = \"agent\"\n[alias.w.pi.modes.nothing]\neffort = \"low\"\n[types]\noff = [\"process\"]\n"},
			[]string{"alias \"w\"", "nothing"},
		},
		{
			map[string]string{"work/.rewake/aliases.toml": "[alias.w]\nharness = \"agent\"\ntype = \"code\"\ncolor = \"blue\"\n"},
			[]string{"alias.w.color", "fields an alias does not have"},
		},
		{
			map[string]string{"work/.rewake/aliases.toml": "[alias.w]\nharness = \"agent\"\ntype = \n"},
			[]string{"could not be read"},
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

// space open refuses a template the obsolete type left out, with what to write
// rather than a bare "no alias".
func TestSpaceOpenRefusesAnOmittedTemplateWithWhatToWrite(t *testing.T) {
	set := typesRoom(t, map[string]string{
		"work/.rewake/aliases.toml": "[alias.w]\nharness = \"agent\"\ntype = \"code\"\n",
	})
	_, err := set.Raise(workers, Request{Alias: "w"})
	if err == nil || !strings.Contains(err.Error(), `the alias "w" is unavailable`) || !strings.Contains(err.Error(), "start_mode") {
		t.Fatalf("space open: %v", err)
	}
}

// An obsolete type follows whole-template precedence: the layer that defines
// the template whole decides, the unrelated templates are untouched, and only
// a definition in force is omitted and named in a notice.
func TestAnObsoleteTypeFollowsWholeTemplatePrecedence(t *testing.T) {
	const kept = "[alias.keep]\nharness = \"agent\"\n"
	userObsolete := "[alias.w]\nharness = \"agent\"\ntype = \"code\"\n" + kept
	userValid := "[alias.w]\nharness = \"agent\"\ndescription = \"user w\"\n" + kept
	projectObsolete := "[alias.w]\nharness = \"agent\"\ntype = \"code\"\n"
	projectValid := "[alias.w]\nharness = \"agent\"\ndescription = \"project w\"\n"
	for name, tc := range map[string]struct {
		user, project        string
		names, description   string
		omitted, noticeNames string
	}{
		"obsolete user, valid project": {userObsolete, projectValid, "keep,w", "project w", "", ""},
		"valid user, obsolete project": {userValid, projectObsolete, "keep", "", "w", "project alias file"},
		"both obsolete":                {userObsolete, projectObsolete, "keep", "", "w", "project alias file"},
		"obsolete user only":           {userObsolete, "", "keep", "", "w", "user alias file"},
		"obsolete project only":        {"", projectObsolete + kept, "keep", "", "w", "project alias file"},
	} {
		s := offRoom(t, tc.user, tc.project)
		if err := s.Err(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := strings.Join(s.Names(), ","); got != tc.names {
			t.Fatalf("%s: names %q, want %q", name, got, tc.names)
		}
		if _, err := s.Template("keep", catalog); err != nil {
			t.Errorf("%s: the unrelated template: %v", name, err)
		}
		if tc.omitted == "" {
			template, err := s.Template("w", catalog)
			if err != nil || template.Description != tc.description {
				t.Fatalf("%s: the valid definition in force: %+v %v", name, template, err)
			}
			if len(s.Migrations) != 0 {
				t.Errorf("%s: a notice for a definition not in force: %v", name, s.Migrations)
			}
			continue
		}
		_, err := s.Template(tc.omitted, catalog)
		if err == nil || !strings.Contains(err.Error(), `the alias "w" is unavailable until its obsolete setting is fixed`) {
			t.Fatalf("%s: the omission: %v", name, err)
		}
		if len(s.Migrations) != 1 || !strings.Contains(s.Migrations[0], tc.noticeNames) || !strings.Contains(s.Migrations[0], "obsolete type field") {
			t.Errorf("%s: notices: %v", name, s.Migrations)
		}
	}
}

// Recognizing an obsolete key must not swallow its value: only the old form is
// skipped, and a malformed one refuses the file as any unknown field does.
func TestTheObsoleteFormsMustStillBeWellFormed(t *testing.T) {
	for name, tc := range map[string]struct{ file, want string }{
		"type is not a string":       {"[alias.w]\nharness = \"agent\"\ntype = 42\n", "must be a string"},
		"type is a list":             {"[alias.w]\nharness = \"agent\"\ntype = [\"code\"]\n", "must be a string"},
		"type is a table":            {"[alias.w]\nharness = \"agent\"\n[alias.w.type]\nname = \"code\"\n", "must be a string"},
		"types is not a table":       {"types = 5\n", "obsolete [types] section"},
		"types off is not a list":    {"[types]\noff = 42\n", "obsolete [types] section"},
		"types off holds a number":   {"[types]\noff = [\"process\", 42]\n", "obsolete [types] section"},
		"types has an unknown field": {"[types]\ncolro = \"blue\"\n", "obsolete [types] section"},
		"types has a subtable":       {"[types.review]\ndescription = \"x\"\n", "obsolete [types] section"},
	} {
		s := typesRoom(t, map[string]string{"work/.rewake/aliases.toml": tc.file})
		err := s.Err()
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want %q", name, err, tc.want)
		}
	}
}

// The old form stays eligible for a notice, including the inline table.
func TestTheWellFormedObsoleteFormsAreSkipped(t *testing.T) {
	for name, file := range map[string]string{
		"section": "[types]\noff = [\"process\"]\n",
		"inline":  "types = { off = [\"process\"] }\n",
		"empty":   "[types]\n",
	} {
		s := typesRoom(t, map[string]string{"work/.rewake/aliases.toml": file})
		if err := s.Err(); err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if len(s.Migrations) != 1 || !strings.Contains(s.Migrations[0], "[types] section is no longer read") {
			t.Errorf("%s: notices: %v", name, s.Migrations)
		}
	}
}

// An obsolete type may leave a valid template out; it may not screen a
// malformed current setting. The current schema is checked first, at the
// catalog and again when the omitted alias is asked for or listed.
func TestAnObsoleteTypeDoesNotHideMalformedModernSettings(t *testing.T) {
	typed := "[alias.w]\nharness = \"agent\"\ntype = \"code\"\n"
	for name, tc := range map[string]struct{ file, want string }{
		"start_mode plan":       {typed + "[alias.w.pi]\nstart_mode = \"plan\"\n", "plan"},
		"start_mode unknown":    {typed + "[alias.w.pi]\nstart_mode = \"nowhere\"\n", "catalog does not hold"},
		"mode effort malformed": {typed + "[alias.w.pi.modes.study]\neffort = \"loud\"\n", "effort"},
		"mode model malformed":  {typed + "[alias.w.pi.modes.study]\nmodel = \"no-provider\"\n", "provider/model-id"},
	} {
		s := typesRoom(t, map[string]string{"work/.rewake/aliases.toml": tc.file})
		if err := s.Err(); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want %q", name, err, tc.want)
		}
	}
	// A field the adapter checks is refused when the omitted alias is asked
	// for, by the same resolver a listed template goes through.
	s := typesRoom(t, map[string]string{"work/.rewake/aliases.toml": typed + "[alias.w.launch]\ncontext = \"bad\"\n"})
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Template("w", catalog); err == nil || !strings.Contains(err.Error(), "context bad is not a window") {
		t.Fatalf("the malformed setting: %v", err)
	}
	for name, tc := range map[string]struct{ file, want string }{
		"unknown harness": {"[alias.w]\nharness = \"nothing\"\ntype = \"code\"\n", "which is not a harness"},
		"main's role":     {"[alias.w]\nharness = \"agent\"\ntype = \"code\"\nrole = \"main\"\n", `a worker's role is "write" or "general"`},
		"empty argument":  {"[alias.w]\nharness = \"agent\"\ntype = \"code\"\nargs = [\"\"]\n", "empty argument"},
	} {
		set := typesRoom(t, map[string]string{"work/.rewake/aliases.toml": tc.file})
		if err := set.Err(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if _, err := set.Template("w", catalog); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: the malformed setting: %v, want %q", name, err, tc.want)
		}
	}
}
