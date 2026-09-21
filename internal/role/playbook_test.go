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
			if len(part.Play.Limits) == 0 {
				t.Error("no limits: every role has at least the ones shared by all of them")
			}
			for _, step := range part.Play.Steps {
				if step.Do == "" || step.Why == "" {
					t.Errorf("a step with a missing half: %+v", step)
				}
			}
		})
	}
}

// Two roles may share how the work flows — write and general both take tasks
// and answer by finishing a turn — but not what they may do.
//
// The comparison is on the limits alone, and that is the whole point. Copying
// a neighboring role and editing the heading is exactly how a new role gets
// written: the heading names the role, so it is the first line anyone changes,
// and a key that included it would let the copy through carrying a limit that
// belongs to someone else — a prohibition about Git, say, in a role that has
// nothing to do with repositories.
func TestNoRoleCarriesAnotherRolesLimits(t *testing.T) {
	seen := map[string]string{}
	for _, part := range All() {
		key := strings.Join(part.Play.Limits, "\n")
		if other, ok := seen[key]; ok {
			t.Errorf("%s has exactly the limits of %s; a copied role carries a rule that is not about it", part.ID, other)
		}
		seen[key] = part.ID
	}
}
