package cli

import (
	"fmt"
	"strings"
)

/*
Minimal argument parser. The shape is small and fixed:

	rewake [--global] <command> [positional...] [--flag value] [--switch]
	rewake [--name N] <harness> [everything after belongs to the harness]

A flag parser framework would add startup cost to a command an agent calls many
times a session, and would not answer the one question that matters here: where
rewake's arguments end and the harness's begin.
*/

// parsed is the outcome of reading argv: either a call, or a request for the
// guide, help or version.
type parsed struct {
	Call    Call
	Guide   bool
	Help    bool
	Version bool
}

// parse reads argv into a call against the command table.
func parse(argv []string) (parsed, error) { return parseKnowing(argv, nil) }

// parseKnowing is parse with the aliases this process can expand, so that a
// word which is neither a command nor an alias is refused with both lists. A
// person who mistypes an alias is told what exists, not only that this is not
// a command.
func parseKnowing(argv []string, aliases []string) (parsed, error) {
	var result parsed
	result.Call.Flags = map[string]string{}
	result.Call.Lists = map[string][]string{}

	index := 0
	// Flags before the command word: global ones, plus the launch flags, which
	// must come first because everything after a harness name is the harness's.
	for index < len(argv) && (strings.HasPrefix(argv[index], "--") || argv[index] == "-h") {
		if argv[index] == "-h" {
			// -h is --help here as after the command word, and what follows
			// is checked the same way: returning at once printed the guide
			// for "-h send" and let "-h --bogus" through.
			result.Call.Flags["help"] = "true"
			index++
			continue
		}
		consumed, err := readFlag(argv, index, nil, &result.Call)
		if err != nil {
			return result, err
		}
		index = consumed
	}

	result.Help = result.Call.Switch("help")
	result.Version = result.Call.Switch("version")

	if index >= len(argv) {
		// No command: either a lone --help/--version, or nothing at all.
		result.Guide = !result.Help && !result.Version
		return result, nil
	}

	word := argv[index]
	if strings.HasPrefix(word, "-") {
		return result, &UsageError{Message: fmt.Sprintf("%s is not a command. rewake's own flags are long ones, such as --help; run rewake for the map.", word)}
	}

	command := findCommand(word)
	if command == nil {
		message := fmt.Sprintf("No command %q.", word)
		if near := nearestCommand(word); near != "" {
			message += fmt.Sprintf(" Did you mean %q?", near)
		}
		if len(aliases) > 0 {
			message += " Aliases here: " + strings.Join(aliases, ", ") + "."
		}
		message += " Run rewake for the map."
		return result, &UsageError{Message: message}
	}
	result.Call.Command = command
	index++

	// Flags read before the command word are checked against it now: --name
	// belongs to a launch command and nowhere else.
	for name := range result.Call.Flags {
		if !knownFlag(command, name) {
			return result, unknownFlagError(command, name)
		}
	}

	if command.Raw {
		rest := argv[index:]
		// A launch command hands everything after its name to the harness, so
		// "rewake claude --help" is claude's own help. Asking rewake for this
		// page instead is the one exception, and only as the first token: any
		// later --help is an argument of a real harness call.
		if len(rest) > 0 && (rest[0] == "--help" || rest[0] == "-h") {
			result.Help = true
			return result, nil
		}
		result.Call.Raw = append([]string{}, rest...)
		return result, nil
	}

	endOfFlags := false
	for index < len(argv) {
		token := argv[index]
		switch {
		case token == "--" && !endOfFlags:
			endOfFlags = true
			index++
		case !endOfFlags && strings.HasPrefix(token, "--"):
			consumed, err := readFlag(argv, index, command, &result.Call)
			if err != nil {
				return result, err
			}
			index = consumed
		case !endOfFlags && token == "-h":
			// The one short flag an agent types by habit. Taken for text, it
			// would be sent as a task that reads "-h".
			result.Call.Flags["help"] = "true"
			index++
		case !endOfFlags && shortFlag(token):
			return result, &UsageError{
				Command: command,
				Message: fmt.Sprintf("%s does not take %s: its flags are long ones, listed below. A text that begins with a dash goes after --.", command.Name, token),
			}
		default:
			result.Call.Positionals = append(result.Call.Positionals, token)
			index++
		}
	}

	result.Help = result.Call.Switch("help")
	result.Version = result.Call.Switch("version")
	if result.Help || result.Version {
		return result, nil
	}

	if command.MaxPositionals != Variadic && len(result.Call.Positionals) > command.MaxPositionals {
		return result, &UsageError{
			Command: command,
			Message: fmt.Sprintf(
				"%s accepts at most %d positional argument(s), got %d. Quote multiword text as one argument.",
				command.Name, command.MaxPositionals, len(result.Call.Positionals)),
		}
	}

	return result, nil
}

// shortFlag tells a token written as a short flag, -x or -json, from text:
// "-" alone is stdin, and a text such as "- fix the login" or "-5" is not
// shaped like a flag.
func shortFlag(token string) bool {
	if len(token) < 2 || token[0] != '-' || token[1] == '-' {
		return false
	}
	letter := token[1]
	if (letter < 'a' || letter > 'z') && (letter < 'A' || letter > 'Z') {
		return false
	}
	return !strings.ContainsAny(token, " \t\n")
}

// readFlag reads one flag at argv[index] into the call and returns the next
// index.
func readFlag(argv []string, index int, command *Command, call *Call) (int, error) {
	body := strings.TrimPrefix(argv[index], "--")
	if body == "" {
		return index, &UsageError{Command: command, Message: "Bare -- is not a flag."}
	}

	name, value, inline := strings.Cut(body, "=")
	if !knownFlag(command, name) {
		return index, unknownFlagError(command, name)
	}
	next := index + 1
	switch {
	case inline:
	case takesValue(command, name):
		if index+1 >= len(argv) || strings.HasPrefix(argv[index+1], "--") {
			return index, &UsageError{
				Command: command,
				Message: fmt.Sprintf("Flag --%s needs a value.", name),
			}
		}
		value, next = argv[index+1], index+2
	default:
		value = "true"
	}

	if repeatable(command, name) {
		call.Lists[name] = append(call.Lists[name], value)
	} else if _, given := call.Flags[name]; given {
		return index, &UsageError{
			Command: command,
			Message: fmt.Sprintf("Flag --%s is given twice; give it once.", name),
		}
	}
	call.Flags[name] = value
	return next, nil
}

// unknownFlagError explains a flag the command does not take. Silently ignoring
// one is worse than refusing: a typo in a filter returns an unfiltered answer
// with exit code 0, and the caller believes it.
func unknownFlagError(command *Command, name string) error {
	if command == nil {
		return &UsageError{Message: fmt.Sprintf("No global flag --%s. Run rewake for the map.", name)}
	}
	return &UsageError{
		Command: command,
		Message: fmt.Sprintf("%s does not take --%s. Its flags are listed below.", command.Name, name),
	}
}
