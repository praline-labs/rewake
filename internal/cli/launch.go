package cli

import (
	"context"
	"errors"
	"strings"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
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
		if prefix, present := call.Flags["name"]; present && prefix == "" {
			return &UsageError{Command: call.Command, Message: "--name needs a nonempty prefix; omit --name to use the selected role."}
		}
		root, err := state.Root()
		if err != nil {
			// An unusable state directory is something about this call and its
			// environment, not about a target that refused: code 2, so a caller
			// branching on the code tries to fix the call.
			return &UsageError{Message: err.Error()}
		}

		dir, err := state.RoomDir(root, call.Flag("room", state.DefaultRoom))
		if err != nil {
			return &UsageError{Command: call.Command, Message: err.Error()}
		}

		part, err := chosenRole(call)
		if err != nil {
			return err
		}

		code, err := wrap.Run(context.Background(), wrap.Request{
			OnTurn: func(self registry.Session, result harness.Completion) error {
				return completeTurn(dir, self, turnResult{ID: result.ID, Text: result.Text, Failed: result.Kind == inbox.Error, Stopped: result.Kind == inbox.Stopped}, result.Thread)
			},
			Harness: h,
			Dir:     dir,
			Name:    call.Flag("name", ""),
			Args:    call.Raw,
			Intro:   !call.Switch("no-intro"),
			Role:    part,
		})
		if err != nil {
			var mainTaken *wrap.MainTakenError
			if errors.As(err, &mainTaken) {
				return &UsageError{Command: call.Command, Message: mainTaken.Error()}
			}
			var taken *registry.NameTakenError
			if errors.As(err, &taken) {
				return &UsageError{Command: call.Command, Message: taken.Error() + " Pick another prefix with --name, or omit --name to get the next free role-based address."}
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

// chosenRole leaves an omitted role unset so the wrapper can distinguish
// default general from an explicit --general in its recorded reason.
func chosenRole(call Call) (role.Role, error) {
	chosen := role.Role{}
	var flags []string
	for _, candidate := range role.All() {
		if call.Switch(candidate.ID) {
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
		options = append(options, Option{Flag: "--" + candidate.ID, Summary: candidate.Summary})
	}
	return options
}

// roleSummary keeps the guide on the same catalog as launch flags and help.
func roleSummary() string {
	var descriptions []string
	for _, candidate := range role.All() {
		label := "--" + candidate.ID
		descriptions = append(descriptions, label+": "+candidate.Summary)
	}
	return "Without a role flag, every launch uses general, even in an empty room. Only --main creates main, and it refuses if main is occupied. A name prefix never selects a role. " + strings.Join(descriptions, " ") + " A silent coordinator prevents reports from waking each other indefinitely."
}
