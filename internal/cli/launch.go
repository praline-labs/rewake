package cli

import (
	"context"

	"github.com/iiiokojiadbi/rewake/internal/harness"
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
			return &FailedError{Message: err.Error()}
		}

		code, err := wrap.Run(context.Background(), wrap.Request{
			Harness: h,
			Dir:     dir,
			Name:    call.Flag("name", ""),
			Args:    call.Raw,
			Intro:   !call.Switch("no-intro"),
		})
		if err != nil {
			return &FailedError{Message: err.Error()}
		}
		if code != 0 {
			return &ExitCodeError{Code: code}
		}
		return nil
	}
}
