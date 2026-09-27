package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/iiiokojiadbi/rewake/internal/grant"
	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/role"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// maxGrantDirs bounds the directories one task grants, both flags together.
const maxGrantDirs = 8

// dirGrants is what --grant-dir and --grant-dir-broad resolve to.
type dirGrants struct {
	// dirs are carried by the message: resolved, none inside another.
	dirs []string
	// broad are those of dirs main confirmed as broad.
	broad []string
	// writable lie in the recipient's workspace already, and are not carried.
	writable []string
}

// requestedDirGrants checks the directories a task grants (docs/grants.md).
// A call to change is refused with exit 2; a directory that is not there, or
// a recipient whose harness cannot take one, with exit 1.
func requestedDirGrants(call Call, sender, target registry.Session, senderErr error, dir string) (dirGrants, error) {
	var result dirGrants
	plain, broad := call.Lists["grant-dir"], call.Lists["grant-dir-broad"]
	if len(plain)+len(broad) == 0 {
		return result, nil
	}
	refuse := func(reason string) (dirGrants, error) {
		return dirGrants{}, &UsageError{Command: call.Command, Message: "--grant-dir: " + reason}
	}
	if _, addendum := call.Flags["to"]; addendum {
		return refuse("--to excludes it: an addendum carries no grant of its own; send a new task with --grant-dir instead.")
	}
	if len(plain)+len(broad) > maxGrantDirs {
		return refuse(fmt.Sprintf("a task grants at most %d directories, both flags together, and this one names %d; grant the directory that holds them.", maxGrantDirs, len(plain)+len(broad)))
	}
	if senderErr != nil || sender.Role != role.Main.ID {
		return refuse("only a verified current main session grants a directory; ask main to send the task.")
	}
	kind, err := chosenKind(call)
	if err != nil {
		return result, err
	}
	if kind.kind != inbox.Task && kind.kind != inbox.Question {
		return refuse("only a task or a question carries a grant; a heads-up asks for no work to write.")
	}
	adapter, _ := harness.Find(target.Harness)
	if capable, ok := adapter.(harness.DirGrantHarness); !ok || !capable.SupportsDirGrant() {
		return dirGrants{}, &FailedError{Message: fmt.Sprintf("--grant-dir: %s runs %s, which cannot take a directory into a running session yet. Do that part of the work yourself, or ask the owner to add the directory to that session in its own terminal.", target.Name, target.Harness)}
	}

	cwd, err := os.Getwd()
	if err != nil {
		return dirGrants{}, failf("--grant-dir: cannot read the current directory to resolve a relative path: %v", err)
	}
	rules := grant.CurrentEnv(state.RootForRoom(dir), harness.AllProtectedDirs()).Rules()
	workspace, _ := filepath.EvalSymlinks(target.CWD)
	var resolved, confirmed []string
	check := func(flag, given string, confirm bool) error {
		if strings.TrimSpace(given) == "" {
			// Resolved against the current directory, an empty value would
			// grant main's own checkout without naming it.
			return &UsageError{Command: call.Command, Message: flag + " needs a directory; an empty value names none."}
		}
		path, err := grant.Resolve(given, cwd)
		if err == nil && workspace != "" && grant.Within(path, workspace) {
			// Carried nowhere, so checked against nothing: the recipient's
			// own workspace is a live session's directory, which is broad.
			result.writable = append(result.writable, path)
			return nil
		}
		if err == nil {
			err = rules.Check(given, path, confirm)
		}
		var refusal *grant.Refusal
		if errors.As(err, &refusal) && refusal.Code == ExitFailed {
			return &FailedError{Message: flag + " " + refusal.Message}
		}
		if err != nil {
			return &UsageError{Command: call.Command, Message: flag + " " + err.Error()}
		}
		resolved = append(resolved, path)
		if confirm {
			confirmed = append(confirmed, path)
		}
		return nil
	}
	for _, given := range plain {
		if err := check("--grant-dir", given, false); err != nil {
			return dirGrants{}, err
		}
	}
	for _, given := range broad {
		if err := check("--grant-dir-broad", given, true); err != nil {
			return dirGrants{}, err
		}
	}
	result.dirs = grant.Outermost(resolved)
	for _, path := range result.dirs {
		if slices.Contains(confirmed, path) {
			result.broad = append(result.broad, path)
		}
	}
	return result, nil
}

// writableLines say which granted directories were not carried, because the
// recipient can write them already.
func writableLines(session registry.Session, model sendModel) []string {
	if len(model.AlreadyWritable) == 0 {
		return nil
	}
	return []string{fmt.Sprintf("Rewake: already writable by %s, in its workspace %s; not granted: %s", session.Name, session.CWD, strings.Join(model.AlreadyWritable, ", "))}
}
