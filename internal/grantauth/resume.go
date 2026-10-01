package grantauth

import (
	"errors"
	"fmt"
	"net"
	"os"

	"github.com/praline-labs/rewake/internal/grant"
	"github.com/praline-labs/rewake/internal/proc"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

// A cold resume starts a new run of the recipient, and its wrapper holds
// nothing of what the run before it was granted (docs/grants.md#after-a-cold-resume).
// What it finds on disk says which grants the conversation had, and a worker
// could have written that; so it asks main's wrapper again, which answers only
// for a grant it still holds, whose task is open, to the wrapper of the run
// that now holds the name. The grant then belongs to that run: the run before
// it can no longer be answered for, and a later resume asks the same way.

// Reconfirm asks the wrapper of the main that sent a message to hand its grant
// over to this run of the recipient, and returns it. The caller is the new
// run's wrapper itself: the other end checks that the asking process is the
// run it names.
func Reconfirm(path string, expect Expect, id, to, epoch, thread string) (Grant, error) {
	conn, err := dial(path)
	if err != nil {
		return Grant{}, err
	}
	defer func() { _ = conn.Close() }()
	if err := expect.answeredBy(conn); err != nil {
		return Grant{}, err
	}
	answer, err := exchange(conn, request{Op: opReconfirm, Grant: Grant{ID: id, To: to, ToEpoch: epoch}, Thread: thread})
	if err != nil {
		return Grant{}, err
	}
	if !proc.Alive(expect.PID, expect.Start) {
		return Grant{}, fmt.Errorf("%w: the sending session's wrapper ended while it answered", ErrNotConfirmed)
	}
	if answer.Error != "" {
		return Grant{}, fmt.Errorf("%w: %s", ErrNotConfirmed, answer.Error)
	}
	if answer.Grant == nil || answer.Grant.ID != id || answer.Grant.To != to || answer.Grant.ToEpoch != epoch {
		return Grant{}, fmt.Errorf("%w: the answer named another grant", ErrNotConfirmed)
	}
	return *answer.Grant, nil
}

// reconfirm hands a delivered grant over to a new run of its recipient. It
// answers only that run's wrapper — the process the run names, alive as it
// started, in this wrapper's namespaces, holding the recipient's name — and
// only for the conversation the grant was delivered into, while the run it
// was granted to has ended and its task is open.
func (a *Authority) reconfirm(conn *net.UnixConn, asked Grant, thread string) (Grant, error) {
	if err := askedByRun(conn, asked.ToEpoch); err != nil {
		return Grant{}, err
	}
	if a.Owns != nil && !a.Owns(asked.To, asked.ToEpoch) {
		return Grant{}, fmt.Errorf("run %s does not hold the name %s", asked.ToEpoch, asked.To)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.prune()
	held, ok := a.held[asked.ID]
	if !ok || held.grant.To != asked.To {
		return Grant{}, fmt.Errorf("this session holds no grant with message %s for %s, or its task is closed", asked.ID, asked.To)
	}
	if !held.delivered {
		return Grant{}, fmt.Errorf("the grant with message %s was never delivered, so no conversation took it", asked.ID)
	}
	if held.thread == "" || held.thread != thread {
		// The copy that pointed here names the conversation, and a worker
		// could have written it: a resume of another conversation would
		// carry the grant somewhere it never went.
		return Grant{}, fmt.Errorf("the grant with message %s went into another conversation than %q", asked.ID, thread)
	}
	if held.grant.ToEpoch == asked.ToEpoch {
		// Asked again by the run it was handed to: a retry.
		return held.grant, nil
	}
	if registry.EpochAlive(held.grant.ToEpoch) {
		return Grant{}, fmt.Errorf("the grant with message %s belongs to a run of %s that is still running", asked.ID, asked.To)
	}
	if a.Open != nil {
		if open, found := a.Open(held.grant); !open || !found {
			return Grant{}, fmt.Errorf("the task of message %s is closed", asked.ID)
		}
	}
	held.grant.ToEpoch = asked.ToEpoch
	a.held[asked.ID] = held
	return held.grant, nil
}

// askedByRun checks that the process asking is the wrapper of the run it
// names: this user's, that pid, alive as it started, in this process's
// namespaces. A sandboxed worker reaches the address too, and could name any
// run; it cannot be one.
func askedByRun(conn *net.UnixConn, epoch string) error {
	peer, err := peerOf(conn)
	if err != nil {
		return fmt.Errorf("cannot tell who asked: %v", err)
	}
	if int(peer.Uid) != os.Getuid() {
		return errors.New("asked by another user")
	}
	pid, _, ok := registry.ParseEpoch(epoch)
	if !ok || int(peer.Pid) != pid || !registry.EpochAlive(epoch) {
		return fmt.Errorf("asked by process %d, which is not the wrapper of run %s", peer.Pid, epoch)
	}
	if err := sameNamespaces(pid); err != nil {
		return fmt.Errorf("a process in a sandbox of its own is not a session's wrapper: %v", err)
	}
	return nil
}

// Restored is what came of one grant a copy named for a resumed conversation.
type Restored struct {
	Hint grant.Hint
	// Grant is what main confirmed again; set when Err is nil.
	Grant Grant
	// Err says why it was not. ErrUnreachable is a main still running that
	// did not answer yet, and it may be asked again; anything else settles it.
	Err error
}

// Restore asks, for each grant the copies name for a conversation, the main
// that sent it to hand it over to this run of the recipient.
func Restore(dir, name, epoch, thread string) []Restored {
	var restored []Restored
	for _, hint := range grant.Hints(dir, thread) {
		restored = append(restored, RestoreHint(dir, name, epoch, thread, hint))
	}
	return restored
}

// RestoreHint asks for the one grant a hint names, for the conversation this
// run continues.
func RestoreHint(dir, name, epoch, thread string, hint grant.Hint) Restored {
	outcome := Restored{Hint: hint}
	gone := fmt.Errorf("%w: %s, which sent it, has ended, and nobody else can confirm the grant", ErrNotConfirmed, hint.From)
	pid, start, ok := registry.ParseEpoch(hint.FromEpoch)
	switch {
	case !ok || !state.ValidName(hint.From):
		outcome.Err = fmt.Errorf("%w: the copy names no main run that could confirm it", ErrNotConfirmed)
	case !registry.EpochAlive(hint.FromEpoch):
		outcome.Err = gone
	default:
		outcome.Grant, outcome.Err = Reconfirm(state.AuthorityAddress(dir, hint.FromEpoch), Expect{PID: pid, Start: start}, hint.Message, name, epoch, thread)
		if errors.Is(outcome.Err, ErrUnreachable) && !proc.Alive(pid, start) {
			outcome.Err = gone
		}
	}
	return outcome
}
