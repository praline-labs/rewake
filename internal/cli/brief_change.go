package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/praline-labs/rewake/internal/core/taskbrief"
	"github.com/praline-labs/rewake/internal/host/reviewcheckout"
)

// The changes of a brief: main's new, edit and close; a bound run's tick and
// comment. Each holds the room's lease, then the brief lock inside the core.

func briefNew(ctx *Context, call Call, home taskbrief.Home, caller briefRun, args []string) error {
	if err := caller.mainOnly("make"); err != nil {
		return err
	}
	text, err := briefText(call, "new", args)
	if err != nil {
		return err
	}
	where, err := briefWhere(call)
	if err != nil {
		return err
	}
	brief, err := taskbrief.New(home, caller.author, text, where)
	if err != nil {
		return briefError(call, err)
	}
	summary := briefSummaryOf(brief)
	return printValue(ctx, summary, func() []string {
		return []string{fmt.Sprintf("Rewake: brief %d made, edition 1: %s. Give it with rewake send <name> \"...\" --brief %d.", brief.ID, summary.Title, brief.ID)}
	})
}

func briefEdit(ctx *Context, call Call, home taskbrief.Home, caller briefRun, args []string) error {
	if err := caller.mainOnly("edit"); err != nil {
		return err
	}
	id, err := briefID(call, "edit", args)
	if err != nil {
		return err
	}
	revision, err := positiveFlag(call, "rev", "brief edit needs --rev <n>, the revision the text starts from, as rewake brief show printed it.")
	if err != nil {
		return err
	}
	text, err := briefText(call, "edit", args[1:])
	if err != nil {
		return err
	}
	brief, err := taskbrief.Edit(home, id, caller.author, revision, text)
	if err != nil {
		return briefError(call, err)
	}
	summary := briefSummaryOf(brief)
	return printValue(ctx, summary, func() []string {
		return []string{fmt.Sprintf("Rewake: brief %d is at edition %d, revision %d: %s. Nobody is told: a worker reads the new text when a letter gives it with --brief %d.", brief.ID, brief.Edition, brief.Revision, summary.Title, brief.ID)}
	})
}

// briefClosed is close's answer.
type briefClosed struct {
	ID      int  `json:"id"`
	Already bool `json:"already"`
	// CheckoutsKept are the reviewers' checkouts the close left, each with
	// why: they hold what removing them would lose.
	CheckoutsKept map[string]string `json:"checkoutsKept,omitempty"`
	// CheckoutsError says why some could not be looked at, and so were not
	// released; the close is done all the same.
	CheckoutsError string `json:"checkoutsError,omitempty"`
}

func briefClose(ctx *Context, call Call, home taskbrief.Home, caller briefRun, args []string) error {
	if err := caller.mainOnly("close"); err != nil {
		return err
	}
	id, err := briefID(call, "close", args)
	if err != nil {
		return err
	}
	if len(args) > 1 {
		return &UsageError{Command: call.Command, Message: "brief close takes the id alone."}
	}
	already, err := taskbrief.Close(home, id)
	if err != nil {
		return briefError(call, err)
	}
	released := reviewcheckout.ReleaseBrief(home.Room, id)
	answer := briefClosed{ID: id, Already: already, CheckoutsKept: released.Kept}
	if released.Err != nil {
		answer.CheckoutsError = released.Err.Error()
	}
	return printValue(ctx, answer, func() []string {
		lines := []string{fmt.Sprintf("Rewake: brief %d closed and archived; its bindings ended. rewake brief list --closed shows it.", id)}
		if already {
			lines = []string{fmt.Sprintf("Rewake: brief %d was closed already.", id)}
		}
		for _, path := range released.Paths() {
			lines = append(lines, fmt.Sprintf("Rewake: the reviewer's checkout %s stays: %s; rewake worktree rm removes it.", path, answer.CheckoutsKept[path]))
		}
		if answer.CheckoutsError != "" {
			lines = append(lines, "Rewake: the reviewers' checkouts could not all be released: "+answer.CheckoutsError+"; rewake worktree ls shows them.")
		}
		return lines
	})
}

// briefTicked is tick's answer.
type briefTicked struct {
	ID      int            `json:"id"`
	Edition int            `json:"edition"`
	Item    taskbrief.Item `json:"item"`
}

func briefTick(ctx *Context, call Call, home taskbrief.Home, caller briefRun, args []string) error {
	id, err := briefID(call, "tick", args)
	if err != nil {
		return err
	}
	if len(args) != 2 {
		return &UsageError{Command: call.Command, Message: fmt.Sprintf("brief tick needs the id and the item's number: rewake brief tick %d 2 --edition 1.", id)}
	}
	item, ok := taskbrief.ParseID(args[1])
	if !ok {
		return &UsageError{Command: call.Command, Message: fmt.Sprintf("%q is not an item's number: a number from 1, as rewake brief show %d prints after each box.", args[1], id)}
	}
	edition, err := positiveFlag(call, "edition", fmt.Sprintf("brief tick needs --edition <e>, the edition the number was read in, as rewake brief show %d printed it.", id))
	if err != nil {
		return err
	}
	ticked, err := taskbrief.Tick(home, id, caller.author, edition, item, !call.Switch("undo"))
	if err != nil {
		return briefError(call, err)
	}
	answer := briefTicked{ID: id, Edition: edition, Item: ticked}
	return printValue(ctx, answer, func() []string {
		state := "done"
		if !ticked.Done {
			state = "not done"
		}
		return []string{fmt.Sprintf("Rewake: brief %d item #%d %s: %s.", id, ticked.N, state, ticked.Text)}
	})
}

// briefCommented is comment's answer.
type briefCommented struct {
	ID int `json:"id"`
	N  int `json:"n"`
}

func briefComment(ctx *Context, call Call, home taskbrief.Home, caller briefRun, args []string) error {
	id, err := briefID(call, "comment", args)
	if err != nil {
		return err
	}
	if len(args) != 2 {
		return &UsageError{Command: call.Command, Message: fmt.Sprintf("brief comment needs the id and the text, quoted as one argument: rewake brief comment %d -- \"...\".", id)}
	}
	text := args[1]
	if text == "-" {
		if text, err = readBounded(os.Stdin); err != nil {
			return failf("could not read the comment from stdin: %v", err)
		}
	}
	if err := taskbrief.CheckComment(text); err != nil {
		return &UsageError{Command: call.Command, Message: err.Error() + "."}
	}
	_, number, err := taskbrief.Comment(home, id, caller.author, text)
	if err != nil {
		return briefError(call, err)
	}
	answer := briefCommented{ID: id, N: number}
	return printValue(ctx, answer, func() []string {
		return []string{fmt.Sprintf("Rewake: comment #%d added to brief %d.", number, id)}
	})
}

// positiveFlag reads a required flag that holds a number from 1.
func positiveFlag(call Call, name, missing string) (int, error) {
	value, given := call.Flags[name]
	if !given {
		return 0, &UsageError{Command: call.Command, Message: missing}
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 1 {
		return 0, &UsageError{Command: call.Command, Message: fmt.Sprintf("--%s takes a number from 1, not %q.", name, value)}
	}
	return n, nil
}

// briefWhere reads --dir: an existing directory, made absolute against the
// directory the command runs in.
func briefWhere(call Call) (string, error) {
	dir, given := call.Flags["dir"]
	if !given {
		return "", nil
	}
	if dir == "" {
		return "", &UsageError{Command: call.Command, Message: "--dir needs a directory."}
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", &UsageError{Command: call.Command, Message: fmt.Sprintf("could not make %s absolute: %v", dir, err)}
	}
	if info, err := os.Stat(abs); err != nil || !info.IsDir() {
		return "", &UsageError{Command: call.Command, Message: fmt.Sprintf("--dir %s is not an existing directory; make it first, the brief goes in %s/brief-<id>.", abs, abs)}
	}
	return abs, nil
}
