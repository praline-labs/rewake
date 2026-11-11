package workmode

// Instructions is the complete behavioral section of one mode: the mode's own
// text, which replaces the previous section rather than adding to it. Nothing
// here grants Git authority or claims to enforce a read-only boundary; the
// role, the permissions and the project's own instructions stand (the owner's
// decision, November 3, 2026).
func Instructions(mode Mode, main bool) string {
	text := "Runtime work mode: " + mode.Name + ". Your role, permissions and the project's rules still apply. " + mode.Text
	if main {
		text += "\nYou remain the orchestrator in every mode: delegate implementation; do not write product code yourself."
	}
	return text
}
