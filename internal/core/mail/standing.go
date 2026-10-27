package mail

import (
	"errors"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
)

// OwesReports says a session of this role answers the tasks it reads: its
// reads record a wait and its turn ends write the reports. A silent role's do
// neither, so a task to one is never answered and nothing waits on it.
func OwesReports(roleName string) bool { return !role.Of(roleName).Silent }

// Why no report is coming, as the awaited view names it.
const (
	// GoneEnded is a recipient run that is over, with no run of the name since.
	GoneEnded = "ended"
	// GoneReplaced is a recipient run that is over, a new run holding the name.
	GoneReplaced = "replaced"
)

// GoneReason is GoneEnded or GoneReplaced when no report can come because the
// run the message was written for is over, and empty otherwise.
func (m AwaitedMessage) GoneReason() string {
	switch m.Run {
	case RunEnded:
		return GoneEnded
	case RunReplaced:
		return GoneReplaced
	}
	return ""
}

// Coming says a report on it can still arrive.
func (m AwaitedMessage) Coming() bool { return !m.Gone() && m.Stage != StageFailed }

// Standing is one task or question a run sent and has no report on yet, with
// what its sender needs to list it.
type Standing struct {
	AwaitedMessage
	// Under is the task an addendum adds to, as that task is now — an edit's
	// replacement included — or the one it was sent to while a copy cannot
	// be read; empty for a task that adds to nothing.
	Under string
	// Nested says the addendum is listed right after its task.
	Nested bool
}

// recipients resolves the run each recipient of a sender's messages holds
// now, reading the registry without ever writing it: Lookup prunes a dead
// record, and the callers of this module only read. Each name is read once.
type recipients struct {
	dir  string
	seen map[string]recipient
}

type recipient struct {
	run    string
	found  bool
	silent bool
	// unknown says the record could not be read. Whoever asks only informs
	// or checks before it acts, and neither can tell a run that ended from one
	// that cannot be read, so it is taken for the live run: it may still
	// report, and "no report is coming" would send the task to a new worker
	// that the live one then duplicates.
	unknown bool
}

func (r *recipients) of(name string) recipient {
	if known, ok := r.seen[name]; ok {
		return known
	}
	live, err := registry.LookupReadOnly(r.dir, name)
	got := recipient{found: err == nil, unknown: err != nil && !errors.Is(err, registry.ErrNotFound)}
	if got.found {
		got.run, got.silent = live.Epoch(), !OwesReports(live.Role)
	}
	if r.seen == nil {
		r.seen = map[string]recipient{}
	}
	r.seen[name] = got
	return got
}

// runOf says what became of the run a message to name was written for.
func (r *recipients) runOf(name, run string) RecipientRun {
	switch live := r.of(name); {
	case live.unknown, live.found && live.run == run:
		return RunLive
	case live.found:
		return RunReplaced
	}
	return RunEnded
}

// Standings lists what the run of name, epoch sent — tasks and questions —
// and has no report on yet, oldest first, each addendum right after the task
// it adds to. A task whose recipient answers nothing (OwesReports) is left
// out unless its run is gone. Like OwedMessages, it only reads; a record that
// cannot be read makes it an error rather than a partial list.
func Standings(dir, name, epoch string) ([]Standing, error) {
	who := &recipients{dir: dir}
	sent, err := awaited(dir, name, epoch, who.runOf)
	if err != nil {
		return nil, err
	}
	var standings []Standing
	for _, message := range sent {
		if !message.Gone() && who.of(message.To).silent {
			continue
		}
		standing := Standing{AwaitedMessage: message}
		if message.AddendumTo != "" {
			if current, err := CurrentTask(dir, message.To, message.AddendumTo); err == nil {
				standing.Under = current
			}
		}
		standings = append(standings, standing)
	}
	return afterTheirTask(standings), nil
}

// StandingOf places one message the run of name, epoch sent, as Standings
// would: where it stands, or that a report has settled it.
func StandingOf(dir, name, epoch string, message Message) (AwaitedMessage, bool, error) {
	who := &recipients{dir: dir}
	return awaitedOne(dir, name, epoch, message, who.runOf)
}

// afterTheirTask lists each addendum right after the task it adds to, which
// other tasks sent in between would otherwise separate it from.
func afterTheirTask(messages []Standing) []Standing {
	roots := map[string]bool{}
	for _, message := range messages {
		roots[message.ID] = message.Under == ""
	}
	ordered := make([]Standing, 0, len(messages))
	for _, message := range messages {
		if roots[message.Under] {
			continue
		}
		ordered = append(ordered, message)
		for _, addendum := range messages {
			if addendum.Under == message.ID && roots[message.ID] {
				addendum.Nested = true
				ordered = append(ordered, addendum)
			}
		}
	}
	return ordered
}
