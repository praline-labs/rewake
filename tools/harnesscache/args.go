package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/iiiokojiadbi/rewake/tools/harnesscache/cache"
	"github.com/iiiokojiadbi/rewake/tools/harnesscache/container"
)

// call is a parsed command line.
type call struct {
	command  string
	harness  cache.Harness
	selector string
	rest     []string

	cache    string
	registry string
	json     bool
	write    string
	network  string
}

// positionals are what each command takes before its flags end.
var positionals = map[string]int{"fetch": 2, "run": 2, "list": 0, "remove": 2}

// parseArgs reads the command line. An unknown flag, a flag the command does
// not take and a stray word are all refusals: a silently ignored flag returns
// an answer the caller believes.
func parseArgs(args []string) (call, error) {
	if len(args) == 0 {
		// No arguments is the guide, not a refusal: the first call of a
		// session should teach how the tool behaves.
		return call{command: "help"}, nil
	}
	c := call{command: args[0]}
	if c.command == "--help" || c.command == "-h" || c.command == "help" {
		return call{command: "help"}, nil
	}
	want, ok := positionals[c.command]
	if !ok {
		return c, fmt.Errorf("no command %q; the commands are fetch, run, list and remove", c.command)
	}
	var words []string
	for index := 1; index < len(args); index++ {
		argument := args[index]
		value := func() (string, error) {
			if name, joined, ok := strings.Cut(argument, "="); ok {
				argument = name
				return joined, nil
			}
			if index+1 >= len(args) {
				return "", fmt.Errorf("%s needs a value", argument)
			}
			index++
			return args[index], nil
		}
		var err error
		switch flag, _, _ := strings.Cut(argument, "="); flag {
		case "--":
			if c.command != "run" {
				return c, fmt.Errorf("%s takes no harness arguments after --", c.command)
			}
			c.rest = args[index+1:]
			index = len(args)
		case "--help", "-h":
			return call{command: "help"}, nil
		case "--cache":
			c.cache, err = value()
		case "--registry":
			c.registry, err = value()
		case "--json":
			if c.command != "fetch" && c.command != "list" {
				return c, fmt.Errorf("--json belongs to fetch and list, not %s", c.command)
			}
			c.json = true
		case "--write", "--network":
			if c.command != "run" {
				return c, fmt.Errorf("%s belongs to run, not %s", flag, c.command)
			}
			got, valueErr := value()
			switch {
			case valueErr != nil:
				err = valueErr
			case flag == "--write":
				// Absolute, because docker reads a relative bind source
				// as a volume name.
				c.write, err = filepath.Abs(got)
			default:
				c.network = got
			}
		default:
			if strings.HasPrefix(argument, "-") {
				return c, fmt.Errorf("unknown flag %q", argument)
			}
			words = append(words, argument)
		}
		if err != nil {
			return c, err
		}
	}
	if len(words) != want {
		return c, fmt.Errorf("%s takes %d %s, got %d: %v", c.command, want, plural(want), len(words), words)
	}
	for _, path := range []string{c.cache, c.write} {
		if err := container.Mountable(path); err != nil {
			return c, err
		}
	}
	if want == 0 {
		return c, nil
	}
	harness, err := cache.Lookup(words[0])
	if err != nil {
		return c, err
	}
	c.harness, c.selector = harness, words[1]
	if c.command == "remove" && !cache.IsExact(c.selector) {
		return c, errors.New("remove takes an exact version; `list` names the cached ones")
	}
	return c, nil
}

func plural(n int) string {
	if n == 1 {
		return "word"
	}
	return "words"
}
