package taskbrief

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// A brief's skills (docs/briefs.md#skills): the line that names them, the
// copy per edition and its bound, an edit cut short, and the close.

func TestTheSkillsLineIsScanned(t *testing.T) {
	for _, c := range []struct {
		text  string
		names []string
		bad   bool
	}{
		{"# Plan\n- [ ] one\n", nil, false},
		{"# Plan\n\nSkills: mods, mp-tdd\n", []string{"mods", "mp-tdd"}, false},
		{"Skills:   one  \r\n", []string{"one"}, false},
		{"```\nSkills: inside\n```\nSkills: outside\n", []string{"outside"}, false},
		{"a line saying `Skills: x` is prose\n", nil, false},
		{"Skills: a\nSkills: b\n", nil, true},
		{"Skills: Mods\n", nil, true},
		{"Skills: ../x\n", nil, true},
		{"Skills: plugin:skill\n", nil, true},
		{"Skills: a, a\n", nil, true},
		{"Skills:\n", nil, true},
		{"Skills: a, b, c, d, e, f, g, h, i\n", nil, true},
	} {
		names, err := DeclaredSkills(c.text)
		if c.bad != errors.Is(err, ErrBadText) || !slices.Equal(names, c.names) {
			t.Errorf("%q: %q, %v", c.text, names, err)
		}
	}
}

// skillAt makes a skill named name in a catalog folder, with content in
// its SKILL.md, a file beside it and a link to a file outside it.
func skillAt(t *testing.T, folder, name, content string) string {
	t.Helper()
	resolved := filepath.Join(t.TempDir(), "skill-"+name)
	for path, text := range map[string]string{
		"SKILL.md":           "---\nname: " + name + "\ndescription: >\n  " + content + "\n  over two lines\n---\n\n" + content + "\n",
		"references/more.md": "more of " + content + "\n",
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(resolved, path)), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(resolved, path), []byte(text), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	outside := filepath.Join(t.TempDir(), "shared.md")
	if err := os.WriteFile(outside, []byte("shared\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(resolved, "shared.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	// The catalog holds a link to the skill, as an installer makes it.
	link := filepath.Join(folder, name)
	_ = os.Remove(link)
	if err := os.Symlink(resolved, link); err != nil {
		t.Fatal(err)
	}
	return resolved
}

func catalogOf(folder string) Catalog {
	return func(name string) (string, error) {
		at := filepath.Join(folder, name)
		if _, err := os.Stat(filepath.Join(at, "SKILL.md")); err != nil {
			return "", fmt.Errorf("no skill %s", name)
		}
		return at, nil
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestANewBriefCopiesItsSkills(t *testing.T) {
	for _, where := range []string{"room", "folder"} {
		t.Run(where, func(t *testing.T) {
			home := Home{Room: room(t)}
			if where == "folder" {
				home, _ = folderHome(t)
			}
			folder := filepath.Join(t.TempDir(), "skills")
			skillAt(t, folder, "probe", "the probe skill")
			brief, err := New(home, main, "# Plan\nSkills: probe\n- [ ] one\n", "", catalogOf(folder))
			if err != nil {
				t.Fatal(err)
			}
			copies, err := brief.Skills()
			if err != nil || len(copies) != 1 || copies[0].Name != "probe" || copies[0].Description != "the probe skill over two lines" {
				t.Fatalf("copies %+v, %v", copies, err)
			}
			if got := read(t, copies[0].Path); !strings.Contains(got, "\nthe probe skill\n") || !strings.Contains(copies[0].Path, filepath.Join("skills", "1", "probe")) {
				t.Fatalf("%s holds %q", copies[0].Path, got)
			}
			dir := filepath.Dir(copies[0].Path)
			if read(t, filepath.Join(dir, "references", "more.md")) != "more of the probe skill\n" || read(t, filepath.Join(dir, "shared.md")) != "shared\n" {
				t.Fatal("the copy is not whole")
			}
			info, err := os.Lstat(filepath.Join(dir, "shared.md"))
			if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 != 0 {
				t.Fatalf("a copied file is %v, %v: a regular file with no executable bit", info.Mode(), err)
			}
			if names := brief.SkillNames(); !slices.Equal(names, []string{"probe"}) {
				t.Fatalf("names %q", names)
			}
		})
	}
}

// A skill that cannot be given refuses the brief before anything is
// written: missing, holding a link to a folder, past the bound.
func TestARefusedSkillWritesNothing(t *testing.T) {
	dir := room(t)
	folder := filepath.Join(t.TempDir(), "skills")
	linked := skillAt(t, folder, "linked", "linked")
	if err := os.Symlink(t.TempDir(), filepath.Join(linked, "folder-link")); err != nil {
		t.Fatal(err)
	}
	large := skillAt(t, folder, "large", "large")
	if err := os.Truncate(filepath.Join(large, "references", "more.md"), maxSkillBytes+1); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"absent", "linked", "large"} {
		if _, err := New(Home{Room: dir}, main, "Skills: "+name+"\n", "", catalogOf(folder)); !errors.Is(err, ErrSkill) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := New(Home{Room: dir}, main, "Skills: absent\n", "", nil); !errors.Is(err, ErrSkill) {
		t.Errorf("no catalog: %v", err)
	}
	if entries, err := os.ReadDir(state.BriefsPath(dir)); err == nil {
		for _, entry := range entries {
			if _, ok := ParseID(entry.Name()); ok || strings.HasPrefix(entry.Name(), tmpPrefix) {
				t.Fatalf("a refused brief left %s", entry.Name())
			}
		}
	}
	if brief := made(t, dir, "- [ ] one\n"); brief.ID != 1 {
		t.Fatalf("a refused brief took an id: the next is %d", brief.ID)
	}
}

// An edit copies the skills again as its own edition's snapshot. One cut
// short before its entry leaves the previous edition with its snapshot
// whole, though the skill changed since; the next edit drops the leftover
// and, once it commits, the older snapshot.
func TestAnEditCutShortKeepsTheEditionsSnapshot(t *testing.T) {
	dir := room(t)
	home := Home{Room: dir}
	folder := filepath.Join(t.TempDir(), "skills")
	resolved := skillAt(t, folder, "probe", "first")
	brief, err := New(home, main, "Skills: probe\n- [ ] one\n", "", catalogOf(folder))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(resolved, "SKILL.md"), []byte("---\ndescription: second\n---\nsecond\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := openPath(dir, brief.ID)
	// The text of edition 2 cannot be written: the edit fails after its
	// snapshot, before its entry.
	if err := os.MkdirAll(textPath(path, 2), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := Edit(home, brief.ID, main, brief.Revision, "Skills: probe\n- [ ] two\n", catalogOf(folder)); err == nil {
		t.Fatal("the edit passed a text it could not write")
	}
	kept := loaded(t, dir, brief.ID)
	copies, err := kept.Skills()
	if err != nil || kept.Edition != 1 || len(copies) != 1 || !strings.Contains(read(t, copies[0].Path), "first") {
		t.Fatalf("edition %d, copies %+v, %v", kept.Edition, copies, err)
	}
	if err := os.Remove(textPath(path, 2)); err != nil {
		t.Fatal(err)
	}
	edited, err := Edit(home, brief.ID, main, kept.Revision, "Skills: probe\n- [ ] two\n", catalogOf(folder))
	if err != nil {
		t.Fatal(err)
	}
	copies, err = edited.Skills()
	if err != nil || edited.Edition != 2 || len(copies) != 1 || copies[0].Description != "second" || !strings.Contains(read(t, copies[0].Path), "second") {
		t.Fatalf("edition %d, copies %+v, %v", edited.Edition, copies, err)
	}
	if _, err := os.Stat(snapshotPath(path, 1)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the older snapshot stayed: %v", err)
	}
	// An edit that names no skill leaves none.
	plain, err := Edit(home, brief.ID, main, edited.Revision, "- [ ] three\n", nil)
	if copies, err2 := plain.Skills(); err != nil || err2 != nil || len(copies) != 0 {
		t.Fatalf("copies %+v, %v, %v", copies, err, err2)
	}
	if entries, _ := os.ReadDir(filepath.Join(path, skillsDir)); len(entries) != 0 {
		t.Fatalf("snapshots left: %v", entries)
	}
}

func TestACloseTakesTheCopies(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "skills")
	skillAt(t, folder, "probe", "probe")
	for _, where := range []string{"room", "folder"} {
		t.Run(where, func(t *testing.T) {
			home := Home{Room: room(t)}
			if where == "folder" {
				home, _ = folderHome(t)
			}
			brief, err := New(home, main, "Skills: probe\n", "", catalogOf(folder))
			if err != nil {
				t.Fatal(err)
			}
			copies, _ := brief.Skills()
			if already, err := Close(home, brief.ID); err != nil || already {
				t.Fatalf("close: %v, %v", already, err)
			}
			if _, err := os.Stat(copies[0].Path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("the copy stayed: %v", err)
			}
			closed, err := Load(home, brief.ID)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(closed.path, skillsDir)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("skills/ stayed: %v", err)
			}
			if copies, err := closed.Skills(); err != nil || len(copies) != 0 || !slices.Equal(closed.SkillNames(), []string{"probe"}) {
				t.Fatalf("closed: %+v, %v, %q", copies, err, closed.SkillNames())
			}
		})
	}
}
