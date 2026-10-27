package cli

import (
	"errors"
	"fmt"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/build"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// What a parsed form does to the room's state
// (docs/rules/state.md#what-is-a-writer). Forms, not commands, are
// classified: inbox and inbox --owed are two forms, and only one of them
// writes. The class is read from the parsed call before the handler and
// before anything that can prune, so a view never reaches a write by a path
// its handler shares with a change.

// access is the class of one parsed form.
type access int

const (
	// The zero access is a form nobody classified. The table test refuses a
	// command whose Access is missing; a handler run directly, outside Run,
	// sees it and keeps the paths of a change.
	_ access = iota
	// accessView answers from what is there and writes nothing.
	accessView
	// accessChange may write the room.
	accessChange
)

func (a access) String() string {
	switch a {
	case accessView:
		return "view"
	case accessChange:
		return "change"
	}
	return "unset"
}

// viewForm and changeForm classify a command whose every form is one class.
func viewForm(Call) access   { return accessView }
func changeForm(Call) access { return accessChange }

// formAccess is the class of a parsed shell invocation. The parser's own
// answers — the version, a help page, the guide — print from the table and
// the binary; everything else is its command's form.
func formAccess(result parsed) access {
	if result.Version || result.Help || result.Guide {
		return accessView
	}
	return result.Call.Command.Access(result.Call)
}

// inboxAccess classifies inbox by the mode its handler will take. A call its
// selection refuses writes nothing; --owed and --awaited show what is there;
// every other mode reads, marks or continues, and --peek takes the mailbox
// lock, which orders it against a read in progress.
func inboxAccess(call Call) access {
	mode, err := inboxSelection(call)
	switch {
	case err != nil, mode.owed, mode.awaited:
		return accessView
	}
	return accessChange
}

// worktreeAccess classifies worktree by its verb: enter, rm, land and finish
// make or change a checkout and look the room's sessions up; ls, and a verb the handler
// refuses before any effect, only answer.
func worktreeAccess(call Call) access {
	if len(call.Positionals) > 0 {
		switch call.Positionals[0] {
		case "enter", "rm", "land", "finish":
			return accessChange
		}
	}
	return accessView
}

// within runs one invocation's scope for its class: from the class through
// the handler, the shell's observation and the printing of the answer, or
// through a tool call's bounded output. Nothing of the invocation that can
// write runs outside it. A change holds the build lease of its room for the
// whole scope (docs/rules/state.md#the-rooms-lease), taken before any
// other lock; a launch takes its own, for the room of its directory or its space, and
// holds it for the wrapper's life. A lease refused ends the scope before the
// handler, which then has had no effect, and is answered by refused the way
// its route answers — with no write at all, the shell's observation included.
func (ctx *Context) within(class access, call Call, run func() int, refused func(error) int) int {
	ctx.access = class
	if class == accessView {
		return ctx.asView(run)
	}
	if class != accessChange || (call.Command != nil && call.Command.Harness != nil) {
		return run()
	}
	ctx.handOff()
	lease, err := acquireCurrent(call.Command)
	if err != nil {
		return refused(err)
	}
	defer lease.Release()
	return run()
}

// asView runs a view's scope in view mode: every write of the state refuses
// there, and an attempt fails the invocation even when the code that met the
// refusal swallowed it. A view takes no lease and waits for none; a room of
// another build is said to be another's, once, on stderr, whatever the form
// prints on stdout.
func (ctx *Context) asView(run func() int) int {
	ctx.handOff()
	leave := state.EnterView()
	if dir, err := state.Dir(); err == nil {
		if line := build.Foreign(dir); line != "" {
			_, _ = fmt.Fprintln(ctx.Stderr, line)
		}
	}
	code := run()
	if err := leave(); err != nil {
		_, _ = fmt.Fprintln(ctx.Stderr, "rewake: internal error: "+err.Error())
		if code == ExitOK {
			code = ExitFailed
		}
	}
	return code
}

// acquireCurrent takes the build lease of the room this invocation's
// environment names, as a writer.
func acquireCurrent(command *Command) (*build.Lease, error) {
	base, err := state.Base()
	if err != nil {
		return nil, &UsageError{Command: command, Message: err.Error()}
	}
	room, err := state.CurrentRoom()
	if err != nil {
		return nil, &UsageError{Command: command, Message: err.Error()}
	}
	return leaseError(command, func() (*build.Lease, error) { return build.Acquire(base, room) })
}

// leaseError runs one acquisition and gives its refusal the CLI's shape: an
// unusable state directory is about the call (code 2), a room held by
// another build is a refusal (code 1), its holders named by session where
// the room's records say whose they are.
func leaseError(command *Command, acquire func() (*build.Lease, error)) (*build.Lease, error) {
	lease, err := acquire()
	if err == nil {
		return lease, nil
	}
	var unusable *build.UnusableError
	if errors.As(err, &unusable) {
		return nil, &UsageError{Command: command, Message: unusable.Error()}
	}
	var refused *build.RefusedError
	if errors.As(err, &refused) {
		nameHolders(refused)
	}
	return nil, &FailedError{Message: err.Error()}
}

// nameHolders adds the session each holder serves, from the room's records
// read without a write.
func nameHolders(refused *build.RefusedError) {
	if len(refused.Holders) == 0 {
		return
	}
	sessions, err := registry.ListReadOnly(refused.Room)
	if err != nil {
		return
	}
	for index, holder := range refused.Holders {
		for _, session := range sessions {
			if session.ServicePID == holder.Pid {
				refused.Holders[index].Session = session.Name
			}
		}
	}
}
