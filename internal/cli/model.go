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

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/host/launch"
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
	// Repeatable marks a flag that may be given more than once, each value
	// kept in Call.Lists. Any other flag given twice is refused: keeping the
	// last one silently dropped the first.
	Repeatable bool
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
	// is here so the table can be checked against the catalog: a launch
	// command that starts the wrong harness is otherwise indistinguishable.
	Harness adapter.Harness
	// Access classifies a parsed call of this command as a view or a change
	// (access.go). It is required: a form without a class is one nobody
	// decided, and the table test refuses it.
	Access func(Call) access
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
	// access is the class of the form this invocation runs (access.go).
	access access
	// table is the command table of this invocation, built from the catalog
	// the binary handed to Run.
	table   *table
	aliases *launch.Set
	// argv is the invocation's arguments, which a hand-off to the room's own
	// copy passes on (handoff.go); empty for a form that is no command of a
	// room — the guide, help, the version — which this build answers.
	argv []string
}

// Call is one parsed invocation.
type Call struct {
	Command     *Command
	Positionals []string
	// Flags holds every flag given; a switch maps to "true"/"false". A
	// repeatable flag maps to its last value.
	Flags map[string]string
	// Lists holds every value of a repeatable flag, in order.
	Lists map[string][]string
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
