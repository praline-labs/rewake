package harness

import "strings"

// AddFlags inserts flags rewake adds into the caller's argument list, in front
// of a "--" terminator when there is one. Everything after that terminator is a
// positional argument of the harness, so appending there would turn a flag into
// part of a prompt.
func AddFlags(args []string, added ...string) []string {
	terminator := -1
	for index, arg := range args {
		if arg == "--" {
			terminator = index
			break
		}
	}
	if terminator < 0 {
		return append(append([]string{}, args...), added...)
	}
	out := append([]string{}, args[:terminator]...)
	out = append(out, added...)
	return append(out, args[terminator:]...)
}

// BeforeTerminator returns the arguments up to "--". What follows is input for
// the harness — a prompt, most often — and a flag-looking word in it is text,
// not a setting.
func BeforeTerminator(args []string) []string {
	for index, arg := range args {
		if arg == "--" {
			return args[:index]
		}
	}
	return args
}

// HasFlag reports whether the caller already passed a flag, in either the
// "--flag value" or the "--flag=value" form. A harness adds its own only when
// the caller has not: their flag is the one they meant.
func HasFlag(args []string, flag string) bool {
	return len(FlagValues(args, flag)) > 0
}

// FlagValues returns what the caller passed for a flag, under any of its
// spellings. One parser, because the ways of writing a flag are a property of
// the command line rather than of any one setting — and a second copy of this
// is how a spelling gets missed: an overlooked form silently overrides a
// choice the person had already made, or adds a duplicate flag the harness
// then refuses.
//
// Every shape a CLI of this kind accepts:
//
//	--flag value    -f value
//	--flag=value    -f=value
//	                -fvalue      (joined short form)
//
// A value may be empty, so presence is reported by the length of the result,
// not by the values themselves.
func FlagValues(args []string, forms ...string) []string {
	visible := BeforeTerminator(args)
	var values []string
	for index, arg := range visible {
		for _, form := range forms {
			value, separate, ok := MatchFlag(arg, form)
			if !ok {
				continue
			}
			if separate {
				value = ""
				if index+1 < len(visible) {
					value = visible[index+1]
				}
			}
			values = append(values, value)
			break
		}
	}
	return values
}

// WithoutFlag removes every occurrence of a flag that carries a value, in any
// of its spellings, before a "--" terminator. It is for a harness that reads
// a flag once and must be handed one merged value instead of the caller's.
func WithoutFlag(args []string, forms ...string) []string {
	visible := BeforeTerminator(args)
	out := make([]string, 0, len(args))
	for index := 0; index < len(visible); index++ {
		matched, skipNext := false, false
		for _, form := range forms {
			if _, separate, ok := MatchFlag(visible[index], form); ok {
				matched, skipNext = true, separate
				break
			}
		}
		if !matched {
			out = append(out, visible[index])
			continue
		}
		if skipNext {
			index++
		}
	}
	return append(out, args[len(visible):]...)
}

// MatchFlag reads one argument against one spelling of a flag. It answers the
// value the argument carries, whether the value is instead the next argument,
// and whether the argument spells this flag at all.
//
// It is the single place that knows the shapes above. Every caller that walks
// a command line — reading a value out of it, or deciding that two arguments
// name the same parameter — asks here, because a second copy of these rules is
// how a spelling gets missed, and a missed spelling is a duplicate flag the
// harness then refuses.
func MatchFlag(arg, form string) (value string, separate bool, ok bool) {
	switch {
	case arg == form:
		return "", true, true
	case strings.HasPrefix(arg, form+"="):
		return strings.TrimPrefix(arg, form+"="), false, true
	case isShort(form) && strings.HasPrefix(arg, form) && len(arg) > len(form):
		// -mvalue: a short flag joined to its value.
		return strings.TrimPrefix(arg, form), false, true
	}
	return "", false, false
}

// isShort reports whether a form is a single-letter flag, which is the only
// kind that can be written joined to its value.
func isShort(form string) bool {
	return len(form) == 2 && form[0] == '-' && form[1] != '-'
}
