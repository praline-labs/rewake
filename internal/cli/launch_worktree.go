package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/worktree"
)

// launchCheckout is the worktree a launch asked for, once made.
type launchCheckout struct {
	record worktree.Record
	// stderr is where the person is told where the session works.
	stderr io.Writer
	// launch is the command that starts the harness, for the way back in.
	launch string
}

// takeWorktree takes a harness's worktree flag out of a launch's arguments,
// makes the checkout it asks for and moves this process into it, so the
// session's record, the harness and whatever it starts all work there. A
// launch without the flag, or of a harness that makes its own, comes back
// unchanged with no checkout.
func takeWorktree(h harness.Harness, call Call, stderr io.Writer) ([]string, *launchCheckout, error) {
	taker, ok := h.(harness.WorktreeHarness)
	if !ok {
		return call.Raw, nil, nil
	}
	flag := taker.WorktreeFlag()
	name, given, args, err := worktreeRequest(call, flag)
	if err != nil || !given {
		return args, nil, err
	}
	if err := taker.WorktreeRefusal(args); err != nil {
		return nil, nil, &UsageError{Command: call.Command, Message: err.Error() + "."}
	}
	from, args, err := taker.LaunchDirectory(args)
	if err != nil {
		return nil, nil, &UsageError{Command: call.Command, Message: err.Error()}
	}
	root, err := worktree.Root()
	if err != nil {
		return nil, nil, &UsageError{Command: call.Command, Message: err.Error()}
	}
	record, err := worktree.Create(root, from, name)
	var exists *worktree.ExistsError
	var taken *worktree.BranchTakenError
	var unusable *worktree.UnusableError
	switch {
	case errors.As(err, &exists):
		return nil, nil, &UsageError{Command: call.Command, Message: fmt.Sprintf(
			"%s. Pick another name with %s=<name>, remove it with rewake worktree rm %s, or cd %s and launch there without %s.",
			exists.Error(), flag, exists.Record.Ref(), exists.Record.Path, flag)}
	case errors.As(err, &taken):
		return nil, nil, &UsageError{Command: call.Command, Message: fmt.Sprintf(
			"%s, and a worktree gets a new branch of its name. Pick another name with %s=<name>; if the worktree is already there, rewake worktree ls names it, and a launch in its directory needs no %s.",
			taken.Error(), flag, flag)}
	case errors.As(err, &unusable):
		return nil, nil, &UsageError{Command: call.Command, Message: unusable.Error() + "."}
	case err != nil:
		return nil, nil, &FailedError{Message: err.Error()}
	}
	if err := os.Chdir(record.Workdir()); err != nil {
		(&launchCheckout{record: record}).takeBack()
		return nil, nil, &FailedError{Message: fmt.Sprintf("cannot enter the new worktree %s: %v", record.Workdir(), err)}
	}
	for _, skipped := range record.Skipped {
		_, _ = fmt.Fprintf(stderr, "rewake: %s did not copy %s\n", worktree.IncludeFile, skipped)
	}
	return args, &launchCheckout{record: record, stderr: stderr, launch: "rewake " + h.ID()}, nil
}

// worktreeRequest finds the flag among the arguments meant for the harness:
// a switch asks for a generated name, =<name> names the checkout. A spaced
// value is not read: the word after a switch is the harness's, a prompt most
// often, and taking it would change what a launch line already means.
func worktreeRequest(call Call, flag string) (name string, given bool, rest []string, err error) {
	visible := harness.BeforeTerminator(call.Raw)
	rest = make([]string, 0, len(call.Raw))
	for _, arg := range visible {
		value, separate, ok := harness.MatchFlag(arg, flag)
		if !ok {
			rest = append(rest, arg)
			continue
		}
		if given {
			return "", false, nil, &UsageError{Command: call.Command, Message: flag + " is given twice; a launch has one worktree."}
		}
		if !separate && value == "" {
			return "", false, nil, &UsageError{Command: call.Command, Message: flag + "= needs a name; write " + flag + " alone for a generated one."}
		}
		given, name = true, value
	}
	return name, given, append(rest, call.Raw[len(visible):]...), nil
}

// claimed writes the session into the checkout's record once its name is
// known, so rewake worktree ls can say whose it is and whether it still runs,
// and only then tells the person where the session works: a launch the
// registration refuses — a taken name, a taken main — ends before this, and
// its checkout is taken back unannounced.
func (c *launchCheckout) claimed(dir string) func(registry.Session) error {
	return func(session registry.Session) error {
		record, err := worktree.Claim(c.record, worktree.Owner{
			Name: session.Name, Room: session.Room, Epoch: session.Epoch(), Harness: session.Harness, Dir: dir,
		})
		c.record = record
		if err == nil {
			_, _ = fmt.Fprintf(c.stderr, "rewake: working in the worktree %s at %s, on the new branch %s from commit %s%s; it stays after the session: rewake worktree land %s takes its commits, finish lands and removes it\n",
				record.Ref(), record.Path, record.Branch, shortCommit(record.Commit), includedNote(record), record.Ref())
		}
		return err
	}
}

// settle runs after the session. A checkout of a launch that failed, or whose
// harness exited with an error — a resume of a conversation that does not
// exist, say — is taken back when it holds nothing new: it was made for this
// launch alone. Anything rm without --force would keep keeps it here too. One
// the session used stays, as the harness's own worktree would, and the person
// is told where.
func (c *launchCheckout) settle(failed bool) {
	if failed {
		why := c.kept()
		if why == "" && c.takeBack() {
			return
		}
		if why == "" {
			why = "it could not be removed"
		}
		_, _ = fmt.Fprintf(c.stderr, "rewake: the launch failed, and the worktree %s stays at %s on the branch %s: %s. To go on there, cd %s and run %s without --worktree; rewake worktree land %s takes its commits, finish lands and removes it, rm removes it\n",
			c.record.Ref(), c.record.Path, c.record.Branch, why, c.record.Path, c.launch, c.record.Ref())
		return
	}
	_, _ = fmt.Fprintf(c.stderr, "rewake: the worktree %s stays at %s on the branch %s; rewake worktree land %s takes its commits, finish lands and removes it, rm removes it\n",
		c.record.Ref(), c.record.Path, c.record.Branch, c.record.Ref())
}

// kept says why a failed launch's checkout is not taken back, or "" when it
// may be.
func (c *launchCheckout) kept() string {
	check, err := worktree.Inspect(c.record)
	if err != nil {
		return err.Error()
	}
	if check.Head != c.record.Commit {
		return "its HEAD moved from the commit it was made at"
	}
	reasons, err := keepReasons(c.record)
	if err != nil {
		return err.Error()
	}
	return strings.Join(reasons, "; ")
}

// takeBack removes a checkout made for a launch that did not use it, and the
// branch made with it when nothing but that branch is lost.
func (c *launchCheckout) takeBack() bool {
	if worktree.Remove(c.record, false) != nil {
		return false
	}
	_, _ = worktree.DropBranch(c.record)
	return true
}

// includedNote names what .worktreeinclude copied in, when anything.
func includedNote(record worktree.Record) string {
	switch len(record.Included) {
	case 0:
		return ""
	case 1:
		return ", with " + record.Included[0] + " copied in by " + worktree.IncludeFile
	}
	return fmt.Sprintf(", with %d files copied in by %s", len(record.Included), worktree.IncludeFile)
}

func shortCommit(commit string) string {
	if len(commit) > 12 {
		return commit[:12]
	}
	return strings.TrimSpace(commit)
}
