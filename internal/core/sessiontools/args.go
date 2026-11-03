package sessiontools

import (
	"math"
	"strconv"
	"strings"
)

// The input is the call's arguments as decoded JSON: a number is a float64,
// so an id is checked to be a whole positive number, and the string "3" is no
// id.

// fixed is the arguments of a tool that takes no input.
func fixed(args ...string) func(map[string]any) ([]string, string) {
	return func(map[string]any) ([]string, string) { return args, "" }
}

// word is a value that may stand as one argument: a non-empty string that
// does not begin with a dash, which would be read as a flag.
func word(value any) (string, bool) {
	text, ok := value.(string)
	return text, ok && text != "" && !strings.HasPrefix(text, "-")
}

// positive is a whole number from 1, written as an argument.
func positive(value any) (string, bool) {
	number, ok := value.(float64)
	if !ok || number <= 0 || number != math.Trunc(number) || math.IsInf(number, 0) {
		return "", false
	}
	return strconv.FormatFloat(number, 'f', -1, 64), true
}

func landArgs(input map[string]any) ([]string, string) {
	name, ok := word(input["name"])
	if !ok {
		return nil, "worktree_land needs name, the worktree's name from worktree_list"
	}
	args := []string{"worktree", "land", name}
	// An empty into is refused, not read as "leave out": it would land into
	// the default (2afdce1).
	if value, given := input["into"]; given {
		into, ok := word(value)
		if !ok {
			return nil, "into must be a branch name; leave it out for the default"
		}
		args = append(args, "--into="+into)
	}
	return append(args, "--json"), ""
}

func enterArgs(input map[string]any) ([]string, string) {
	value, given := input["name"]
	if !given {
		return []string{"worktree", "enter", "--json"}, ""
	}
	name, ok := word(value)
	if !ok {
		return nil, "name must be a worktree name; leave it out for a generated one"
	}
	return []string{"worktree", "enter", name, "--json"}, ""
}

func readArgs(input map[string]any) ([]string, string) {
	id, ok := positive(input["id"])
	if !ok {
		return nil, "brief_read needs id, the brief's number from 1"
	}
	return []string{"brief", "show", id}, ""
}

func tickArgs(input map[string]any) ([]string, string) {
	id, ok := positive(input["id"])
	if !ok {
		return nil, "brief_tick needs id, the brief's number from 1"
	}
	item, ok := positive(input["item"])
	if !ok {
		return nil, "brief_tick needs item, the item's number from 1 as brief_read shows it"
	}
	edition, ok := positive(input["edition"])
	if !ok {
		return nil, "brief_tick needs edition, the edition brief_read named"
	}
	undo := false
	if value, given := input["undo"]; given {
		if undo, ok = value.(bool); !ok {
			return nil, "undo is true or false"
		}
	}
	args := []string{"brief", "tick", id, item, "--edition=" + edition}
	if undo {
		args = append(args, "--undo")
	}
	return args, ""
}

// commentArgs puts the text after --, so one that begins with a dash is no
// flag.
func commentArgs(input map[string]any) ([]string, string) {
	id, ok := positive(input["id"])
	if !ok {
		return nil, "brief_comment needs id, the brief's number from 1"
	}
	text, ok := input["text"].(string)
	if !ok || strings.TrimSpace(text) == "" {
		return nil, "brief_comment needs text, the comment"
	}
	return []string{"brief", "comment", id, "--", text}, ""
}
