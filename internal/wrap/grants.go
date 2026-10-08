package wrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/praline-labs/rewake/internal/grant"
	"github.com/praline-labs/rewake/internal/grantauth"
	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/proc"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

// grantLifetime is how long main's wrapper keeps a grant whose letter it
// cannot find: the wait of the task that carries it, and some to spare for the
// pass that delivers it. One whose task it finds it keeps until that task is
// closed (grantauth.Authority).
const grantLifetime = inbox.DefaultTTL + 5*time.Minute

// checkGrant checks a message's grant again at delivery, with this session's
// own view of the machine — its home, its PATH, where each harness keeps its
// configuration — and asks the wrapper of the main that sent it whether it
// did. What the sender's view allowed, the receiver's may not, and the
// receiver's is the one the worker writes in; and nothing in the message
// itself proves who wrote it (docs/grants.md#who-can-grant).
//
// For a harness that takes a grant through its own hook, the grant then
// waits for the session to be idle, as every grant does, and goes into the
// keeper the hook asks; keeper is nil for any other. thread is the session's
// conversation; nil for a harness that names none.
//
// The confirmation here names no conversation: a message that waits — for an
// idle session, or for a harness busy with a turn — may be pinned to another
// one than the session had when it was first checked, and main keeps the
// conversation this wrapper names, not a guess. pinGrant tells it the one the
// letter is pinned to, and again at each pin of a delivery tried again.
func checkGrant(dir, name, epoch string, keeper *grantauth.Keeper, busy func() bool, thread func() (string, error)) func(inbox.Message) error {
	root := state.RootForRoom(dir)
	return func(message inbox.Message) error {
		rules := grant.CurrentEnv(root, harness.AllProtectedDirs()).Rules()
		for _, directory := range message.GrantDirs {
			if err := rules.Recheck(directory, slices.Contains(message.GrantBroad, directory)); err != nil {
				return err
			}
		}
		if err := confirmGrant(dir, name, epoch, "", message); err != nil || keeper == nil || len(message.GrantDirs) == 0 {
			return err
		}
		if busy != nil && busy() {
			return fmt.Errorf("%w: %s", inbox.ErrNotYet, idleWait)
		}
		// Read once the session is idle, just before the letter is pinned:
		// the conversation a resume would look for the grant in.
		conversation := ""
		if thread != nil {
			conversation, _ = thread()
		}
		// Where it came from, so a run resuming the conversation can ask
		// for it again (grant_resume.go).
		origin := grant.Entry{Message: message.ID, At: time.Now(), Thread: conversation, From: message.From, FromEpoch: message.FromEpoch}
		return keeper.GrantFrom(origin, message.GrantDirs, false)
	}
}

// pinGrant tells the main that sent a granted task the conversation its
// letter was pinned to, as the letter becomes readable: the only one a resume
// may take the grant into again (docs/grants-resume.md). A delivery tried
// again pins the letter again, maybe into another conversation, and tells
// main that one. Told by this wrapper
// and no other process, and told nothing else: the grant was confirmed
// already. A main that does not answer leaves the grant without a
// conversation, as one delivered into none — it holds for this run and is not
// restored after a resume — rather than holding the letter back.
func pinGrant(dir, name, epoch string) func(inbox.Message, string) {
	return func(message inbox.Message, thread string) {
		pid, start, ok := registry.ParseEpoch(message.FromEpoch)
		if !ok || thread == "" {
			return
		}
		_, _ = grantauth.Confirm(state.AuthorityAddress(dir, message.FromEpoch), grantauth.Expect{PID: pid, Start: start}, message.ID, name, epoch, thread)
	}
}

// idleWait is why a grant for a hook waits: a task that arrives on a turn of
// its own is reported on by that turn, and the grant ends with the report.
const idleWait = "a task carrying a grant waits for the session to be idle, so it arrives on a turn of its own"

// keepGrants starts, for a harness that takes a grant through its own hook,
// the keeper that hook asks; nil for any other harness, and when the address
// cannot be bound, which the session is told.
func keepGrants(ctx context.Context, dir, name, epoch string, self int, adapter harness.Harness) *grantauth.Keeper {
	granter, ok := adapter.(harness.HookGranter)
	if !ok {
		return nil
	}
	keeper, err := grantauth.Keep(state.KeeperAddress(dir, name, epoch), self)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "rewake: this session cannot take a directory grant: "+err.Error())
		return nil
	}
	keeper.Decide = granter.DecideGrant
	keeper.Settled = func(id string) bool { return inbox.Settled(dir, name, id) }
	keeper.Mirror = func(entries []grant.Entry) { _ = grant.Save(dir, name, epoch, entries) }
	go keeper.Serve(ctx)
	return keeper
}

// working says whether an observed session is in a turn, from what its
// telemetry last heard; not knowing is not working, since a grant through a
// hook holds from whenever the session next asks to write.
func working(observer harness.Observer) func() bool {
	if observer == nil {
		return nil
	}
	return func() bool {
		snapshot := observer.SessionState()
		return snapshot.ActivityFresh && snapshot.Activity != nil && *snapshot.Activity == "working"
	}
}

// confirmGrant asks the sending wrapper for the grant it registered, and
// takes the message's only if it is the same. A wrapper that cannot be reached
// while it runs may be busy: the message waits, within its own time to live.
// One that has ended can confirm nothing, now or later. thread tells it the
// conversation the message goes into.
func confirmGrant(dir, name, epoch, thread string, message inbox.Message) error {
	pid, start, ok := registry.ParseEpoch(message.FromEpoch)
	if !ok || !state.ValidName(message.From) {
		return errors.New("its sender is not a session run that can be asked to confirm it")
	}
	expect := grantauth.Expect{PID: pid, Start: start}
	confirmed, err := grantauth.Confirm(state.AuthorityAddress(dir, message.FromEpoch), expect, message.ID, name, epoch, thread)
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
	authority.Open = func(held grantauth.Grant) (bool, bool) { return inbox.TaskOpen(dir, held.To, held.ID) }
	authority.Owns = func(name, epoch string) bool { return registry.OwnsName(dir, name, epoch) }
	return authority
}
