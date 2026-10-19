package launch

// Where an alias's arguments meet those typed beside it: a typed flag the
// harness takes once replaces the alias's copy, everything else is appended.
//
// Only the flags the harness takes at most once, which it names itself.
// Ordering alone would not do: a harness may refuse to parse a repeated
// --model rather than take the last one. Everything else is appended, and that
// is the safe default rather than a gap: a repeated --add-dir is how a second
// directory is added, and replacing it would take away something nobody asked
// to remove.

import (
	"slices"
	"strings"

	"github.com/praline-labs/rewake/internal/adapter"
)

// MergeArguments puts typed arguments after an alias's, dropping the alias's
// copy of each single-use flag typed. A terminator in the alias still makes
// later words prompt input.
func MergeArguments(base, typed []string, flags []adapter.Flag) []string {
	return append(slices.Clone(replaced(base, typed, flags)), typed...)
}

// replaced drops each argument of the alias whose parameter was named on the
// line, along with the value belonging to it.
//
// The terminator is read on the command being built, not on each list on its
// own. "--" ends the flags of everything that follows it there: a terminator
// the person typed ends their own arguments, and one the alias carries ends
// both its own tail and everything the line appends behind it. Either way a
// word of a prompt must never replace a setting — which is what happened when
// each half was checked separately.
func replaced(arguments, typed []string, flags []adapter.Flag) []string {
	visible := adapter.BeforeTerminator(arguments)
	if len(visible) != len(arguments) {
		// The alias itself ends the flags. Everything the line adds lands
		// behind that terminator in the command being built, so all of it is
		// input for the harness — and a word of a prompt must not replace a
		// setting. Nothing is dropped here at all.
		return arguments
	}
	named := map[string]bool{}
	for _, name := range parameters(adapter.BeforeTerminator(typed), flags) {
		named[name] = true
	}
	kept := make([]string, 0, len(arguments))
	for i := 0; i < len(visible); i++ {
		name, width := parameterAt(visible, i, flags)
		if name != "" && named[name] {
			i += width - 1
			continue
		}
		kept = append(kept, visible[i:i+width]...)
		i += width - 1
	}
	return kept
}

// parameters names every parameter these arguments set.
func parameters(arguments []string, flags []adapter.Flag) []string {
	var names []string
	for i := 0; i < len(arguments); i++ {
		name, width := parameterAt(arguments, i, flags)
		if name != "" {
			names = append(names, name)
		}
		i += width - 1
	}
	return names
}

// parameterAt answers which parameter the argument at index sets, and how many
// arguments it occupies. A flag that is not in the list answers "" — it may be
// repeated for all we know, so it is left alone.
//
// The shapes of a flag are read by adapter.MatchFlag, the one place that knows
// them: --flag value, --flag=value, -f value, -f=value and -fvalue. Each of
// those has cost us a defect somewhere else already.
func parameterAt(arguments []string, index int, flags []adapter.Flag) (string, int) {
	arg := arguments[index]
	if !strings.HasPrefix(arg, "-") || arg == "-" || arg == "--" {
		return "", 1
	}
	for _, flag := range flags {
		for _, spelling := range flag.Spellings {
			_, separate, ok := adapter.MatchFlag(arg, spelling)
			if !ok {
				continue
			}
			width := 1
			if separate && flag.TakesValue && index+1 < len(arguments) {
				// The value is the next argument — unless nothing follows,
				// which is a malformed line the harness will refuse anyway.
				width = 2
			}
			return flag.Spellings[0], width
		}
	}
	return "", 1
}
