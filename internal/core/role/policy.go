package role

import "slices"

// For returns what a role does under the main-implementation policy a run was
// launched with (docs/roles.md#main-implementation). Main's playbook has two
// versions of the one rule the policy changes, and every other role is one, so
// a worker, or a guide asked outside a session, reads the delegating default.
//
// The catalogue's Main keeps that default and is never rewritten: a renderer
// that mutated it would give the next launch of any project the last project's
// policy.
func For(part Role, mainImplements bool) Playbook {
	if part.ID != Main.ID || !mainImplements {
		return part.Play
	}
	play := part.Play
	play.Sections = slices.Clone(part.Play.Sections)
	for index, section := range play.Sections {
		if section.Title != handingOutWork {
			continue
		}
		lines := slices.Clone(section.Lines)
		lines[0] = implementingRule
		play.Sections[index] = Section{Title: section.Title, Lines: lines}
		break
	}
	return play
}
