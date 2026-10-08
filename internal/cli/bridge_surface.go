package cli

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/praline-labs/rewake/internal/bridge"
)

// The words a tool call may run (docs/mail-bridge.md#one-implementation-and-a-narrow-surface).
// The mail tool reaches the CLI from outside the harness's sandbox, so it runs
// the mail and nothing else: reading, the pending mark, a heads-up to a live
// session, who this is, the sessions of the room, and continuing or
// reconciling what an earlier call began. A command added to the CLI later stays out until it is reviewed for
// this surface, which is why the list names what is allowed rather than what
// is not.

// toolFlags lists, per allowed command, the flags a tool call may give it.
var toolFlags = map[string][]string{
	"inbox":   {"json", "peek", "message", "owed", "awaited", "next"},
	"pending": {"json"},
	"send":    {"notify", "json", "wait"},
	"whoami":  {"json"},
	"retry":   {"json"},
	"list":    {"json"},
}

// maxToolWait bounds send's --wait under the tool: the call has a deadline of
// its own, shorter than a person's patience at a shell.
const maxToolWait = 5

// ToolWords checks the words of a tool call against the surface and returns
// them normalized: the form the receipt digest is taken over. The server
// calls it before starting the CLI, and the CLI calls it again once it is
// running in bridge mode; neither trusts the other to have done it.
func ToolWords(words []string) ([]string, error) {
	call, err := toolCall(words)
	if err != nil {
		return nil, err
	}
	return normalized(call), nil
}

// toolCall parses tool words with the real parser — without aliases, which
// could turn a word into a launch — and refuses everything outside the surface.
func toolCall(words []string) (parsed, error) {
	if len(words) == 0 {
		return parsed{}, &UsageError{Message: "the rewake tool needs the words of a rewake command, such as [\"inbox\"]; the program name is not one of them."}
	}
	size := 0
	for _, word := range words {
		size += len(word)
	}
	if len(words) > bridge.MaxWords || size > bridge.MaxWordsBytes {
		return parsed{}, &UsageError{Message: fmt.Sprintf("the rewake tool takes at most %d words of %d bytes in all; a long letter goes in parts, not in one call.", bridge.MaxWords, bridge.MaxWordsBytes)}
	}
	result, err := parse(words)
	if err != nil {
		return parsed{}, err
	}
	command := result.Call.Command
	if command == nil || result.Guide || result.Version {
		return parsed{}, toolRefusal(nil, "only the mail commands run through the rewake tool")
	}
	allowed, ok := toolFlags[command.Name]
	if !ok {
		return parsed{}, toolRefusal(nil, command.Name+" does not run through the rewake tool")
	}
	if result.Help {
		return result, nil
	}
	for name, value := range result.Call.Flags {
		if !contains(allowed, name) {
			return parsed{}, toolRefusal(command, fmt.Sprintf("%s --%s does not run through the rewake tool", command.Name, name))
		}
		if !takesValue(command, name) && value != "true" {
			return parsed{}, &UsageError{Command: command, Message: fmt.Sprintf("--%s is a switch and takes no value.", name)}
		}
	}
	if err := toolShape(result.Call); err != nil {
		return parsed{}, err
	}
	return result, nil
}

// toolShape checks what the flags alone do not: the positionals each command
// needs, and send's limits.
func toolShape(call Call) error {
	command := call.Command
	switch command.Name {
	case "inbox":
		_, err := inboxSelection(call)
		return err
	case "pending", "retry":
		if len(call.Positionals) != 1 || strings.TrimSpace(call.Positionals[0]) == "" {
			return &UsageError{Command: command, Message: command.Name + " needs one word: " + command.Args + "."}
		}
	case "send":
		if !call.Switch("notify") {
			return toolRefusal(command, "only send --notify runs through the rewake tool; a task goes from main in a shell")
		}
		if len(call.Positionals) != 2 {
			return &UsageError{Command: command, Message: "send needs a session name and the text to deliver."}
		}
		if text := call.Positionals[1]; text == "-" || strings.TrimSpace(text) == "" {
			return &UsageError{Command: command, Message: "the rewake tool sends the text given as a word; it reads nothing from stdin and sends nothing empty."}
		}
		if raw, given := call.Flags["wait"]; given {
			seconds, err := strconv.ParseFloat(raw, 64)
			if err != nil || seconds < 0 || seconds > maxToolWait {
				return &UsageError{Command: command, Message: fmt.Sprintf("--wait takes 0 to %d seconds through the rewake tool, got %q.", maxToolWait, raw)}
			}
		}
	}
	return nil
}

// toolRefusal is a refusal that names the shell as the place for the rest.
func toolRefusal(command *Command, reason string) error {
	return &UsageError{Command: command, Message: "Rewake: " + reason + ". The tool runs: inbox (--peek, --message, --owed, --awaited, --next), pending, send --notify, whoami, retry and list."}
}

// normalized is the canonical form of a call: the command, its flags sorted
// with their values inline, then its positionals after --. Two spellings of
// one call normalize alike; texts stay exactly as given.
func normalized(result parsed) []string {
	call := result.Call
	words := []string{call.Command.Name}
	names := make([]string, 0, len(call.Flags))
	for name := range call.Flags {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if takesValue(call.Command, name) {
			words = append(words, "--"+name+"="+call.Flags[name])
		} else {
			words = append(words, "--"+name)
		}
	}
	if len(call.Positionals) > 0 {
		words = append(append(words, "--"), call.Positionals...)
	}
	return words
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}
