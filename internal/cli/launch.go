package cli

import (
	"context"
	"errors"
	"strings"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/role"
	"github.com/iiiokojiadbi/rewake/internal/state"
	"github.com/iiiokojiadbi/rewake/internal/wrap"
)

// handleLaunch starts one harness as a rewake session. It returns only when the
// harness exits, and with its exit code: rewake sits in the middle and should
// not hide what the program said.
func handleLaunch(h harness.Harness) func(*Context, Call) error {
	return func(_ *Context, call Call) error {
		dir, err := state.Dir()
		if err != nil {
			// An unusable state directory is something about this call and its
			// environment, not about a target that refused: code 2, so a caller
			// branching on the code tries to fix the call.
			return &UsageError{Message: err.Error()}
		}

		part, err := chosenRole(call)
		if err != nil {
			return err
		}

		code, err := wrap.Run(context.Background(), wrap.Request{
			Harness: h,
			Dir:     dir,
			Name:    call.Flag("name", ""),
			Args:    call.Raw,
			Intro:   !call.Switch("no-intro"),
			Role:    part,
		})
		if err != nil {
			var taken *registry.NameTakenError
			if errors.As(err, &taken) {
				return &UsageError{Command: call.Command, Message: taken.Error() + " Pick another name, or omit --name to get the next free one."}
			}
			if errors.Is(err, registry.ErrUnusableName) {
				return &UsageError{Command: call.Command, Message: err.Error()}
			}
			return &FailedError{Message: err.Error()}
		}
		if code != 0 {
			return &ExitCodeError{Code: code}
		}
		return nil
	}
}

// chosenRole reads the role from the flags. A session is the default role unless
// another is written out: every worker must report its turns, so that is what
// happens without a word.
func chosenRole(call Call) (role.Role, error) {
	chosen := role.Default()
	var flags []string
	for _, candidate := range role.All() {
		if candidate.ID != role.Default().ID && call.Switch(candidate.ID) {
			chosen = candidate
			flags = append(flags, "--"+candidate.ID)
		}
	}
	if len(flags) > 1 {
		return role.Role{}, &UsageError{
			Command: call.Command,
			Message: strings.Join(flags, " and ") + " exclude each other: a session has one role.",
		}
	}
	return chosen, nil
}

// roleOptions are the launch flags that choose a role, for the command table.
func roleOptions() []Option {
	var options []Option
	for _, candidate := range role.All() {
		if candidate.ID != role.Default().ID {
			options = append(options, Option{Flag: "--" + candidate.ID, Summary: candidate.Summary})
		}
	}
	return options
}
