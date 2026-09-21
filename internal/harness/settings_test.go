package harness_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/harness"
)

// settingsWorld puts a case in a home and a working directory of its own, so
// the files it writes are the only ones read.
func settingsWorld(t *testing.T) (string, string) {
	t.Helper()
	home := t.TempDir()
	working := t.TempDir()
	t.Setenv("HOME", home)
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(working); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	return home, working
}

func writeUserSettings(t *testing.T, home, content string) string {
	t.Helper()
	dir := filepath.Join(home, ".config", "rewake")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "settings")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeProjectSettings(t *testing.T, working, content string) string {
	t.Helper()
	path := filepath.Join(working, ".rewake.env")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// The order is the point: the closer a choice is made to the launch, the more
// it means.
func TestSettingsOrderFromClosestToFurthest(t *testing.T) {
	home, working := settingsWorld(t)
	writeUserSettings(t, home, "REWAKE_CODEX_MODEL=from-the-user-file\n")
	writeProjectSettings(t, working, "REWAKE_CODEX_MODEL=from-the-project-file\n")

	settings := harness.LoadSettings()
	value, source, ok := settings.Lookup("REWAKE_CODEX_MODEL")
	if !ok || value != "from-the-project-file" {
		t.Fatalf("value=%q source=%q, want the project file to win over the user file", value, source)
	}
	if !strings.Contains(source, "project") {
		t.Errorf("source=%q does not name the project file", source)
	}

	t.Setenv("REWAKE_CODEX_MODEL", "from-the-environment")
	settings = harness.LoadSettings()
	value, source, _ = settings.Lookup("REWAKE_CODEX_MODEL")
	if value != "from-the-environment" {
		t.Errorf("value=%q, want a variable already set to beat both files", value)
	}
	if !strings.Contains(source, "environment") {
		t.Errorf("source=%q does not name the environment", source)
	}
}

func TestOnlyOurOwnKeysAreReadFromAFile(t *testing.T) {
	_, working := settingsWorld(t)
	writeProjectSettings(t, working, strings.Join([]string{
		"PATH=/somewhere/else",
		"LD_PRELOAD=/tmp/x.so",
		// Our own prefix, but not a launch default: a file must not be able to
		// point a session at another state directory or room.
		"REWAKE_DIR=/somewhere/else",
		"REWAKE_ROOM=somebody-elses",
		"REWAKE_CODEX_MODEL=ours",
	}, "\n"))

	settings := harness.LoadSettings()
	// Lookup reads the real environment first, and PATH is of course set
	// there — what must not happen is a *file* deciding any of these.
	for _, forbidden := range []string{"LD_PRELOAD", "PATH", "REWAKE_DIR", "REWAKE_ROOM"} {
		if _, source, ok := settings.Lookup(forbidden); ok && strings.Contains(source, "file") {
			t.Errorf("%s came from %s, which is not one of the settings a file may decide", forbidden, source)
		}
	}
	if value, _, ok := settings.Lookup("REWAKE_CODEX_MODEL"); !ok || value != "ours" {
		t.Errorf("our own setting was not read: %q", value)
	}
	// Silently: a file found in whatever directory somebody is in must not be
	// a way to change the harness's environment, and complaining about every
	// unrelated line would make that file unusable for anything else.
	for _, note := range settings.Notes {
		if strings.Contains(note, "LD_PRELOAD") || strings.Contains(note, "PATH") {
			t.Errorf("an unrelated name was reported: %q", note)
		}
	}
}

// "export KEY=VALUE" is the likeliest way to write one of these files, and
// dropping it would lose the setting without a word.
func TestTheExportFormIsAccepted(t *testing.T) {
	_, working := settingsWorld(t)
	writeProjectSettings(t, working, "export REWAKE_CODEX_MODEL=exported\n")

	settings := harness.LoadSettings()
	if value, _, ok := settings.Lookup("REWAKE_CODEX_MODEL"); !ok || value != "exported" {
		t.Errorf("value=%q ok=%v, want the exported form to be read", value, ok)
	}
}

// An empty variable is how a person turns a default off for one launch. The
// files must not answer behind it.
func TestAnEmptyVariableTurnsTheDefaultOff(t *testing.T) {
	home, working := settingsWorld(t)
	writeUserSettings(t, home, "REWAKE_CODEX_MODEL=from-the-user-file\n")
	writeProjectSettings(t, working, "REWAKE_CODEX_MODEL=from-the-project-file\n")
	t.Setenv("REWAKE_CODEX_MODEL", "")

	settings := harness.LoadSettings()
	if value, _, ok := settings.Lookup("REWAKE_CODEX_MODEL"); ok || value != "" {
		t.Errorf("value=%q ok=%v, want an empty variable to mean no default at all", value, ok)
	}
}

// A project file created under the usual umask is 0644, and warning about that
// every time would teach people to skip these notes.
func TestOrdinaryProjectPermissionsAreNotWarnedAbout(t *testing.T) {
	_, working := settingsWorld(t)
	path := writeProjectSettings(t, working, "REWAKE_CODEX_MODEL=fine\n")
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if notes := harness.LoadSettings().Notes; len(notes) != 0 {
		t.Errorf("an ordinary project file was complained about: %v", notes)
	}
}

func TestTheFormatIsTheSmallestThingThatWorks(t *testing.T) {
	_, working := settingsWorld(t)
	writeProjectSettings(t, working, strings.Join([]string{
		"# a comment",
		"",
		"  REWAKE_CODEX_MODEL = \"quoted value\"  ",
		"REWAKE_CODEX_EFFORT=low  # the cheapest one",
		"REWAKE_CLAUDE_MODEL=$NOT_EXPANDED",
		"REWAKE_CLAUDE_EFFORT=keeps#the-hash",
	}, "\n"))

	settings := harness.LoadSettings()
	for key, want := range map[string]string{
		"REWAKE_CODEX_MODEL":   "quoted value",
		"REWAKE_CODEX_EFFORT":  "low",
		"REWAKE_CLAUDE_MODEL":  "$NOT_EXPANDED",
		"REWAKE_CLAUDE_EFFORT": "keeps#the-hash",
	} {
		if value, _, ok := settings.Lookup(key); !ok || value != want {
			t.Errorf("%s=%q, want %q", key, value, want)
		}
	}
}

func TestAMalformedLineIsReportedAndSkipped(t *testing.T) {
	_, working := settingsWorld(t)
	writeProjectSettings(t, working, "this line has no equals sign\nREWAKE_CODEX_MODEL=still-read\n")

	settings := harness.LoadSettings()
	if value, _, ok := settings.Lookup("REWAKE_CODEX_MODEL"); !ok || value != "still-read" {
		t.Errorf("a bad line stopped the rest of the file: %q", value)
	}
	if len(settings.Notes) == 0 {
		t.Error("a line that made no sense was not reported")
	}
}

func TestAnUnreadableFileIsReportedNotIgnored(t *testing.T) {
	_, working := settingsWorld(t)
	path := writeProjectSettings(t, working, "REWAKE_CODEX_MODEL=unreachable\n")
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	settings := harness.LoadSettings()
	if _, _, ok := settings.Lookup("REWAKE_CODEX_MODEL"); ok {
		t.Skip("this user can read a 0000 file, so the case cannot be made here")
	}
	if len(settings.Notes) == 0 {
		t.Error("a file that could not be read was passed over in silence")
	}
}

// The user file is the one the convention is about, and the one that may end
// up holding more than a model name.
func TestWidePermissionsOnTheUserFileAreWarnedAbout(t *testing.T) {
	home, _ := settingsWorld(t)
	path := writeUserSettings(t, home, "REWAKE_CODEX_MODEL=readable-by-everyone\n")
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}

	settings := harness.LoadSettings()
	if value, _, ok := settings.Lookup("REWAKE_CODEX_MODEL"); !ok || value != "readable-by-everyone" {
		t.Error("the file was not read despite being readable")
	}
	warned := false
	for _, note := range settings.Notes {
		if strings.Contains(note, "readable by others") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("permissions wider than the convention were not mentioned: %v", settings.Notes)
	}
}

func TestNoFilesIsNotAProblem(t *testing.T) {
	settingsWorld(t)
	settings := harness.LoadSettings()
	if len(settings.Notes) != 0 {
		t.Errorf("absent files were reported as trouble: %v", settings.Notes)
	}
	if _, _, ok := settings.Lookup("REWAKE_CODEX_MODEL"); ok {
		t.Error("a value appeared from nowhere")
	}
}
