package harness

import "strings"

// Launch defaults come from the environment and from settings files, never
// from the source.
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
//     configured otherwise. Where a value may be configured, and in what
//     order, is in settings.go.
//   - Whatever is substituted is announced in a launch note. A silently
//     swapped model is the worst kind of surprise.

// Default is one value rewake may supply for a launch.
type Default struct {
	// Env is the name the value is read under, in the environment or in a
	// settings file.
	Env string
	// What names the setting in the launch note ("model", "reasoning effort").
	What string
	// Present reports whether the launch already sets this, in any spelling.
	Present func(args []string) bool
	// Apply adds the value to the arguments in the form this harness takes.
	Apply func(args []string, value string) []string
}

// ApplyDefaults substitutes the defaults that are configured and that the
// caller did not give, and returns the notes to print.
func ApplyDefaults(args []string, defaults []Default) ([]string, []string) {
	settings := LoadSettings()
	// Trouble with the files is said first: somebody whose settings did not
	// load needs to know that before wondering about the model.
	notes := settings.Notes
	for _, candidate := range defaults {
		value, source, ok := settings.Lookup(candidate.Env)
		value = strings.TrimSpace(value)
		if !ok || value == "" || candidate.Present(args) {
			continue
		}
		args = candidate.Apply(args, value)
		// The note names the source as well as the variable: somebody seeing
		// an unexpected model should know in one step where to change it.
		notes = append(notes, "using the "+candidate.What+" from "+candidate.Env+
			" in "+source+" for this launch; pass your own flag to override it")
	}
	return args, notes
}
