package cli

import (
	"strings"

	"github.com/praline-labs/rewake/internal/adapter"
)

// table is the command table of one invocation. It is built from the catalog
// the binary hands to Run rather than held by the package, so the launch
// commands are exactly the adapters of that catalog and a test builds its own
// from adapters of its own.
type table struct {
	catalog adapter.Catalog
	groups  []Group
}

func newTable(catalog adapter.Catalog) *table {
	return &table{catalog: catalog, groups: buildGroups(catalog)}
}

// lookupOption finds a flag among the global options and those of a command.
// With no command it searches every command instead: flags written before the
// command word are read first and checked against the command afterwards, which
// is what makes "rewake --name api <harness>" possible at all.
func (t *table) lookupOption(command *Command, name string) (Option, bool) {
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
	for _, group := range t.groups {
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
func (t *table) repeatable(command *Command, name string) bool {
	option, ok := t.lookupOption(command, name)
	return ok && option.Repeatable
}

func (t *table) takesValue(command *Command, name string) bool {
	option, ok := t.lookupOption(command, name)
	return ok && option.Value != ""
}

// launchFlagTakesValue answers, for a flag standing before the command word,
// whether the next argument is its value. Only launch commands and the global
// options may appear there, so those are what it looks through — and it
// answers for the whole set rather than one command, because the command is
// exactly what is not known yet at that point.
func (t *table) launchFlagTakesValue(name string) bool {
	for _, option := range globalOptions {
		if strings.TrimPrefix(option.Flag, "--") == name {
			return option.Value != ""
		}
	}
	for _, group := range t.groups {
		for _, command := range group.Commands {
			if !command.Raw {
				continue
			}
			if option, ok := t.lookupOption(command, name); ok {
				return option.Value != ""
			}
		}
	}
	return false
}

// knownFlag reports whether a flag exists at all in this position.
func (t *table) knownFlag(command *Command, name string) bool {
	_, ok := t.lookupOption(command, name)
	return ok
}

// findCommand returns the command with this name.
func (t *table) findCommand(name string) *Command {
	for _, group := range t.groups {
		for _, command := range group.Commands {
			if command.Name == name {
				return command
			}
		}
	}
	return nil
}

// commandNames lists every command name, in guide order.
func (t *table) commandNames() []string {
	var names []string
	for _, group := range t.groups {
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
func (t *table) nearestCommand(name string) string {
	best, bestDistance := "", 3
	for _, candidate := range t.commandNames() {
		if strings.HasPrefix(candidate, name) && len(name) >= 2 {
			return candidate
		}
		if distance := editDistance(name, candidate); distance < bestDistance {
			best, bestDistance = candidate, distance
		}
	}
	return best
}
