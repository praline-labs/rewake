package control

import (
	"context"
	"sync"
	"time"
)

// The wrapper's half of an interrupt, the same for every harness whose agent
// only stops the turn (docs/remote-control.md#interrupt): who stopped which
// turn is recorded before the command goes out, so an end that comes at once
// finds it; the end heard on the mail path settles it; and the actor's words
// replace that end's answer and go into the agent's history as its trace.
// The agent keeps the native abort and its check that the named turn is its
// running one.

// Attribution is who interrupted which turn, kept until that turn ends or
// attributionLife passes, and handed to the wrapper's next image with the run.
type Attribution struct {
	Request string    `json:"request"`
	From    string    `json:"from"`
	Turn    string    `json:"turn"`
	At      time.Time `json:"at"`
}

// Words of an interrupt's settling, the same whichever harness answered.
const (
	NotSettled = "the turn has not settled after interrupt"
	TraceLost  = "the turn stopped, but its interruption trace could not be recorded"
)

// SettleWait is how long an interrupt waits for its turn's end.
const SettleWait = 3 * time.Second

// attributionLife bounds an attribution whose turn never ends; maxAttributions
// bounds how many are kept, the oldest going first.
const (
	attributionLife = time.Hour
	maxAttributions = 8
)

// Actor is what an interrupted turn's report and the agent's history say of
// it: the asking main's name.
func Actor(from string) string { return from + " interrupted this turn with rewake interrupt" }

// Interrupts are a run's attributions and the answers their askers wait on.
type Interrupts struct {
	mu      sync.Mutex
	kept    []Attribution
	waiting map[string]chan Answer
}

// Record attributes turn's interrupt to request before it is dispatched.
func (i *Interrupts) Record(request Request, turn string, now time.Time) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.pruneLocked(now)
	if len(i.kept) == maxAttributions {
		i.dropLocked(i.kept[0].Request)
	}
	i.kept = append(i.kept, Attribution{Request: request.ID, From: request.From, Turn: turn, At: now})
	if i.waiting == nil {
		i.waiting = map[string]chan Answer{}
	}
	i.waiting[request.ID] = make(chan Answer, 1)
}

// Forget drops request's attribution: the agent refused or failed it, and
// no end of its turn is the interrupt's.
func (i *Interrupts) Forget(request string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.dropLocked(request)
	delete(i.waiting, request)
}

// Wait is request's final answer, or an open failure when its turn has not
// ended by deadline or the wait was cut short; the attribution stays, so a
// later end still carries the actor and its trace.
func (i *Interrupts) Wait(ctx context.Context, request string, deadline time.Time, stop <-chan struct{}) Answer {
	i.mu.Lock()
	answer, ok := i.waiting[request]
	i.mu.Unlock()
	open := Answer{Outcome: Failed, Reason: CutShort, Detail: NotSettled, Open: true}
	if !ok {
		return open
	}
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	defer func() {
		i.mu.Lock()
		delete(i.waiting, request)
		i.mu.Unlock()
	}()
	select {
	case final := <-answer:
		return final
	case <-timer.C:
	case <-stop:
	case <-ctx.Done():
	}
	return open
}

// Ended matches a turn's end heard on the mail path. An aborted turn that was
// interrupted answers its actor, and settled, to be called once the trace was
// put into the agent's history or could not be; one that ended otherwise
// answers its asker that no turn was running. A turn nobody interrupted
// answers "".
func (i *Interrupts) Ended(turn string, aborted bool, now time.Time) (actor string, settled func(traced bool)) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.pruneLocked(now)
	for _, kept := range i.kept {
		if kept.Turn != turn {
			continue
		}
		answer := i.waiting[kept.Request]
		i.dropLocked(kept.Request)
		give := func(final Answer) {
			if answer != nil {
				select {
				case answer <- final:
				default:
				}
			}
		}
		if !aborted {
			give(Answer{Outcome: Refused, Reason: NoTurn})
			return "", nil
		}
		return Actor(kept.From), func(traced bool) {
			if traced {
				give(Answer{Outcome: Done})
				return
			}
			give(Answer{Outcome: Failed, Detail: TraceLost})
		}
	}
	return "", nil
}

// Handed are the attributions to hand to the next image; the answers waited
// on stay with this one, whose askers get an open failure.
func (i *Interrupts) Handed() []Attribution {
	i.mu.Lock()
	defer i.mu.Unlock()
	return append([]Attribution(nil), i.kept...)
}

// Adopt takes the attributions an earlier image handed over.
func (i *Interrupts) Adopt(kept []Attribution) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.kept = append(i.kept[:0], kept...)
}

func (i *Interrupts) pruneLocked(now time.Time) {
	for len(i.kept) > 0 && now.Sub(i.kept[0].At) > attributionLife {
		i.dropLocked(i.kept[0].Request)
	}
}

func (i *Interrupts) dropLocked(request string) {
	for n, kept := range i.kept {
		if kept.Request == request {
			i.kept = append(i.kept[:n], i.kept[n+1:]...)
			break
		}
	}
}
