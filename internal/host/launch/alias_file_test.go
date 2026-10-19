package launch

// Reading the catalog, and refusing what it must not say.

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
)

type settingsHarness struct{ adapter.Harness }

func (settingsHarness) ID() string                     { return "agent" }
func (settingsHarness) Title() string                  { return "Test agent" }
func (settingsHarness) SingleUseFlags() []adapter.Flag { return agentLike }
func (settingsHarness) LaunchSettings(model, effort string) []string {
	return []string{"--model", model, "--effort", effort}
}

type plainHarness struct{ adapter.Harness }

func (plainHarness) ID() string                     { return "plain" }
func (plainHarness) Title() string                  { return "Plain" }
func (plainHarness) SingleUseFlags() []adapter.Flag { return nil }

var catalog = adapter.NewCatalog(settingsHarness{}, plainHarness{})

// readSet reads contents as the user's file, or as a project's.
func readSet(t *testing.T, contents string, user bool) *Set {
	t.Helper()
	path := filepath.Join(t.TempDir(), userFile)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	s := newSet()
	description := "the project alias file"
	if user {
		description = "the user alias file"
	}
	s.read(path, description, user)
	return s
}

func TestATemplateCarriesTheAliasWithItsSettings(t *testing.T) {
	s := readSet(t, `[alias.review]
harness = "agent"
role = "write"
command = "agent-worker"
description = "Checks a patch"
args = ["--model", "old", "--", "literal prompt"]
[alias.review.launch]
model = "chosen"
effort = "high"
context = "large"

[alias.plain]
harness = "plain"
`, true)
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	got, err := s.Template("review", catalog)
	if err != nil {
		t.Fatal(err)
	}
	// The structured settings replace the copy in args and stay before the
	// prompt's terminator; context has no flag here and is only shown.
	want := []string{"--model", "chosen", "--effort", "high", "--", "literal prompt"}
	if got.Harness.ID() != "agent" || got.Role.ID != "write" || got.Command != "agent-worker" || got.Description != "Checks a patch" ||
		!reflect.DeepEqual(got.Args, want) || !got.Applied.Model || !got.Applied.Effort || got.Applied.Context || got.Settings.Context != "large" {
		t.Fatalf("template: %+v", got)
	}
	plain, err := s.Template("plain", catalog)
	if err != nil || plain.Role.ID != "general" || plain.Command != "" || len(plain.Args) != 0 {
		t.Fatalf("an alias without a role is general: %+v, %v", plain, err)
	}
}

// A file written before October 19, 2026 is refused whole, naming each field
// that went and what took its place, so it is rewritten once.
func TestAnOldFileIsRefusedNamingWhatChanged(t *testing.T) {
	s := readSet(t, `[alias.lead]
harness = "agent"
main = true
rewake = ["--main"]

[alias.worker]
harness = "agent"
rewake = ["--write", "--name", "w"]
color = "blue"
[alias.worker.launch]
apply = false
`, true)
	err := s.Err()
	if err == nil || len(s.Names()) != 0 {
		t.Fatalf("an old file was read: %v, %v", err, s.Names())
	}
	for _, want := range []string{"alias.lead.main (line 3): main is no alias", "alias.lead.rewake (line 4)", "alias.worker.rewake (line 8)", `role = "write" or "general"`, "alias.worker.launch.apply (line 11)", "fields an alias does not have: alias.worker.color (line 9)", "rewake workers --help"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not say %q: %v", want, err)
		}
	}
}

func TestAnAliasThatMakesNoSenseIsRefused(t *testing.T) {
	for _, tc := range []struct {
		name, file string
		user       bool
		want       string
	}{
		{"no harness", "[alias.x]\nrole = \"write\"\n", true, "names no harness"},
		{"unknown harness", "[alias.x]\nharness = \"nothing\"\n", true, "which is not a harness"},
		{"main's role", "[alias.x]\nharness = \"agent\"\nrole = \"main\"\n", true, `a worker's role is "write" or "general"`},
		{"empty argument", "[alias.x]\nharness = \"agent\"\nargs = [\"\"]\n", true, "empty argument"},
		{"settings without flags", "[alias.x]\nharness = \"plain\"\n[alias.x.launch]\nmodel = \"m\"\n", true, "put the model and effort in args"},
		// A directory may not choose a worker's role or the program run with
		// the person's credentials.
		{"project role", "[alias.x]\nharness = \"agent\"\nrole = \"write\"\n", false, "sets role; it belongs in the user alias file"},
		{"project command", "[alias.x]\nharness = \"agent\"\ncommand = \"a\"\n", false, "sets command; it belongs in the user alias file"},
	} {
		_, err := readSet(t, tc.file, tc.user).Template("x", catalog)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want %q", tc.name, err, tc.want)
		}
	}
}

// Permissions are said out loud for the user file, which names a role and a
// program; a note, not a refusal.
func TestAUserFileOpenToOthersIsReported(t *testing.T) {
	path := filepath.Join(t.TempDir(), userFile)
	if err := os.WriteFile(path, []byte("[alias.x]\nharness = \"agent\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := newSet()
	s.read(path, "the user alias file", true)
	if len(s.Notes) == 0 || s.Err() != nil || len(s.Names()) != 1 {
		t.Fatalf("notes %v, err %v, names %v", s.Notes, s.Err(), s.Names())
	}
}

func TestABrokenFileNamesItsLine(t *testing.T) {
	s := readSet(t, "[alias.x]\nharness = [\n", false)
	if s.Err() == nil || !strings.Contains(s.Err().Error(), userFile+":2:") || len(s.Names()) != 0 {
		t.Fatalf("broken file: %v, %v", s.Err(), s.Names())
	}
}
