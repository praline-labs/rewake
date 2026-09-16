package cli

import (
	"context"
	"errors"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/registry"
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

		code, err := wrap.Run(context.Background(), wrap.Request{
			Harness: h,
			Dir:     dir,
			Name:    call.Flag("name", ""),
			Args:    call.Raw,
			Intro:   !call.Switch("no-intro"),
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
