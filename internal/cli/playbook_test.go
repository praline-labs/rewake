package cli

import (
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/brief"
	"github.com/praline-labs/rewake/internal/role"
)

// Every step and every limit of every role has to appear in both places a
// session reads: the briefing it is launched with, and the guide it comes back
// to. One source makes the two agree in principle; this is what keeps a
// renderer from quietly dropping half of it — the review found exactly that,
// a guide that printed the steps and not the limits, with nothing failing.
//
// It lives in the cli package because it is the only one that can see both
// renderings. Checking them separately is how the gap stayed open: each test
// compared a rendering against the same slice it rendered from.
func TestBothRenderingsCarryTheWholePlaybook(t *testing.T) {
	for _, part := range role.All() {
		t.Run(part.ID, func(t *testing.T) {
			play := role.Of(part.ID).Play
			briefing := brief.Intro(brief.Context{Name: "api", Room: "work", Role: part, Reason: "selected explicitly"})
			guide := formatGuide(&play)
			for where, text := range map[string]string{"the briefing": briefing, "the guide": guide} {
				if !strings.Contains(text, play.Heading) {
					t.Errorf("%s does not say what this role is for", where)
				}
				for _, step := range play.Steps {
					if !strings.Contains(text, step.Do) {
						t.Errorf("%s is missing the step %q", where, step.Do)
					}
				}
				for _, limit := range play.Limits {
					// The guide wraps its lines, so the comparison is on words
					// rather than on the line as written.
					if !containsWrapped(text, limit) {
						t.Errorf("%s is missing the limit %q", where, limit)
					}
				}
			}
		})
	}
}

// containsWrapped reports whether a text carries a sentence, ignoring where
// the renderer chose to break its lines.
func containsWrapped(text, sentence string) bool {
	return strings.Contains(strings.Join(strings.Fields(text), " "), strings.Join(strings.Fields(sentence), " "))
}
