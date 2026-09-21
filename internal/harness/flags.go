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
			switch {
			case arg == form:
				if index+1 < len(visible) {
					values = append(values, visible[index+1])
				} else {
					values = append(values, "")
				}
			case strings.HasPrefix(arg, form+"="):
				values = append(values, strings.TrimPrefix(arg, form+"="))
			case isShort(form) && strings.HasPrefix(arg, form) && len(arg) > len(form):
				values = append(values, strings.TrimPrefix(arg, form))
			default:
				continue
			}
			break
		}
	}
	return values
}

// isShort reports whether a form is a single-letter flag, which is the only
// kind that can be written joined to its value.
func isShort(form string) bool {
	return len(form) == 2 && form[0] == '-' && form[1] != '-'
}
