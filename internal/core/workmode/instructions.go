package workmode

// Instructions is the complete behavioral section of one mode: the mode's own
// text, which replaces the previous section rather than adding to it. Nothing
// here grants Git authority or claims to enforce a read-only boundary; the
// role, the permissions and the project's own instructions stand (the owner's
// decision, November 3, 2026).
//
// main and mainImplements are the run's role and the authorship policy frozen
// at its launch (docs/roles.md). A delegating main is told so in every mode; an
// opting-in main is told that implementation is permitted only where the task,
// the mode and the project's rules allow it, so a mode's own read-only text
// still holds. A worker gets the mode's text alone.
func Instructions(mode Mode, main, mainImplements bool) string {
	text := "Runtime work mode: " + mode.Name + ". Your role, permissions and the project's rules still apply. " + mode.Text
	switch {
	case main && mainImplements:
		text += "\nYou lead this room, and implementation is permitted only when the task, the mode and the project's rules allow it. This policy grants no additional permissions."
	case main:
		text += "\nYou remain the orchestrator in every mode: delegate implementation; do not write product code yourself."
	}
	return text
}
