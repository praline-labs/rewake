package launch

import (
	"fmt"
	"slices"
	"strings"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/core/worktype"
)

// GeneratedWorktree is the worktree of a worker that asked for one without a
// name: rewake generates it.
const GeneratedWorktree = "true"

// refusal is a request refused for what it asks, its text the whole sentence
// the caller is shown.
type refusal struct{ reason string }

func (r *refusal) Error() string { return r.reason }

func refuse(format string, args ...any) error {
	return &refusal{fmt.Sprintf(format, args...)}
}

// Request is what main asks a worker to be raised with at rewake space open:
// the template, the flags typed beside it, and the words after --. A flag not
// typed is nil; one typed with no value is "".
type Request struct {
	// Alias names the template: a worker is raised only from one (the
	// owner's decision, October 19, 2026), which names its harness and role.
	Alias string
	// Worktree is --worktree: GeneratedWorktree for a generated name, else
	// its name.
	Worktree   *string
	NoWorktree bool
	// Type is --type, the kind of work by name.
	Type   *string
	NoType bool
	// Args are the words typed after --, the harness's.
	Args []string
}

// Raised is a worker resolved from a request: what its space's record, its
// space host and its launch carry. Every conflict between the template and
// what was typed is settled before it exists.
type Raised struct {
	Harness adapter.Harness
	Role    role.Role
	// Command is the program the template starts, empty for the harness's
	// own.
	Command string
	// Args are the template's arguments with the typed ones merged in, the
	// worktree flag taken out of both.
	Args []string
	// Template is the alias the worker was raised from.
	Template string
	// Worktree is the one worktree the worker starts in: "" for none,
	// GeneratedWorktree, or its name.
	Worktree string
	// Enter says the worker may enter a checkout mid-session: its template
	// says enter_worktree = true and it starts without one.
	Enter bool
	// Work is the kind of work it is raised for, empty Name for none.
	Work worktype.Type
	// Label is the template's name for its model, empty when it has none.
	Label string
}

// Raise resolves a request against the catalog of templates: the template,
// its worktree and its kind of work, each refused here, before anything
// starts, when the request asks opposite things or names what is not there.
func (s *Set) Raise(catalog adapter.Catalog, request Request) (Raised, error) {
	if request.Worktree != nil && *request.Worktree == "" {
		return Raised{}, refuse("--worktree= needs a name; use --worktree for a generated name.")
	}
	if request.Alias == "" {
		return Raised{}, refuse("space open needs --alias <alias>; rewake workers lists them.")
	}
	if err := s.Err(); err != nil {
		return Raised{}, err
	}
	template, err := s.Template(request.Alias, catalog)
	if err != nil {
		return Raised{}, err
	}
	checkout, aliasArgs, typedArgs, err := resolveWorktree(request, template)
	if err != nil {
		return Raised{}, err
	}
	work, err := s.resolveType(request, template)
	if err != nil {
		return Raised{}, err
	}
	return Raised{
		Harness: template.Harness, Role: template.Role, Command: template.Command, Template: request.Alias,
		Worktree: checkout, Enter: template.EnterWorktree && checkout == "", Work: work, Label: template.Settings.Label,
		Args: MergeArguments(aliasArgs, typedArgs, template.Harness.SingleUseFlags()),
	}, nil
}

// Launch is the worker's complete ordinary launch, the words after rewake,
// with conversation as the fresh conversation id where its harness takes one.
func (r Raised) Launch(conversation string) ([]string, error) {
	return WorkerArgs(r.Harness, Worker{Command: r.Command, Args: r.Args, Worktree: r.Worktree, Conversation: conversation})
}

// resolveType resolves the worker's kind of work: a typed --type, else none
// for --no-type, else the template's type, else none. One that is not there,
// or is turned off, is refused.
func (s *Set) resolveType(request Request, template Template) (worktype.Type, error) {
	switch {
	case request.Type != nil && request.NoType:
		return worktype.Type{}, refuse("--type and --no-type ask opposite things; give one.")
	case request.Type != nil && *request.Type == "":
		return worktype.Type{}, refuse("--type needs a name; rewake workers lists the types.")
	case request.NoType:
		return worktype.Type{}, nil
	case request.Type != nil:
		return s.Type(*request.Type)
	case template.Type != "":
		value, err := s.Type(template.Type)
		if err != nil {
			return worktype.Type{}, fmt.Errorf("alias %q names type = %q: %w; give --type or --no-type", template.Name, template.Type, err)
		}
		return value, nil
	}
	return worktype.Type{}, nil
}

// resolveWorktree resolves the worker's worktree from what asks for one: a
// typed --worktree, or the harness's flag typed after --, wins; else
// --no-worktree leaves the template's out; else the template's worktree =
// true, or the flag in its args, stands. Two typed requests, or a typed one
// beside --no-worktree, are refused: a worker has one worktree. The flag is
// taken out of both argument lists, so the launch carries the one answer the
// space host also gets.
func resolveWorktree(request Request, template Template) (string, []string, []string, error) {
	aliasArgs, typedArgs := slices.Clone(template.Args), slices.Clone(request.Args)
	typed := request.Worktree != nil
	if _, ok := template.Harness.(adapter.WorktreeHarness); !ok {
		if typed {
			return "", nil, nil, refuse("%s makes no worktree through rewake; open the space without --worktree.", template.Harness.Title())
		}
		return "", aliasArgs, typedArgs, nil
	}
	fromAlias, aliasArgs, err := takeWorktree(template.Harness, aliasArgs)
	if err != nil {
		return "", nil, nil, fmt.Errorf("alias %q: %w", template.Name, err)
	}
	afterDashes, typedArgs, err := takeWorktree(template.Harness, typedArgs)
	if err != nil {
		return "", nil, nil, err
	}
	switch {
	case typed && afterDashes != "":
		return "", nil, nil, refuse("--worktree is given both to space open and after --; a worker has one worktree, so give it once.")
	case request.NoWorktree && (typed || afterDashes != ""):
		return "", nil, nil, refuse("--no-worktree and --worktree ask opposite things; give one.")
	case afterDashes != "":
		return afterDashes, aliasArgs, typedArgs, nil
	case typed:
		return *request.Worktree, aliasArgs, typedArgs, nil
	case request.NoWorktree:
		return "", aliasArgs, typedArgs, nil
	case template.Worktree:
		return GeneratedWorktree, aliasArgs, typedArgs, nil
	}
	return fromAlias, aliasArgs, typedArgs, nil
}

// takeWorktree takes the harness's worktree flag out of args, read as a
// launch reads it: "" when absent, GeneratedWorktree for a generated name,
// else the name.
func takeWorktree(h adapter.Harness, args []string) (string, []string, error) {
	name, given, rest, err := WorktreeRequest(h, args)
	switch {
	case err != nil:
		return "", nil, err
	case !given:
		return "", args, nil
	case name == "":
		return GeneratedWorktree, rest, nil
	}
	return name, rest, nil
}

// WorktreeRequest finds h's worktree flag among the arguments meant for the
// harness, for a direct launch and a worker's alike: a switch asks for a
// generated name (given, name ""), =<name> names the checkout. A spaced value
// is not read: the word after a switch is the harness's, a prompt most often,
// and taking it would change what a launch line already means. Where the
// harness's own flag takes a spaced name, such a word is refused instead: the
// person meant it as the name. A harness rewake makes no worktree for has no
// flag to find.
func WorktreeRequest(h adapter.Harness, args []string) (name string, given bool, rest []string, err error) {
	taker, ok := h.(adapter.WorktreeHarness)
	if !ok {
		return "", false, args, nil
	}
	flag := taker.WorktreeFlag()
	visible := adapter.BeforeTerminator(args)
	rest = make([]string, 0, len(args))
	for index, arg := range visible {
		value, separate, ok := adapter.MatchFlag(arg, flag)
		if !ok {
			rest = append(rest, arg)
			continue
		}
		if given {
			return "", false, nil, refuse("%s is given twice; a launch has one worktree.", flag)
		}
		if !separate && value == "" {
			return "", false, nil, refuse("%s= needs a name; write %s alone for a generated one.", flag, flag)
		}
		if separate && taker.WorktreeNameSpaced() && index+1 < len(visible) && !strings.HasPrefix(visible[index+1], "-") {
			word := visible[index+1]
			return "", false, nil, refuse(
				"rewake's %s takes its name only as %s=<name>, and %q after it would reach the harness as the prompt of a worktree with a generated name. Write %s=%s to name it, or %s -- %q for a generated name with that prompt.",
				flag, flag, word, flag, word, flag, word)
		}
		given, name = true, value
	}
	return name, given, append(rest, args[len(visible):]...), nil
}
