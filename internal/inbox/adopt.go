package inbox

import (
	"context"
	"errors"
	"os"
	"slices"
	"time"

	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

// A cold resume starts a new run of a session in the conversation an earlier
// run worked in. What that run read and had not reported on yet is in the
// conversation, and the new run is the one that will finish it; so the new run
// takes over those waits, and its turn's report settles them
// (docs/archive-1.x/delivery-conversation.md#a-resumed-conversation). A task is matched to the
// conversation by the thread its delivery was pinned to (thread.go). A task
// delivered into another conversation, or one nobody pinned, is not taken
// over: the new run never saw it.
//
// The earlier runs' records are swept only once the new run knows its
// conversation, and has taken over what belongs to it.

// resumeWindow is how long, from when a task was read, a run that ended
// still owes it: a resume of its conversation may take it over until then.
// Past it the task is lost for good — its sender reads that no report is
// coming, a resume no longer takes it over, and main lets its grant go — so
// that a sender told it is lost can send it again without the work being done
// twice. A day, as long as finished mail is kept. Counted per task, not from
// when the wait began: a wait gathers what is read until the next report, and
// a task read late into a long wait is owed as long as one read first.
const resumeWindow = keepFinished

// owedStands says whether a run's wait still owes the message id: it names
// it, and the run is running or ended within the resume window of reading it.
func owedStands(run string, waiter Waiter, id string) bool {
	index := slices.Index(waiter.Messages, id)
	if index < 0 {
		return false
	}
	if registry.EpochAlive(run) {
		return true
	}
	return time.Since(time.Unix(0, waiter.readAt(index))) < resumeWindow
}

// mayResume says whether a message a run that ended still owes can yet be
// reported on by a run resuming its conversation: it was delivered into one,
// and the wait naming it stands.
func mayResume(dir string, message Message, run string, wait Waiter) (bool, error) {
	thread, err := deliveryThread(dir, message.To, message.ID)
	if err != nil {
		return false, err
	}
	return thread != "" && owedStands(run, wait, message.ID), nil
}

// AdoptWaits moves to this run the waits of earlier runs of the name for the
// messages delivered into thread, as if this run had read them when the
// earlier one did. It returns the ids taken over. The caller holds the mailbox
// lock. An error says a wait or its thread could not be read, or a wait taken
// over could not be written: the earlier runs' records must then stay, since
// sweeping them would lose what they owe.
//
// An earlier run's turn end left unfinished is completed first: its reports
// are out, and a wait they answered, taken over, would be answered again.
func AdoptWaits(dir, name, epoch, thread string) ([]string, error) {
	if err := Reconcile(context.Background(), dir, name); err != nil {
		return nil, err
	}
	if thread == "" {
		return nil, nil
	}
	runs, err := os.ReadDir(state.AwaitingPath(dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var adopted []string
	for _, run := range runs {
		if !run.IsDir() || run.Name() == epoch {
			continue
		}
		waiters, err := ReadWaiters(dir, name, run.Name())
		if err != nil {
			return adopted, err
		}
		for _, waiter := range waiters {
			for index, id := range waiter.Messages {
				if !safeID(id) || !owedStands(run.Name(), waiter, id) || slices.Contains(adopted, id) {
					continue
				}
				delivered, err := deliveryThread(dir, name, id)
				if err != nil {
					return adopted, err
				}
				if delivered != thread {
					continue
				}
				// Read when the earlier run read it: taking it over does not
				// start the window again, or a chain of resumes would keep a
				// task, and its grant, owed for ever.
				if err := markScopedAwaiting(dir, name, epoch, Message{ID: id, From: waiter.Name, FromEpoch: waiter.Epoch}, waiter.readAt(index)); err != nil {
					return adopted, err
				}
				adopted = append(adopted, id)
			}
		}
	}
	return adopted, nil
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
		// Not followed yet: the next pass tries again, and the earlier runs'
		// waits stay until what they owe here has been taken over.
		if _, err := AdoptWaits(s.Dir, s.Name, s.Epoch, thread); err != nil {
			return err
		}
		sweepAwaiting(s.Dir, s.Name, s.Epoch)
		return nil
	}) == nil {
		s.followed = true
	}
}

// adoptedBy finds the run of the recipient that took over a message's wait
// from the run it was written for: the report will come from that run.
func adoptedBy(dir string, message Message) (string, bool, error) {
	runs, err := os.ReadDir(state.AwaitingPath(dir, message.To))
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	for _, run := range runs {
		if !run.IsDir() || run.Name() == message.ToEpoch {
			continue
		}
		waiters, err := ReadWaiters(dir, message.To, run.Name())
		if err != nil {
			return "", false, err
		}
		for _, waiter := range waiters {
			if waiter.Name == message.From && waiter.Epoch == message.FromEpoch && slices.Contains(waiter.Messages, message.ID) {
				return run.Name(), true, nil
			}
		}
	}
	return "", false, nil
}
