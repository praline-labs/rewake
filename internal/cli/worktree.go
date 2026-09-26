package cli

import (
	"bytes"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/worktree"
)

// worktreeCommand lists and removes the checkouts rewake made for launches.
// One command with a verb, as the table has no subcommands: the verb is its
// first positional.
func worktreeCommand() *Command {
	return &Command{
		Name:           "worktree",
		Args:           "ls | rm <name>",
		MaxPositionals: 2,
		Summary:        "The worktrees rewake made for launches with --worktree: list them, or remove one.",
		Options: []Option{
			{Flag: "--force", Summary: "rm only: remove a worktree with changes, with commits no branch holds, or of a session that still runs."},
			jsonOption,
		},
		Examples: []string{
			"rewake worktree ls",
			"rewake worktree ls --json",
			"rewake worktree rm fix-login",
			"rewake worktree rm rewake-3f9a1c/fix-login --force",
		},
		Next: []string{"rewake codex --worktree"},
		Notes: []string{
			"They live under " + worktreeRootHelp + ", one directory per repository, named for it; each worktree has its record beside it, saying whose session it was made for, from which repository and at which commit.",
			"rm takes a name, or <repository>/<name> as ls prints it when one name is in two repositories. It removes the checkout with git worktree remove, so the repository forgets it too; the commits made in it stay in the repository while a branch holds them.",
			"rm refuses a worktree with changes or with commits only it holds, and one whose session still runs, and says which; --force removes it anyway.",
		},
		Handler: handleWorktree,
	}
}

const worktreeRootHelp = "$" + worktree.RootEnv + ", else $XDG_DATA_HOME/rewake/worktrees, else ~/.local/share/rewake/worktrees"

// worktreeView is one checkout as ls reports it.
type worktreeView struct {
	worktree.Record
	// Running says the session it was made for still runs.
	Running bool           `json:"running"`
	Check   worktree.Check `json:"check"`
	// Problem is why the checkout could not be looked at, when it could not.
	Problem string `json:"problem,omitempty"`
}

type worktreeListing struct {
	Root      string         `json:"root"`
	Worktrees []worktreeView `json:"worktrees"`
}

type worktreeRemoval struct {
	Removed worktree.Record `json:"removed"`
	Forced  bool            `json:"forced,omitempty"`
}

func handleWorktree(ctx *Context, call Call) error {
	verb := ""
	if len(call.Positionals) > 0 {
		verb = call.Positionals[0]
	}
	root, err := worktree.Root()
	if err != nil {
		return &UsageError{Command: call.Command, Message: err.Error()}
	}
	switch {
	case verb == "ls" && len(call.Positionals) == 1:
		if call.Switch("force") {
			return &UsageError{Command: call.Command, Message: "--force is for rm; ls removes nothing."}
		}
		return listWorktrees(ctx, root)
	case verb == "rm" && len(call.Positionals) == 2:
		return removeWorktree(ctx, call, root, call.Positionals[1])
	case verb == "rm":
		return &UsageError{Command: call.Command, Message: "rm needs the name of one worktree: rewake worktree rm <name>. rewake worktree ls lists them."}
	case verb == "ls":
		return &UsageError{Command: call.Command, Message: "ls takes no name; it lists every worktree."}
	}
	return &UsageError{Command: call.Command, Message: fmt.Sprintf("worktree needs ls or rm, got %q.", verb)}
}

func listWorktrees(ctx *Context, root string) error {
	records, err := worktree.List(root)
	if err != nil {
		return &FailedError{Message: err.Error()}
	}
	listing := worktreeListing{Root: root, Worktrees: []worktreeView{}}
	for _, record := range records {
		view := worktreeView{Record: record, Running: sessionRuns(record)}
		if check, err := worktree.Inspect(record); err != nil {
			view.Problem = err.Error()
		} else {
			view.Check = check
		}
		listing.Worktrees = append(listing.Worktrees, view)
	}
	return printValue(ctx, listing, func() []string {
		if len(listing.Worktrees) == 0 {
			return []string{"No worktrees under " + root + "."}
		}
		var buffer bytes.Buffer
		writer := tabwriter.NewWriter(&buffer, 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(writer, "WORKTREE\tSESSION\tSTATE\tFROM\tPATH")
		for _, view := range listing.Worktrees {
			_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n", view.Ref(), ownerLabel(view), stateLabel(view), shortCommit(view.Commit), view.Path)
		}
		_ = writer.Flush()
		lines := []string{"root: " + root}
		return append(lines, strings.Split(strings.TrimRight(buffer.String(), "\n"), "\n")...)
	})
}

func removeWorktree(ctx *Context, call Call, root, ref string) error {
	found, err := worktree.Find(root, ref)
	if err != nil {
		return &FailedError{Message: err.Error()}
	}
	switch len(found) {
	case 0:
		return &UsageError{Command: call.Command, Message: fmt.Sprintf("No worktree %q under %s. rewake worktree ls lists them.", ref, root)}
	case 1:
	default:
		var refs []string
		for _, record := range found {
			refs = append(refs, record.Ref())
		}
		return &UsageError{Command: call.Command, Message: fmt.Sprintf("%q names a worktree in %d repositories: %s. Name one as <repository>/<name>.", ref, len(found), strings.Join(refs, ", "))}
	}
	record := found[0]
	force := call.Switch("force")
	if !force {
		if reasons, err := keepReasons(record); err != nil {
			return &FailedError{Message: fmt.Sprintf("cannot tell whether %s holds work: %v. --force removes it without looking.", record.Ref(), err)}
		} else if len(reasons) > 0 {
			return &UsageError{Command: call.Command, Message: fmt.Sprintf("%s is kept: %s. Remove it anyway with rewake worktree rm %s --force.", record.Ref(), strings.Join(reasons, "; "), record.Ref())}
		}
	}
	if err := worktree.Remove(record, force); err != nil {
		return &FailedError{Message: err.Error()}
	}
	result := worktreeRemoval{Removed: record, Forced: force}
	return printValue(ctx, result, func() []string {
		return []string{"removed " + record.Ref() + " at " + record.Path}
	})
}

// keepReasons says what removing a checkout would lose or cut off.
func keepReasons(record worktree.Record) ([]string, error) {
	var reasons []string
	if sessionRuns(record) {
		reasons = append(reasons, "its session "+record.Session.Name+" in room "+record.Session.Room+" still runs")
	}
	check, err := worktree.Inspect(record)
	if err != nil {
		return nil, err
	}
	if check.Changes {
		reasons = append(reasons, "it has changes git status shows")
	}
	if check.Unreachable {
		reasons = append(reasons, "its HEAD "+shortCommit(check.Head)+" is on no branch or tag, and those commits would be left to garbage collection")
	}
	return reasons, nil
}

// sessionRuns says the session a checkout was made for still runs: its record
// is in the room it registered in, alive, and of the same run.
func sessionRuns(record worktree.Record) bool {
	owner := record.Session
	if owner == nil || owner.Dir == "" {
		return false
	}
	session, err := registry.Load(owner.Dir, owner.Name)
	return err == nil && session.Epoch() == owner.Epoch && session.Alive()
}

func ownerLabel(view worktreeView) string {
	if view.Session == nil {
		return "-"
	}
	if view.Running {
		return view.Session.Name + " (running)"
	}
	return view.Session.Name + " (ended)"
}

func stateLabel(view worktreeView) string {
	switch {
	case view.Problem != "":
		return "unreadable"
	case view.Check.Missing:
		return "missing"
	case view.Check.Changes && view.Check.Unreachable:
		return "changes, unreachable"
	case view.Check.Changes:
		return "changes"
	case view.Check.Unreachable:
		return "unreachable"
	}
	return "clean"
}
