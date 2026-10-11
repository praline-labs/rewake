package role

import (
	"strings"
	"testing"
)

// Every role has to carry its own instructions, and they have to be its own.
//
// A field rather than a lookup already makes the compiler ask for one, but the
// cheapest way to satisfy a field is to copy the neighbour's — and a session
// launched under a new role would then read a briefing about a different one,
// with nothing failing and nothing to notice. Same shape as a harness that
// returns an empty flag list, and answered the same way.
func TestEveryRoleCarriesItsOwnPlaybook(t *testing.T) {
	for _, part := range All() {
		t.Run(part.ID, func(t *testing.T) {
			if part.Play.Heading == "" {
				t.Error("no heading: the session would not be told what it is for")
			}
			if len(part.Play.Steps) == 0 {
				t.Error("no steps: the session would not be told what to do")
			}
			for _, step := range part.Play.Steps {
				if step.Do == "" || step.Why == "" {
					t.Errorf("a step with a missing half: %+v", step)
				}
			}
			sections := part.Play.Sections
			if len(sections) < 2 {
				t.Fatal("no sections of its own: every role has at least one besides the mail every role shares")
			}
			for _, section := range sections {
				if section.Title == "" || len(section.Lines) == 0 {
					t.Errorf("a section with no title or no rules: %+v", section)
				}
				for _, line := range section.Lines {
					if line == "" {
						t.Errorf("an empty rule in %s", section.Title)
					}
				}
			}
			// Mail is last and whole: it carries the rule every role shares
			// and what every session sees happen to its mail.
			last := sections[len(sections)-1]
			if last.Title != Mail.Title || strings.Join(last.Lines, "\n") != strings.Join(Mail.Lines, "\n") {
				t.Errorf("the last section is %q, not the shared mail section", last.Title)
			}
		})
	}
}

// Two roles may share how the work flows — write and general both take tasks
// and answer by finishing a turn — but not what they may do.
//
// The comparison is on the sections alone, and that is the whole point.
// Copying a neighboring role and editing the heading is exactly how a new role
// gets written: the heading names the role, so it is the first line anyone
// changes, and a key that included it would let the copy through carrying a
// rule that belongs to someone else — a prohibition about Git, say, in a role
// that has nothing to do with repositories.
func TestNoRoleCarriesAnotherRolesSections(t *testing.T) {
	seen := map[string]string{}
	for _, part := range All() {
		var key []string
		for _, section := range part.Play.Sections {
			key = append(key, section.Title)
			key = append(key, section.Lines...)
		}
		joined := strings.Join(key, "\n")
		if other, ok := seen[joined]; ok {
			t.Errorf("%s has exactly the sections of %s; a copied role carries a rule that is not about it", part.ID, other)
		}
		seen[joined] = part.ID
	}
}
