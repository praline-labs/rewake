/*
Package cli turns one table of commands into everything the caller sees: the
parser, the guide printed without arguments, the help page of a command, and the
hint attached to a refusal.

The table is the single source on purpose. A guide that drifts from the flags a
command actually takes is worse than no guide: it teaches a wrong call
confidently, and the caller learns otherwise one failed run later.
*/
package cli

import (
	"io"
	"strings"

	"github.com/iiiokojiadbi/rewake/internal/harness"
)

// Option is a flag of a command, or a global flag.
type Option struct {
	// Flag is the long form, including the dashes: "--name".
	Flag string
	// Value is the placeholder for a flag that takes one, empty for a switch.
	Value string
	// Summary says what the flag does, in one line.
	Summary string
	// Required marks a flag the command refuses to run without.
	Required bool
}

// Label renders the flag the way help prints it.
func (o Option) Label() string {
	if o.Value == "" {
		return o.Flag
	}
	return o.Flag + " " + o.Value
}

// Command is one invocation rewake answers.
type Command struct {
	// Name is the word that selects the command.
	Name string
	// Args is the positional shape shown in help: "<name> <text>".
	Args string
	// MaxPositionals caps positional tokens; Variadic permits any number.
	MaxPositionals int
	// Summary is the one-line entry in the guide.
	Summary string
	// Options are the flags of this command, beyond the global ones.
	Options []Option
	// Examples are real invocations: whoever reads them copies them as-is.
	Examples []string
	// Next names what one usually runs after this command.
	Next []string
	// Notes hold decisions and limits worth knowing before running it.
	Notes []string
	// Raw means everything after the command name belongs to the program this
	// command starts, and rewake must not interpret it.
	Raw bool
	// Hidden keeps a command out of the guide and out of "did you mean". It is
	// for the ones a harness calls on its own; an agent has no reason to.
	Hidden bool
	// Harness is what this command starts, for the commands that start one. It
	// is here so the table can be checked against the catalogue: a launch
	// command that starts the wrong harness is otherwise indistinguishable.
	Harness harness.Harness
	// Handler runs the command.
	Handler func(*Context, Call) error
}

// Variadic marks a command that accepts any number of positional arguments.
const Variadic = -1

// Label renders the command the way the guide and help print it.
func (c *Command) Label() string {
	if c.Args == "" {
		return c.Name
	}
	return c.Name + " " + c.Args
}

// visibleCommands are the commands of a group the guide shows.
func visibleCommands(group Group) []*Command {
	out := make([]*Command, 0, len(group.Commands))
	for _, command := range group.Commands {
		if !command.Hidden {
			out = append(out, command)
		}
	}
	return out
}

// Group is a titled section of the guide.
type Group struct {
	Title    string
	Summary  string
	Commands []*Command
}

// FlowStep is one line of the worked order printed in the guide.
type FlowStep struct {
	Command string
	Summary string
}

// Note is a titled paragraph about how the tool behaves.
type Note struct {
	Title string
	Body  string
}

// Context carries what a handler prints to and how.
type Context struct {
	Stdout io.Writer
	Stderr io.Writer
	// JSON asks for the model behind the output instead of the printed lines.
	JSON bool
}

// Call is one parsed invocation.
type Call struct {
	Command     *Command
	Positionals []string
	// Flags holds every flag given; a switch maps to "true"/"false".
	Flags map[string]string
	// Raw is everything after the command name of a Raw command.
	Raw []string
}

// Flag returns the value of a flag, or fallback when it was not given.
func (c Call) Flag(name, fallback string) string {
	if value, ok := c.Flags[name]; ok {
		return value
	}
	return fallback
}

// Switch reports whether a boolean flag was given and not disabled.
func (c Call) Switch(name string) bool {
	value, ok := c.Flags[name]
	return ok && value != "false"
}

// lookupOption finds a flag among the global options and those of a command.
// With no command it searches every command instead: flags written before the
// command word are read first and checked against the command afterwards, which
// is what makes "rewake --name api claude" possible at all.
func lookupOption(command *Command, name string) (Option, bool) {
	flag := "--" + name
	for _, option := range globalOptions {
		if option.Flag == flag {
			return option, true
		}
	}
	if command != nil {
		for _, option := range command.Options {
			if option.Flag == flag {
				return option, true
			}
		}
		return Option{}, false
	}
	for _, group := range Groups() {
		for _, candidate := range group.Commands {
			for _, option := range candidate.Options {
				if option.Flag == flag {
					return option, true
				}
			}
		}
	}
	return Option{}, false
}

// takesValue reports whether a flag carries a value.
func takesValue(command *Command, name string) bool {
	option, ok := lookupOption(command, name)
	return ok && option.Value != ""
}

// knownFlag reports whether a flag exists at all in this position.
func knownFlag(command *Command, name string) bool {
	_, ok := lookupOption(command, name)
	return ok
}

// findCommand returns the command with this name.
func findCommand(name string) *Command {
	for _, group := range Groups() {
		for _, command := range group.Commands {
			if command.Name == name {
				return command
			}
		}
	}
	return nil
}

// commandNames lists every command name, in guide order.
func commandNames() []string {
	var names []string
	for _, group := range Groups() {
		for _, command := range group.Commands {
			if !command.Hidden {
				names = append(names, command.Name)
			}
		}
	}
	return names
}

// nearestCommand returns the closest command name to a mistyped one, if any is
// close enough to be worth suggesting.
func nearestCommand(name string) string {
	best, bestDistance := "", 3
	for _, candidate := range commandNames() {
		if strings.HasPrefix(candidate, name) && len(name) >= 2 {
			return candidate
		}
		if distance := editDistance(name, candidate); distance < bestDistance {
			best, bestDistance = candidate, distance
		}
	}
	return best
}

// editDistance is the Levenshtein distance between two short words.
func editDistance(a, b string) int {
	previous := make([]int, len(b)+1)
	current := make([]int, len(b)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(a); i++ {
		current[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			current[j] = min(previous[j]+1, min(current[j-1]+1, previous[j-1]+cost))
		}
		copy(previous, current)
	}
	return previous[len(b)]
}
