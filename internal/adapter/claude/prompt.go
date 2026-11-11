package claude

import (
	"fmt"
	"strings"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/workmode"
)

// maxArgument is the longest one argument of a launch may be: Linux holds each
// to 128 KiB with its terminating zero (MAX_ARG_STRLEN), and exec fails past
// it with "argument list too long", the worker never started.
const maxArgument = 128<<10 - 1

// promptPart is one text of the --append-system-prompt rewake composes, named
// for a refusal that says which is big.
type promptPart struct {
	what string
	text string
}

// composePrompt joins the parts into the one --append-system-prompt Claude
// Code keeps, refusing a prompt one argument cannot carry rather than let
// exec fail or cut it: each part may be within its own bound and the sum
// past the argument's (a type added to a large CLAUDE.md, found in review,
// October 21, 2026).
func composePrompt(parts []promptPart, request adapter.LaunchRequest) (string, error) {
	texts := make([]string, 0, len(parts))
	sizes := make([]string, 0, len(parts))
	for _, part := range parts {
		texts = append(texts, part.text)
		size := fmt.Sprintf("%s %d bytes", part.what, len(part.text))
		if part.what == "rewake's briefing" && request.Mode.Name != "" {
			size += fmt.Sprintf(" (the work mode %q's text %d of it)", request.Mode.Name, len(request.Mode.Text))
		}
		sizes = append(sizes, size)
	}
	prompt := strings.Join(texts, "\n\n")
	if len(prompt) > maxArgument {
		hint := "shorten one of them"
		if request.Mode.Source != workmode.BuiltIn && request.Mode.Name != "" {
			hint += ", or shorten the mode's own text at " + request.Mode.Path
		}
		return "", fmt.Errorf("the system prompt rewake would pass in one --append-system-prompt is %d bytes, %d over the %d one argument of a launch may hold: %s; %s",
			len(prompt), len(prompt)-maxArgument, maxArgument, strings.Join(sizes, ", "), hint)
	}
	return prompt, nil
}
