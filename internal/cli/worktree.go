package cli

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/host/worktree"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// worktreeCommand lists the checkouts rewake made for launches, lands their
// work and removes them. One command with a verb, as the table has no
// subcommands: the verb is its first positional.
func worktreeCommand(catalog adapter.Catalog) *Command {
	return &Command{
		Name:           "worktree",
		Args:           "ls | status | enter [<name>] | land <name> | finish <name> | rm <name>",
		MaxPositionals: 2,
		Summary:        "List, land, finish or remove the worktrees rewake made for --worktree launches, say which one this session works in, or make the one a worker enters.",
		Options: []Option{
			{Flag: "--into", Value: "<branch>", Summary: "land and finish: the branch to fast-forward; by default the one checked out where the worktree was made from."},
			{Flag: "--force", Summary: "rm only: remove a worktree whatever rm would keep it for."},
			{Flag: "--this-room", Summary: "ls only: the worktrees made for a session of this room, not every repository's under the root."},
			jsonOption,
		},
		Examples: []string{
			"rewake worktree ls",
			"rewake worktree ls --this-room --json",
			"rewake worktree ls --json",
			"rewake worktree status --json",
			"rewake worktree enter --json",
			"rewake worktree enter fix-login --json",
			"rewake worktree land fix-login",
			"rewake worktree land fix-login --into release",
			"rewake worktree finish fix-login",
			"rewake worktree rm fix-login",
			"rewake worktree rm rewake-3f9a1c/fix-login --force",
			"rewake worktree land feat/login",
		},
		Next: worktreeLaunch(catalog),
		Notes: []string{
			"They live under " + worktreeRootHelp + ", one directory per repository, each worktree with a record beside it: the session it was made for, the repository, the commit. Each is on a branch of its own name; a slash in a name, feat/login, is + in its directory.",
			"A worktree is named by its name, or by <repository>/<name> as ls prints it when two repositories share one; a word that could be either is taken as <repository>/<name> when both parts match.",
			"ls lists every worktree under the root, of every repository and room; --this-room keeps those whose record names a session of the current room, which is what a main's worktree_list tool asks for. What it says a removal would lose is changes, ignored files, and a HEAD that no branch, tag or remote-tracking ref holds — the worktree's own branch counts, so its new commits read as held; status, not ls, forecasts a space's close.",
			"A .worktreeinclude at the repository's top, in .gitignore syntax, names the ignored files — a .env, a local configuration — each new checkout rewake makes gets a copy of, by a launch's --worktree, space open --worktree or enter. A file that cannot be copied is named, and the checkout stands without it.",
			"status answers for the session it runs in, by its room, name and run: the checkout that run claimed (path, branch, the commit it was made at) or none, and what its space's close would do with it if the space closed now — remove (nothing would be lost; files git ignores go too), keep with the reasons, or unknown. The close removes only a space worker's own checkout, so for main or a session that is no worker the answer is none: its checkout stays until finish or rm. A look that writes nothing, not a promise: later edits, commits and sessions change it.",
			"enter makes the one checkout of a worker whose template says enter_worktree = true, from the commit its session is at, and claims it for its run, which its worktree_enter tool then moves the session into; its space's close releases it as a launch's own. Called again it answers the same checkout: reused, or already when the run was started in it. Refused without that template, and for another name once the run has one: one checkout per worker.",
			"land fast-forwards the target to the worktree's branch and does nothing else: hashes are kept, the worktree and its session untouched, and it can be repeated. The target is --into or the branch checked out where the worktree was made from; checked out there, it moves by git merge --ff-only, so git refuses to overwrite changes; checked out nowhere, its ref moves. Refused: a target checked out in another checkout, the worktree's own included, or one a rebase or bisect is on; and one that moved on — rebase the branch onto it in the worktree, then land again.",
			"finish lands once more, then removes the worktree and its branch. It refuses, landing and removing nothing, while a rewake session runs there or a launch is still starting one, while it has changes or ignored files, is off its branch, is locked or holds a submodule checked out, and when the fast-forward is impossible.",
			"rm removes the checkout with git worktree remove, so the repository forgets it too, and its branch once another branch, tag or remote-tracking ref holds its commits and no checkout has it out.",
			"rm refuses, saying why, a worktree with changes; with files git ignores (a .env, a local build) other than unchanged copies .worktreeinclude made; whose HEAD is on no branch, tag or remote-tracking ref; whose directory is gone while the repository lists it; locked with git worktree lock; holding a submodule checked out; whose launch is still starting its session; or where a rewake session runs, the one it was made for or any started there since. --force removes it anyway, a locked one too. A process rewake did not start is not seen.",
			"Exit 1 for a refusal over the state of the worktree or its repository — a running session, changes, a branch or repository gone, a source on the worktree's own branch, a target that moved on or that another checkout holds, git refusing the merge, a hook still running after a minute. Exit 2 for a wrong call: a name that names no worktree, a branch name git refuses, --into the worktree's own branch or empty.",
		},
		Access:  worktreeAccess,
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
	// BranchDropped says the worktree's branch went with it; BranchKept,
	// when it stayed, says why; BranchGone, when there was none to drop.
	BranchDropped bool   `json:"branchDropped,omitempty"`
	BranchKept    string `json:"branchKept,omitempty"`
	BranchGone    string `json:"branchGone,omitempty"`
}

func (r worktreeRemoval) branchLines() []string {
	switch {
	case r.BranchDropped:
		return []string{"deleted the branch " + r.Removed.Branch + ", which another ref holds"}
	case r.BranchKept != "":
		return []string{"kept the branch " + r.Removed.Branch + ": " + r.BranchKept}
	case r.BranchGone != "" && r.Removed.Branch != "":
		return []string{"no branch " + r.Removed.Branch + " to delete: " + r.BranchGone}
	}
	return nil
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
	_, into := call.Flags["into"]
	switch {
	case call.Switch("this-room") && verb != "ls":
		return &UsageError{Command: call.Command, Message: "--this-room is for ls; the other verbs name one worktree."}
	case call.Switch("force") && verb != "rm":
		return &UsageError{Command: call.Command, Message: "--force is for rm; ls, status, enter, land and finish remove nothing without their checks. rewake worktree rm <name> --force removes without them."}
	case into && verb != "land" && verb != "finish":
		return &UsageError{Command: call.Command, Message: "--into is for land and finish."}
	case into && call.Flags["into"] == "":
		return &UsageError{Command: call.Command, Message: "--into= needs a branch name; leave --into out to land into the branch checked out where the worktree was made from."}
	case verb == "ls" && len(call.Positionals) == 1:
		return listWorktrees(ctx, call, root)
	case verb == "ls":
		return &UsageError{Command: call.Command, Message: "ls takes no name; it lists every worktree, or with --this-room those of this room."}
	case verb == "status" && len(call.Positionals) == 1:
		return worktreeStatusView(ctx, call, root)
	case verb == "status":
		return &UsageError{Command: call.Command, Message: "status takes no name; it answers for the session it runs in."}
	case verb == "enter":
		return enterWorktree(ctx, call, root)
	case (verb == "rm" || verb == "land" || verb == "finish") && len(call.Positionals) == 1:
		return &UsageError{Command: call.Command, Message: fmt.Sprintf("%s needs the name of one worktree: rewake worktree %s <name>. rewake worktree ls lists them.", verb, verb)}
	}
	if verb != "rm" && verb != "land" && verb != "finish" {
		return &UsageError{Command: call.Command, Message: fmt.Sprintf("worktree needs ls, status, enter, land, finish or rm, got %q.", verb)}
	}
	record, err := findWorktree(call, root, call.Positionals[1])
	if err != nil {
		return err
	}
	switch verb {
	case "land":
		return landWorktree(ctx, call, record)
	case "finish":
		return finishWorktree(ctx, call, record)
	}
	return removeWorktree(ctx, call, record)
}

func listWorktrees(ctx *Context, call Call, root string) error {
	records, err := worktree.List(root)
	if err != nil {
		return &FailedError{Message: err.Error()}
	}
	if call.Switch("this-room") {
		dir, err := state.Dir()
		if err != nil {
			return &UsageError{Command: call.Command, Message: err.Error()}
		}
		records = inRoom(records, dir)
	}
	listing := worktreeListing{Root: root, Worktrees: []worktreeView{}}
	for _, record := range records {
		view := worktreeView{Record: record}
		running, err := worktree.RunningIn(record)
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
		_, _ = fmt.Fprintln(writer, "WORKTREE\tSESSION\tSTATE\tBRANCH\tFROM\tPATH")
		for _, view := range listing.Worktrees {
			_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\n", view.Ref(), ownerLabel(view), stateLabel(view), branchLabel(view), shortCommit(view.Commit), view.Path)
		}
		_ = writer.Flush()
		lines := []string{"root: " + root}
		return append(lines, strings.Split(strings.TrimRight(buffer.String(), "\n"), "\n")...)
	})
}

// inRoom keeps the checkouts whose record names a session of the room at dir,
// the one that was made for, ended or not.
func inRoom(records []worktree.Record, dir string) []worktree.Record {
	var kept []worktree.Record
	for _, record := range records {
		if owner := record.Session; owner != nil && filepath.Clean(owner.Dir) == filepath.Clean(dir) {
			kept = append(kept, record)
		}
	}
	return kept
}

// findWorktree resolves the name a call gave to one checkout.
func findWorktree(call Call, root, ref string) (worktree.Record, error) {
	found, err := worktree.Find(root, ref)
	if err != nil {
		return worktree.Record{}, &FailedError{Message: err.Error()}
	}
	switch len(found) {
	case 0:
		return worktree.Record{}, &UsageError{Command: call.Command, Message: fmt.Sprintf("No worktree %q under %s. rewake worktree ls lists them.", ref, root)}
	case 1:
		return found[0], nil
	}
	var refs []string
	for _, record := range found {
		refs = append(refs, record.Ref())
	}
	return worktree.Record{}, &UsageError{Command: call.Command, Message: fmt.Sprintf("%q names a worktree in %d repositories: %s. Name one as <repository>/<name>.", ref, len(found), strings.Join(refs, ", "))}
}

func removeWorktree(ctx *Context, call Call, record worktree.Record) error {
	force := call.Switch("force")
	var keep func(worktree.Record) error
	if !force {
		keep = func(current worktree.Record) error {
			if reasons, err := worktree.KeepReasons(current); err != nil {
				return &FailedError{Message: fmt.Sprintf("cannot tell whether %s holds work: %v. --force removes it without looking.", record.Ref(), err)}
			} else if len(reasons) > 0 {
				return &FailedError{Message: fmt.Sprintf("%s is kept: %s. Remove it anyway with rewake worktree rm %s --force.", record.Ref(), strings.Join(reasons, "; "), record.Ref())}
			}
			return nil
		}
	}
	if err := worktree.RemoveChecked(record, force, keep); err != nil {
		var refused *FailedError
		if errors.As(err, &refused) {
			return err
		}
		return &FailedError{Message: err.Error()}
	}
	result := worktreeRemoval{Removed: record, Forced: force}
	result.fateOfBranch(record)
	return printValue(ctx, result, func() []string {
		return append([]string{"removed " + record.Ref() + " at " + record.Path}, result.branchLines()...)
	})
}

// ownerLabel names who works in a checkout: the sessions running there, the
// one it was made for, ended, or the reviewer it was made for.
func ownerLabel(view worktreeView) string {
	switch {
	case view.Running:
		names := make([]string, 0, len(view.RunningSessions))
		for _, running := range view.RunningSessions {
			name, _, _ := strings.Cut(running, " in room ")
			names = append(names, name)
		}
		return strings.Join(names, ",") + " (running)"
	case view.Launching():
		return "- (launching)"
	case view.Session != nil:
		return view.Session.Name + " (ended)"
	case view.Review != nil:
		return view.Review.Reviewer.Name + " (review of brief " + view.Review.Brief + ")"
	}
	return "-"
}

// branchLabel is the worktree's branch, and the branch it is on instead when
// it left it.
func branchLabel(view worktreeView) string {
	branch := view.Branch
	if branch == "" {
		branch = "-"
	}
	if view.Problem == "" && !view.Check.Missing && !view.Check.Forgotten && view.Check.Branch != view.Branch {
		now := view.Check.Branch
		if now == "" {
			now = "detached"
		}
		return branch + " (now " + now + ")"
	}
	return branch
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

// worktreeLaunch is how main gets a worker a worktree, when a harness of the
// catalog makes one; none offers nothing to run next. Not a launch: main reads
// this page from inside its session, where a launch is refused.
func worktreeLaunch(catalog adapter.Catalog) []string {
	for _, h := range catalog.All() {
		if _, ok := h.(adapter.WorktreeHarness); ok {
			return []string{"rewake space open <name> --alias <alias> --worktree"}
		}
	}
	return nil
}
