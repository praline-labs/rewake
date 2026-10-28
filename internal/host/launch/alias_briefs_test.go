package launch

// A project says where its briefs go: [briefs] dir.

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAProjectNamesWhereItsBriefsGo(t *testing.T) {
	s := offRoom(t, "", "[briefs]\ndir = \".shift/briefs\"\n")
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	if got, want := s.BriefsDir("/work"), filepath.Join("/work", ".shift/briefs"); got != want {
		t.Errorf("a relative dir is main's directory's: %q, want %q", got, want)
	}
	if got := offRoom(t, "", "").BriefsDir("/work"); got != "" {
		t.Errorf("no setting, no folder: %q", got)
	}
}

func TestAUserFileMayNotNameTheBriefsFolder(t *testing.T) {
	s := offRoom(t, "[briefs]\ndir = \"/x\"\n", "")
	if err := s.Err(); err == nil || !strings.Contains(err.Error(), "[briefs] dir") {
		t.Errorf("the user file's [briefs] must be refused, got %v", err)
	}
}

// Both files may bar a skill from briefs; each bar names its file.
func TestBothFilesBarSkills(t *testing.T) {
	s := offRoom(t, "[skills]\nnever = [\"private\"]\n", "[skills]\nnever = [\"secret\"]\n")
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(s.SkillBarred("private"), "aliases.toml") || !strings.HasSuffix(s.SkillBarred("secret"), "aliases.toml") || s.SkillBarred("private") == s.SkillBarred("secret") {
		t.Errorf("barred by %q and %q", s.SkillBarred("private"), s.SkillBarred("secret"))
	}
	if got := s.SkillBarred("mods"); got != "" {
		t.Errorf("a skill no list names is barred by %q", got)
	}
}
