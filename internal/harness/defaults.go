package harness

import (
	"os"
	"strings"
)

// Launch defaults come from the environment, never from the source.
//
// Sessions multiply, and one that quietly picks an expensive model costs real
// money for work that did not need it. So rewake can supply a cheaper default
// — but it supplies it as flags for a single launch, and the names of models
// live in the environment of whoever set them, not here. A repository has no
// business naming somebody's provider or model.
//
// Three rules, and all three are about not surprising anyone:
//
//   - An explicit flag always wins. If the launch already names a model or an
//     effort, rewake adds nothing and says nothing: the person meant that.
//   - An unset variable means no default. Nothing is guessed, nothing is
//     "tried", and a session behaves exactly as it does today unless somebody
//     configured otherwise.
//   - Whatever is substituted is announced in a launch note. A silently
//     swapped model is the worst kind of surprise.

// Default is one value rewake may supply for a launch.
type Default struct {
	// Env is the variable the value is read from.
	Env string
	// What names the setting in the launch note ("model", "reasoning effort").
	What string
	// Present reports whether the launch already sets this, in any spelling.
	Present func(args []string) bool
	// Apply adds the value to the arguments in the form this harness takes.
	Apply func(args []string, value string) []string
}

// ApplyDefaults substitutes the defaults whose variable is set and whose
// setting the caller did not give, and returns the notes to print.
func ApplyDefaults(args []string, defaults []Default) ([]string, []string) {
	var notes []string
	for _, candidate := range defaults {
		value := strings.TrimSpace(os.Getenv(candidate.Env))
		if value == "" || candidate.Present(args) {
			continue
		}
		args = candidate.Apply(args, value)
		notes = append(notes, "using the "+candidate.What+" from "+candidate.Env+
			" for this launch; pass your own flag to override it")
	}
	return args, notes
}
