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
	lines = append(lines, "")
	lines = append(lines, play.Limits...)
	lines = append(lines, deliveryFacts...)
	lines = append(lines, "Run rewake guide for the complete rules, including this list.")
	return strings.Join(lines, "\n")
}

// deliveryFacts describe how mail arrives. They are the same for every role and
// they are not instructions: nothing here is a thing to do or to avoid, only
// what a session will see happen to it, which is why they stay in the briefing
// rather than joining the playbook. Anything that tells a session how to
// behave belongs in a playbook, where the guide will repeat it.
var deliveryFacts = []string{
	"Mail is announced as \"Rewake: <sender> <kind>, N new message(s)\", with a preview of the author's first line.",
	"Each notice has fixed members. Ready mail is submitted promptly: active work is steered, idle work is woken. Nothing waits for a peek or a finished turn, and old unread mail is not announced again.",
}
