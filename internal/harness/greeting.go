package harness

import (
	"strings"

	"github.com/iiiokojiadbi/rewake/internal/brief"
)

// GreetingPolicy describes only arguments whose arity is known. Unknown flags
// preserve caller intent by opting out rather than consuming a guessed prompt.
type GreetingPolicy struct{ Values, Switches, Continued string }

// GreetingPrompt never merges with or replaces a user prompt. Continuations
// already have a turn; a bare terminator can still receive the new positional.
func GreetingPrompt(request LaunchRequest, policy GreetingPolicy) (string, string) {
	if !request.Greeting {
		return "", ""
	}
	contains := func(list, value string) bool {
		for _, item := range strings.Fields(list) {
			if item == value {
				return true
			}
		}
		return false
	}
	for index := 0; index < len(request.Args); index++ {
		arg := request.Args[index]
		if arg == "--" {
			if index+1 == len(request.Args) {
				break
			}
			return "", "not adding a greeting: a user prompt follows --"
		}
		key, _, joined := strings.Cut(arg, "=")
		if contains(policy.Continued, key) || key == "--help" || key == "-h" || key == "--version" {
			return "", "not adding a greeting: this invocation continues a thread or requests command output"
		}
		shortValue := false
		for _, flag := range strings.Fields(policy.Values) {
			if len(flag) == 2 && strings.HasPrefix(arg, flag) && len(arg) > 2 {
				shortValue = true
				break
			}
		}
		if shortValue {
			continue
		}
		if contains(policy.Values, key) {
			if !joined {
				index++
				if index >= len(request.Args) || request.Args[index] == "--" {
					return "", "not adding a greeting: an option value is missing"
				}
			}
			continue
		}
		if contains(policy.Switches, key) {
			continue
		}
		return "", "not adding a greeting: arguments already contain a prompt, subcommand or unknown option"
	}
	return brief.Greeting(request.BriefContext()), ""
}

// AppendGreeting terminates option parsing after all transport flags. Both
// single-value and variadic options must leave the bootstrap text positional.
func AppendGreeting(args []string, greeting string) []string {
	if len(BeforeTerminator(args)) == len(args) {
		args = append(args, "--")
	}
	return append(args, greeting)
}
