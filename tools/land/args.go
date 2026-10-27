package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// call is a parsed command line.
type call struct {
	help    bool
	target  string
	subject string
	noPush  bool
	install string
}

// defaultInstall is the directory of the build the owner's room runs, as
// ~/.local/bin/rewake2 starts it.
func defaultInstall(home string) string {
	return filepath.Join(home, ".local", "share", "rewake2")
}

// parseArgs reads the command line. An unknown flag, a repeated one and a
// stray word are refusals: a land that ignored one would push something
// other than asked.
func parseArgs(args []string) (call, error) {
	var c call
	if len(args) == 0 {
		return call{help: true}, nil
	}
	seen := map[string]bool{}
	for index := 0; index < len(args); index++ {
		argument := args[index]
		flag, joined, hasValue := strings.Cut(argument, "=")
		if strings.HasPrefix(flag, "-") && seen[flag] {
			return c, fmt.Errorf("%s is given twice", flag)
		}
		seen[flag] = true
		valued := func() (string, error) {
			if !hasValue {
				if index+1 >= len(args) {
					return "", fmt.Errorf("%s needs a value", flag)
				}
				index++
				joined = args[index]
			}
			if strings.TrimSpace(joined) == "" {
				return "", fmt.Errorf("%s needs a value", flag)
			}
			return joined, nil
		}
		var err error
		switch flag {
		case "--help", "-h":
			return call{help: true}, nil
		case "--no-push":
			if hasValue {
				return c, errors.New("--no-push takes no value")
			}
			c.noPush = true
		case "--subject":
			c.subject, err = valued()
		case "--install":
			c.install, err = valued()
			if err == nil && !filepath.IsAbs(c.install) {
				err = fmt.Errorf("--install takes an absolute path, not %q", c.install)
			}
		default:
			if strings.HasPrefix(argument, "-") {
				return c, fmt.Errorf("no flag %s; the flags are --subject, --no-push, --install and --help", argument)
			}
			if c.target != "" {
				return c, fmt.Errorf("one branch or commit only: %s and %s", c.target, argument)
			}
			c.target = argument
		}
		if err != nil {
			return c, err
		}
	}
	if c.target == "" {
		return c, errors.New("name the branch or commit to land, such as wt-3da691")
	}
	if strings.Contains(c.subject, "\n") {
		return c, errors.New("--subject is one line: commits here are a single subject line and the trailers")
	}
	return c, nil
}
