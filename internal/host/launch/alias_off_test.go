package launch

// A project turns templates of the merged catalog off.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func offRoom(t *testing.T, user, project string) *Set {
	t.Helper()
	home, work := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(work)
	for path, text := range map[string]string{
		filepath.Join(home, ".config", userDir, userFile): user,
		filepath.Join(work, projectDir, userFile):         project,
	} {
		if text == "" {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return Load()
}

const offUser = "[alias.a]\nharness = \"agent\"\n[alias.b]\nharness = \"agent\"\n"

func TestAProjectTurnsAUsersTemplateOff(t *testing.T) {
	s := offRoom(t, offUser, "[aliases]\noff = [\"a\"]\n")
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	if names := strings.Join(s.Names(), ","); names != "b" {
		t.Fatalf("names: %s", names)
	}
	_, err := s.Template("a", catalog)
	if err == nil || !strings.Contains(err.Error(), "turned off in the project alias file") || !strings.Contains(err.Error(), "available: b") {
		t.Fatalf("refusal: %v", err)
	}
}

func TestAnOffListRefusesWhatItCannotMean(t *testing.T) {
	for name, tc := range map[string]struct{ user, project, want string }{
		"a typo":             {offUser, "[aliases]\noff = [\"c\"]\n", `turns off "c" in [aliases] off, which is no alias of the catalog; available: a, b`},
		"its own alias":      {offUser, "[aliases]\noff = [\"p\"]\n[alias.p]\nharness = \"agent\"\n", "defines [alias.p] and turns it off"},
		"in the user's file": {"[aliases]\noff = [\"a\"]\n" + offUser, "", "belongs in a project's .rewake/aliases.toml"},
	} {
		s := offRoom(t, tc.user, tc.project)
		if err := s.Err(); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want %q", name, err, tc.want)
		}
	}
}
