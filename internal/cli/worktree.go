package cli

import (
	"bytes"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/state"
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
			{Flag: "--force", Summary: "rm only: remove a worktree with changes or ignored files, with commits no branch holds, whose directory went missing, or with a session still running in it."},
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
			"rm refuses a worktree with changes, with files git ignores (a .env, a local build), with commits only it holds, whose directory is gone while the repository still lists it, or in which a rewake session still runs — the one it was made for or any started there since — and says which; --force removes it anyway. A process rewake did not start is not seen.",
		},
		Handler: handleWorktree,
	}
}

const worktreeRootHelp = "$" + worktree.RootEnv + ", else $XDG_DATA_HOME/rewake/worktrees, else ~/.local/share/rewake/worktrees"

// worktreeView is one checkout as ls reports it.
type worktreeView struct {
	worktree.Record
	// Running says a rewake session still runs in it: the one it was made
	// for, or another started there since.
	Running bool `json:"running"`
	// RunningSessions names them, each with its room.
	RunningSessions []string       `json:"runningSessions,omitempty"`
	Check           worktree.Check `json:"check"`
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
		view := worktreeView{Record: record}
		running, err := runningIn(record)
		view.RunningSessions, view.Running = running, len(running) > 0
		if err != nil {
			view.Problem = err.Error()
		} else if check, err := worktree.Inspect(record); err != nil {
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

// keepReasons says what removing a checkout would lose or cut off. Whatever
// cannot be told is an error, and rm then refuses too: without --force it
// never loses work.
func keepReasons(record worktree.Record) ([]string, error) {
	var reasons []string
	running, err := runningIn(record)
	if err != nil {
		return nil, err
	}
	if len(running) > 0 {
		reasons = append(reasons, "rewake sessions still run in it: "+strings.Join(running, ", "))
	}
	check, err := worktree.Inspect(record)
	if err != nil {
		return nil, err
	}
	if check.Missing && !check.Forgotten {
		reasons = append(reasons, "its directory "+record.Path+" is gone while the repository still lists it; if it was moved, git worktree repair <new path> run in the repository reconnects it, and whatever it holds is out of sight here")
	}
	if check.Forgotten && !check.Missing {
		reasons = append(reasons, "its repository "+record.CommonDir+" is gone, so git cannot tell what "+record.Path+" holds")
	}
	if check.Changes {
		reasons = append(reasons, "it has changes git status shows")
	}
	if check.Ignored {
		reasons = append(reasons, "it holds files git ignores, a .env or a local build, which removal deletes")
	}
	if check.Unreachable {
		reasons = append(reasons, "its HEAD "+shortCommit(check.Head)+" is on no branch, tag or remote-tracking ref, and those commits would be left to garbage collection")
	}
	return reasons, nil
}

// runningIn names the rewake sessions still running with their work in a
// checkout: the one it was made for, and any started there since — after the
// first ended, or by hand in its directory, as a taken name's refusal
// suggests. It looks in every room of the current state directory and of the
// one the owner registered in. A process rewake did not start is not seen.
func runningIn(record worktree.Record) ([]string, error) {
	var roots []string
	if root, err := state.Root(); err == nil {
		roots = append(roots, root)
	}
	owner := record.Session
	if owner != nil && owner.Dir != "" {
		if root := state.RootForRoom(owner.Dir); !slices.Contains(roots, root) {
			roots = append(roots, root)
		}
	}
	var found []string
	for _, root := range roots {
		rooms, err := state.RoomDirs(root)
		if err != nil {
			return nil, fmt.Errorf("cannot list the rooms of %s: %w", root, err)
		}
		for _, room := range rooms {
			sessions, err := registry.ListReadOnly(room)
			if err != nil {
				return nil, fmt.Errorf("cannot list the sessions of %s: %w", room, err)
			}
			for _, session := range sessions {
				made := owner != nil && filepath.Clean(owner.Dir) == room && session.Name == owner.Name && session.Epoch() == owner.Epoch
				label := session.Name + " in room " + filepath.Base(room)
				if session.Alive() && (made || worktree.Within(session.CWD, record.Path)) && !slices.Contains(found, label) {
					found = append(found, label)
				}
			}
		}
	}
	return found, nil
}

// ownerLabel names who works in a checkout: the sessions running there, or
// the one it was made for, ended.
func ownerLabel(view worktreeView) string {
	switch {
	case view.Running:
		names := make([]string, 0, len(view.RunningSessions))
		for _, running := range view.RunningSessions {
			name, _, _ := strings.Cut(running, " in room ")
			names = append(names, name)
		}
		return strings.Join(names, ",") + " (running)"
	case view.Session != nil:
		return view.Session.Name + " (ended)"
	}
	return "-"
}

func stateLabel(view worktreeView) string {
	if view.Problem != "" {
		return "unreadable"
	}
	var parts []string
	for _, part := range []struct {
		on   bool
		name string
	}{
		{view.Check.Missing, "missing"},
		{view.Check.Forgotten, "forgotten"},
		{view.Check.Changes, "changes"},
		{view.Check.Ignored, "ignored"},
		{view.Check.Unreachable, "unreachable"},
	} {
		if part.on {
			parts = append(parts, part.name)
		}
	}
	if len(parts) == 0 {
		return "clean"
	}
	return strings.Join(parts, ", ")
}
