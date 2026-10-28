package taskbrief

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The skills a named skill links come along (docs/briefs.md#skills): a
// chain, a cycle, a barred link, a link outside the catalog, the bound.

func TestLinksOutOfASkillAreFound(t *testing.T) {
	for _, c := range []struct {
		text, dir string
		names     []string
	}{
		{"See [design](../mp-codebase-design/SKILL.md).", ".", []string{"mp-codebase-design"}},
		{"read ../mp-grilling/SKILL.md and ../mp-grilling/more.md, then ../mp-domain-modeling.", ".", []string{"mp-grilling", "mp-domain-modeling"}},
		{"[up](../../other/SKILL.md)", "references", []string{"other"}},
		{"[inside](../SKILL.md) and references/../x.md", "references", nil},
		{"the mods skill, mp-tdd and /home/x/../y, https://h/../z", ".", nil},
		{"../../outside/x and ../Upper/x", ".", nil},
	} {
		if got := linksOut(c.text, c.dir); !slices.Equal(got, c.names) {
			t.Errorf("%q in %s: %q, want %q", c.text, c.dir, got, c.names)
		}
	}
}

// skillLinking makes a skill named name in folder whose SKILL.md says body.
func skillLinking(t *testing.T, folder, name, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(folder, name), 0o700); err != nil {
		t.Fatal(err)
	}
	text := "---\ndescription: the " + name + " skill\n---\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(folder, name, skillFile), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

// barring is catalogOf(folder) with the names barred.
func barring(folder string, barred ...string) Catalog {
	return func(name string) (string, error) {
		if slices.Contains(barred, name) {
			return "", SkillBarred(fmt.Errorf("%s is barred", name))
		}
		return catalogOf(folder)(name)
	}
}

func TestLinkedSkillsComeAlong(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "skills")
	// tdd → design → tdd is a cycle; design → grilling a chain; tdd links a
	// barred skill and a folder beside it that is no skill.
	skillLinking(t, folder, "tdd", "See ../design/SKILL.md, ../keychain/SKILL.md and ../cypress/support/e2e.js.")
	skillLinking(t, folder, "design", "Back to [tdd](../tdd/SKILL.md); grill with ../grilling/GRILL.md.")
	skillLinking(t, folder, "grilling", "Plain.")
	skillLinking(t, folder, "keychain", "Secret.")
	if err := os.WriteFile(filepath.Join(folder, "grilling", "GRILL.md"), []byte("ask ../design/SKILL.md\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	brief, err := New(Home{Room: room(t)}, main, "Skills: tdd\n", "", barring(folder, "keychain"))
	if err != nil {
		t.Fatal(err)
	}
	copies, err := brief.Skills()
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range copies {
		got = append(got, c.Name+"<"+c.BroughtBy+">"+strings.Join(c.Barred, ","))
	}
	if want := []string{"tdd<>keychain", "design<tdd>", "grilling<design>"}; !slices.Equal(got, want) {
		t.Fatalf("copies %q, want %q", got, want)
	}
	// The link in the copy leads to the linked skill's copy.
	if body := read(t, filepath.Join(filepath.Dir(copies[0].Path), "..", "design", skillFile)); !strings.Contains(body, "Back to") {
		t.Fatalf("design's copy holds %q", body)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(copies[0].Path), "..", "keychain")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the barred skill was copied: %v", err)
	}
	// A linked skill the text also names stays named, in the line's order.
	both, err := New(Home{Room: room(t)}, main, "Skills: design, tdd\n", "", barring(folder, "keychain"))
	if err != nil {
		t.Fatal(err)
	}
	copies, _ = both.Skills()
	if len(copies) != 3 || copies[1].Name != "tdd" || copies[1].BroughtBy != "" || copies[2].BroughtBy != "design" {
		t.Fatalf("copies %+v", copies)
	}
}

// Past the bound of skills given, the brief is refused naming the chain
// that reaches past it, and nothing is written; a linked skill whose lookup
// fails for another reason than missing or barred refuses too.
func TestLinkedSkillsKeepToTheBound(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "skills")
	for i := range MaxGivenSkills {
		skillLinking(t, folder, fmt.Sprintf("s%02d", i), fmt.Sprintf("next ../s%02d/SKILL.md", i+1))
	}
	skillLinking(t, folder, fmt.Sprintf("s%02d", MaxGivenSkills), "last")
	dir := room(t)
	_, err := New(Home{Room: dir}, main, "Skills: s00\n", "", catalogOf(folder))
	if !errors.Is(err, ErrSkill) || !strings.Contains(err.Error(), "s00 → s01 → s02") || !strings.Contains(err.Error(), "s15 → s16 links one past the bound") {
		t.Fatalf("past the bound: %v", err)
	}
	if brief, err := New(Home{Room: dir}, main, "Skills: s01\n", "", catalogOf(folder)); err != nil || brief.ID != 1 {
		t.Fatalf("at the bound: %v; a refused brief took no id", err)
	}
	broken := func(name string) (string, error) {
		if name == "s02" {
			return "", errors.New("the alias files do not read")
		}
		return catalogOf(folder)(name)
	}
	if _, err := New(Home{Room: dir}, main, "Skills: s01\n", "", broken); !errors.Is(err, ErrSkill) || !strings.Contains(err.Error(), "s02, which s01 links") {
		t.Fatalf("a lookup that fails: %v", err)
	}
}
