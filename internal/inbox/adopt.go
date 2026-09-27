package inbox

import (
	"context"
	"os"
	"slices"

	"github.com/iiiokojiadbi/rewake/internal/state"
)

// A cold resume starts a new run of a session in the conversation an earlier
// run worked in. What that run read and had not reported on yet is in the
// conversation, and the new run is the one that will finish it; so the new run
// takes over those waits, and its turn's report settles them
// (docs/delivery.md#a-resumed-conversation). A task is matched to the
// conversation by the thread its delivery was pinned to (thread.go). A task
// delivered into another conversation, or one nobody pinned, is not taken
// over: the new run never saw it.
//
// The earlier runs' records are swept only once the new run knows its
// conversation, and has taken over what belongs to it.

// AdoptWaits moves to this run the waits of earlier runs of the name for the
// messages delivered into thread, as if this run had read them now. It
// returns the ids taken over. The caller holds the mailbox lock.
func AdoptWaits(dir, name, epoch, thread string) []string {
	if thread == "" {
		return nil
	}
	runs, err := os.ReadDir(state.AwaitingPath(dir, name))
	if err != nil {
		return nil
	}
	var adopted []string
	for _, run := range runs {
		if !run.IsDir() || run.Name() == epoch {
			continue
		}
		for _, waiter := range Waiters(dir, name, run.Name()) {
			for _, id := range waiter.Messages {
				if !safeID(id) || deliveryThread(dir, name, id) != thread || slices.Contains(adopted, id) {
					continue
				}
				if markScopedAwaiting(dir, name, epoch, Message{ID: id, From: waiter.Name, FromEpoch: waiter.Epoch}) == nil {
					adopted = append(adopted, id)
				}
			}
		}
	}
	return adopted
}

// followEarlierRun takes over, once this run's conversation is known, what the
// earlier runs of the name owe in it, and then forgets their waits. A session
// whose harness names no conversation forgets them at once: nothing it runs
// could continue one.
func (s *Server) followEarlierRun(ctx context.Context) {
	if s.followed || s.Epoch == "" {
		return
	}
	thread := ""
	if s.Thread != nil {
		var err error
		if thread, err = s.Thread(); err != nil || thread == "" {
			return
		}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if s.lockWithContext(ctx, func() error {
		AdoptWaits(s.Dir, s.Name, s.Epoch, thread)
		sweepAwaiting(s.Dir, s.Name, s.Epoch)
		return nil
	}) == nil {
		s.followed = true
	}
}

// adoptedBy finds the run of the recipient that took over a message's wait
// from the run it was written for: the report will come from that run.
func adoptedBy(dir string, message Message) (string, bool) {
	runs, err := os.ReadDir(state.AwaitingPath(dir, message.To))
	if err != nil {
		return "", false
	}
	for _, run := range runs {
		if !run.IsDir() || run.Name() == message.ToEpoch {
			continue
		}
		for _, waiter := range Waiters(dir, message.To, run.Name()) {
			if waiter.Name == message.From && waiter.Epoch == message.FromEpoch && slices.Contains(waiter.Messages, message.ID) {
				return run.Name(), true
			}
		}
	}
	return "", false
}
