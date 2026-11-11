package brief

import (
	"fmt"
	"strings"

	"github.com/praline-labs/rewake/internal/core/workmode"
)

// The briefing a session is launched with: who it is, what it does, and the
// handful of facts about delivery that it cannot discover by trying.
//
// The middle part — what it does — is not written here. It comes from the
// playbook, the same text `rewake guide` prints for this role, so the first
// thing a session reads and the thing it re-reads later cannot drift apart.
// The briefing is not wrapped: a harness shows it as the model reads it, and a
// rule broken across lines would read as two.

func roleText(c Context) string {
	play := c.Role.Play
	lines := []string{
		fmt.Sprintf("You are session %q in room %q, role %s: %s.", c.Name, c.Room, c.Role.ID, c.Reason),
		play.Heading,
		"",
	}
	for _, step := range play.Steps {
		lines = append(lines, "  "+step.Do+" — "+step.Why)
	}
	for _, section := range play.Sections {
		lines = append(lines, "", section.Title)
		for _, line := range section.Lines {
			lines = append(lines, "- "+line)
		}
	}
	if c.Mode.Name != "" {
		lines = append(lines, "", workmode.Instructions(c.Mode, c.Role.Spaces))
	}
	if len(c.Harness.Lines) > 0 {
		lines = append(lines, "", c.Harness.Title)
		for _, line := range c.Harness.Lines {
			lines = append(lines, "- "+line)
		}
	}
	if len(c.Migrations) > 0 {
		lines = append(lines, "", "Worker settings rewake skipped as obsolete; this session runs on the rest. Repair them when the work allows:")
		for _, migration := range c.Migrations {
			lines = append(lines, "- "+migration)
		}
	}
	lines = append(lines, "", "Run `rewake <words>` in the shell.")
	lines = append(lines, "Run rewake guide for the complete rules, including this list.")
	return strings.Join(lines, "\n")
}
