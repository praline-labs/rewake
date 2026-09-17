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
	for _, arg := range BeforeTerminator(args) {
		if arg == flag || strings.HasPrefix(arg, flag+"=") {
			return true
		}
	}
	return false
}
