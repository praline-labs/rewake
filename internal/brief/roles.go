package brief

import (
	"fmt"
	"strings"
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
	lines = append(lines, "", transport(c.Tool))
	lines = append(lines, "Run rewake guide for the complete rules, including this list.")
	return strings.Join(lines, "\n")
}

// The transport sentence (docs/mail-bridge-channel.md#the-briefing). The shell
// joins no operation by its words and the tool only in the same turn, so a
// call whose answer was lost is continued by its receipt, never repeated.
const (
	transportFirst = "Run `rewake <words>` through the `rewake` tool when you have it, otherwise in the shell."
	transportRest  = " A call the tool answered \"nothing ran\" may be made again in the shell. A call the harness refused for permission is not made another way; main is told. A call whose outcome is unknown is never repeated in the shell: continue it with `rewake retry <token>`, or with the same words through the tool in the same turn; with neither, leave it and say in your report which call it was."
)

func transport(tool bool) string {
	if tool {
		return transportFirst + transportRest
	}
	return transportFirst
}
