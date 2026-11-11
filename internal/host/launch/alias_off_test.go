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

// [aliases] off is subtraction: a name no file holds any more is already off,
// so a project that keeps the removed name in its list still launches, with a
// notice saying the name can go.
func TestAnAbsentNameInOffChangesNothing(t *testing.T) {
	for name, tc := range map[string]struct{ user, project, want string }{
		"removed from the user's file":    {offUser, "[aliases]\noff = [\"long-gone-write\"]\n", "a,b"},
		"removed from the project's file": {"", "[aliases]\noff = [\"long-gone-write\"]\n[alias.p]\nharness = \"agent\"\n", "p"},
	} {
		s := offRoom(t, tc.user, tc.project)
		if err := s.Err(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if names := strings.Join(s.Names(), ","); names != tc.want {
			t.Fatalf("%s: names: %s, want %s", name, names, tc.want)
		}
		if _, err := s.Template(strings.Split(tc.want, ",")[0], catalog); err != nil {
			t.Errorf("%s: the remaining template: %v", name, err)
		}
		if len(s.Migrations) != 1 || !strings.Contains(s.Migrations[0], `"long-gone-write"`) || !strings.Contains(s.Migrations[0], "already off") {
			t.Errorf("%s: migrations: %v", name, s.Migrations)
		}
		if _, err := s.Template("long-gone-write", catalog); err == nil || !strings.Contains(err.Error(), `no alias "long-gone-write"`) {
			t.Errorf("%s: an absent name is no template off: %v", name, err)
		}
	}
}

// An absent name in the list does not make a present one less turned off.
func TestAPresentTemplateOffStaysOffBesideAnAbsentName(t *testing.T) {
	s := offRoom(t, offUser, "[aliases]\noff = [\"long-gone-write\", \"a\"]\n")
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	if names := strings.Join(s.Names(), ","); names != "b" {
		t.Fatalf("names: %s", names)
	}
	if _, err := s.Template("a", catalog); err == nil || !strings.Contains(err.Error(), "turned off in the project alias file") {
		t.Fatalf("refusal: %v", err)
	}
	if len(s.Migrations) != 1 || !strings.Contains(s.Migrations[0], "long-gone-write") {
		t.Fatalf("migrations: %v", s.Migrations)
	}
}

func TestAnOffListRefusesWhatItCannotMean(t *testing.T) {
	for name, tc := range map[string]struct{ user, project, want string }{
		"its own alias":      {offUser, "[aliases]\noff = [\"p\"]\n[alias.p]\nharness = \"agent\"\n", "defines [alias.p] and turns it off"},
		"in the user's file": {"[aliases]\noff = [\"a\"]\n" + offUser, "", "belongs in a project's .rewake/aliases.toml"},
	} {
		s := offRoom(t, tc.user, tc.project)
		if err := s.Err(); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want %q", name, err, tc.want)
		}
	}
}
