package launch

// The project's [main] implements: read once, default false, never from the
// user file (docs/roles.md#main-implementation).

import (
	"strings"
	"testing"
)

func TestMainPolicyIsAbsentFalseOrTrue(t *testing.T) {
	for _, tc := range []struct {
		name, file string
		want       MainPolicy
	}{
		{"absent", "[alias.x]\nharness = \"agent\"\n", MainPolicy{}},
		{"false", "[main]\nimplements = false\n", MainPolicy{Implements: false, Explicit: true}},
		{"true", "[main]\nimplements = true\n", MainPolicy{Implements: true, Explicit: true}},
		{"empty section", "[main]\n", MainPolicy{}},
	} {
		s := readSet(t, tc.file, false)
		if err := s.Err(); err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if got := s.Main(); got != tc.want {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

// The user file may not choose main's authorship: that is one project's
// workflow, not a default for every room the person starts.
func TestMainPolicyBelongsToTheProjectFile(t *testing.T) {
	s := readSet(t, "[main]\nimplements = true\n", true)
	if s.Err() == nil || !strings.Contains(s.Err().Error(), "has [main]; it belongs in a project's .rewake/aliases.toml") {
		t.Fatalf("a user [main]: %v", s.Err())
	}
}

func TestMainPolicyRefusesWhatItCannotBe(t *testing.T) {
	for _, tc := range []struct {
		name, file, want string
	}{
		{"a string", "[main]\nimplements = \"yes\"\n", userFile},
		{"a number", "[main]\nimplements = 1\n", userFile},
		{"an unknown key", "[main]\nimplements = true\nother = true\n", "main.other (line 3)"},
	} {
		s := readSet(t, tc.file, false)
		err := s.Err()
		if err == nil {
			t.Errorf("%s: read without a refusal", tc.name)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v does not name %q", tc.name, err, tc.want)
		}
	}
	// The unknown key is refused with what [main] takes, not as an alias field.
	s := readSet(t, "[main]\nimplements = true\nother = true\n", false)
	if err := s.Err(); err == nil || !strings.Contains(err.Error(), "[main] does not have, which takes implements = true or false") {
		t.Fatalf("an unknown [main] key: %v", err)
	}
}

// The whole-template and user-only rules are unchanged by the new section: a
// project still replaces a user template whole, and still may not set its role
// or command.
func TestMainPolicyLeavesTemplateRulesAlone(t *testing.T) {
	s := readSet(t, "[main]\nimplements = true\n[alias.x]\nharness = \"agent\"\n", false)
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Template("x", catalog); err != nil {
		t.Fatal(err)
	}
	project := readSet(t, "[main]\nimplements = true\n[alias.x]\nharness = \"agent\"\nrole = \"write\"\n", false)
	if err := project.Err(); err != nil {
		t.Fatal(err)
	}
	if _, err := project.Template("x", catalog); err == nil || !strings.Contains(err.Error(), "sets role; it belongs in the user alias file") {
		t.Fatalf("a project role: %v", err)
	}
}
