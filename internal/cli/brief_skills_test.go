package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// skillIn writes a skill named name into folder.
func skillIn(t *testing.T, folder, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(folder, name), 0o700); err != nil {
		t.Fatal(err)
	}
	text := "---\nname: " + name + "\ndescription: the " + name + " skill\n---\nbody of " + name + "\n"
	if err := os.WriteFile(filepath.Join(folder, name, "SKILL.md"), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Main names a skill its harness reads; the brief carries the copy, its
// letter and show give the path, and a skill barred, missing or named badly
// refuses the brief, saying why (docs/briefs.md#skills).
func TestABriefGivesTheSkillsMainNames(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	_, _, project := projectMain(t, "[skills]\nnever = [\"secret\"]\n")
	skillIn(t, filepath.Join(home, ".agent", "skills"), "probe")
	skillIn(t, filepath.Join(project, ".agent", "skills"), "secret")
	for _, c := range []struct {
		text, says string
		code       int
	}{
		{"Skills: secret\n", "[skills] never in " + filepath.Join(project, ".rewake", "aliases.toml"), ExitFailed},
		{"Skills: absent\n", "none of " + filepath.Join(home, ".agent", "skills"), ExitFailed},
		{"Skills: plugin:skill\n", "a plugin's skill", ExitUsage},
	} {
		if code, _, errOut := run("brief", "new", c.text); code != c.code || !strings.Contains(errOut, c.says) {
			t.Errorf("%q: %d %q, want %q", c.text, code, errOut, c.says)
		}
	}
	if code, out, errOut := run("brief", "new", "# Probe\nSkills: probe\n- [ ] read it\n"); code != ExitOK || !strings.Contains(out, "brief 1 made") {
		t.Fatalf("new: %d %q %q", code, out, errOut)
	}
	_, raw, _ := run("brief", "show", "1", "--json")
	var shown struct {
		Skills     []string `json:"skills"`
		SkillFiles []struct {
			Name, Description, Path string
		} `json:"skillFiles"`
	}
	if err := json.Unmarshal([]byte(raw), &shown); err != nil || len(shown.Skills) != 1 || len(shown.SkillFiles) != 1 || shown.SkillFiles[0].Description != "the probe skill" {
		t.Fatalf("show --json = %s (%v)", raw, err)
	}
	if body, err := os.ReadFile(shown.SkillFiles[0].Path); err != nil || !strings.Contains(string(body), "body of probe") {
		t.Fatalf("the copy at %s: %q, %v", shown.SkillFiles[0].Path, body, err)
	}
	_, out, _ := run("brief", "show", "1")
	if !strings.Contains(out, "Rewake: skills main gave with this brief") || !strings.Contains(out, "- probe — the probe skill — "+shown.SkillFiles[0].Path) {
		t.Fatalf("show = %q", out)
	}
	run("brief", "close", "1")
	if _, err := os.Stat(shown.SkillFiles[0].Path); !os.IsNotExist(err) {
		t.Fatalf("the copy outlived the close: %v", err)
	}
}

// A skill a named one links comes along, listed after the named and marked
// with the skill that links it; a link to a barred skill is named under the
// skill holding it, and a link to a folder that is no skill is passed over.
func TestABriefBringsTheSkillsItsSkillsLink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	projectMain(t, "[skills]\nnever = [\"secret\"]\n")
	user := filepath.Join(home, ".agent", "skills")
	skillIn(t, user, "probe")
	skillIn(t, user, "secret")
	skillIn(t, user, "linker")
	text := "---\ndescription: the linker skill\n---\nRead ../probe/SKILL.md, ../secret/SKILL.md and ../cypress/e2e.js.\n"
	if err := os.WriteFile(filepath.Join(user, "linker", "SKILL.md"), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, errOut := run("brief", "new", "Skills: linker\n- [ ] one\n"); code != ExitOK {
		t.Fatalf("new: %d %q", code, errOut)
	}
	_, raw, _ := run("brief", "show", "1", "--json")
	var shown struct {
		SkillFiles []struct {
			Name, Path, BroughtBy string
			Barred                []string
		} `json:"skillFiles"`
	}
	if err := json.Unmarshal([]byte(raw), &shown); err != nil || len(shown.SkillFiles) != 2 {
		t.Fatalf("show --json = %s (%v)", raw, err)
	}
	linker, probe := shown.SkillFiles[0], shown.SkillFiles[1]
	if linker.Name != "linker" || len(linker.Barred) != 1 || linker.Barred[0] != "secret" || probe.Name != "probe" || probe.BroughtBy != "linker" {
		t.Fatalf("skillFiles = %+v", shown.SkillFiles)
	}
	_, out, _ := run("brief", "show", "1")
	for _, want := range []string{
		"- linker — the linker skill — " + linker.Path + "\n  - its link to secret leads nowhere: [skills] never bars secret from briefs, so it is not given\n",
		"- probe — the probe skill — " + probe.Path + " — linked from linker\n",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("show = %q, want %q", out, want)
		}
	}
}
