package brief

import (
	"fmt"
	"strings"

	"github.com/praline-labs/rewake/internal/core/role"
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
	play := role.For(c.Role, c.MainImplements)
	lines := []string{
		fmt.Sprintf("You are session %q in room %q, role %s: %s.", c.Name, c.Room, c.Role.ID, c.Reason),
		play.Heading,
	}
	if c.Role.Spaces {
		lines = append(lines, mainPolicyLine(c))
	}
	lines = append(lines, "")
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
		lines = append(lines, "", workmode.Instructions(c.Mode, c.Role.Spaces, c.MainImplements))
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

// mainPolicyLine says what the launch froze about main's authorship, and
// whether a project chose it or it is the default. It sits beside the role's
// heading, before the rules it qualifies: a person reading the briefing should
// not have to open the alias file to learn why main does or does not
// implement, and the policy never changes under a running run.
func mainPolicyLine(c Context) string {
	if c.MainImplements {
		return "This run's project policy lets main implement product changes itself, within the task, its work mode and the project's rules."
	}
	if c.MainPolicyExplicit {
		return "This run's project policy keeps main delegating implementation: the project's [main] implements = false."
	}
	return "This run's project policy keeps main delegating implementation: no project sets [main] implements, the default."
}
