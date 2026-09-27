package wrap

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/grant"
	"github.com/iiiokojiadbi/rewake/internal/grantauth"
	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/proc"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// grantLifetime is how long main's wrapper can confirm a grant: the wait of
// the task that carries it, and some to spare for the pass that delivers it.
const grantLifetime = inbox.DefaultTTL + 5*time.Minute

// checkGrant checks a message's grant again at delivery, with this session's
// own view of the machine — its home, its PATH, where each harness keeps its
// configuration — and asks the wrapper of the main that sent it whether it
// did. What the sender's view allowed, the receiver's may not, and the
// receiver's is the one the worker writes in; and nothing in the message
// itself proves who wrote it (docs/grants.md#who-can-grant).
func checkGrant(dir, name, epoch string) func(inbox.Message) error {
	root := state.RootForRoom(dir)
	return func(message inbox.Message) error {
		rules := grant.CurrentEnv(root, harness.AllProtectedDirs()).Rules()
		for _, directory := range message.GrantDirs {
			if err := rules.Recheck(directory, slices.Contains(message.GrantBroad, directory)); err != nil {
				return err
			}
		}
		return confirmGrant(dir, name, epoch, message)
	}
}

// confirmGrant asks the sending wrapper for the grant it registered, and
// takes the message's only if it is the same. A wrapper that cannot be reached
// while it runs may be busy: the message waits, within its own time to live.
// One that has ended can confirm nothing, now or later.
func confirmGrant(dir, name, epoch string, message inbox.Message) error {
	pid, start, ok := registry.ParseEpoch(message.FromEpoch)
	if !ok || !state.ValidName(message.From) {
		return errors.New("its sender is not a session run that can be asked to confirm it")
	}
	expect := grantauth.Expect{PID: pid, Start: start}
	confirmed, err := grantauth.Confirm(state.AuthorityAddress(dir, message.FromEpoch), expect, message.ID, name, epoch)
	switch {
	case err == nil:
	case errors.Is(err, grantauth.ErrUnreachable) && proc.Alive(pid, start):
		return fmt.Errorf("%w: the wrapper of %s did not confirm the grant yet: %v", inbox.ErrNotYet, message.From, err)
	case errors.Is(err, grantauth.ErrUnreachable):
		return fmt.Errorf("%s, which sent it, has ended, so nobody can confirm the grant; send the task again from the current main", message.From)
	default:
		return fmt.Errorf("%s did not confirm the grant: %v", message.From, err)
	}
	carried := grantauth.Grant{ID: message.ID, To: name, ToEpoch: epoch, Dirs: message.GrantDirs, Broad: message.GrantBroad, Git: message.GrantGit}
	if !confirmed.Same(carried) {
		return fmt.Errorf("%s registered another grant with this message than the message carries", message.From)
	}
	return nil
}

// listenAuthority binds, for a main run, the address that answers for the
// grants its commands register; any other role has none to answer for, and
// no socket. It binds before the run's record is written: from then on the
// address can be computed, and a listener there first would take main's
// registrations. Nil when it cannot bind, and the session is told why.
func listenAuthority(dir, epoch string, self int) *grantauth.Authority {
	authority, err := grantauth.Listen(state.AuthorityAddress(dir, epoch), self, grantLifetime)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "rewake: this session cannot grant directories or Git access: "+err.Error())
		return nil
	}
	return authority
}
