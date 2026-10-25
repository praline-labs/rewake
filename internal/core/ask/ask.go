// Package ask holds a worker's permission requests and questions while main
// answers them (docs/permission-requests.md). A request lives in the memory
// of the worker's wrapper only: it is opened by the harness's side, waited on
// by its polls, answered by main through `rewake answer`, and settled by the
// harness's side with whichever answer came first — main's, the person's at
// the worker's screen, or the deadline. A request lost to a restart is asked
// again by the harness; nothing of it is written down.
package ask

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

// The kinds of a request.
const (
	Permission = "permission"
	Question   = "question"
)

// Lifetime is how long a request waits for an answer from its opening.
const Lifetime = 120 * time.Second

// Offer is what a request shows its answerer.
type Offer struct {
	Kind string
	// Tool, Input and Cwd are a permission's call: the tool, its arguments
	// as text, and the directory the worker runs in.
	Tool, Input, Cwd string
	// ReadOnly says the harness's check called the call read-only. It is
	// the tool's word, not proof: a read outside the project can still
	// expose keys and logins, so main still answers.
	ReadOnly bool
	// Questions are what the harness's question dialog shows; a permission
	// has one, whose options are the permission's choices.
	Questions []Prompt
}

// Prompt is one question with its options.
type Prompt struct {
	Text, Header string
	Multi        bool
	Options      []Option
}

// Option is one choice of a question.
type Option struct {
	Label, Description string
}

// Answer is main's answer: per question its text — a label, labels joined
// with ", ", or the text typed under Other — and for a permission the reason
// a refusal gives the worker's model.
type Answer struct {
	Answers map[string]string
	Reason  string
}

// The winners a request is settled with.
const (
	Main    = "main"
	Local   = "local"
	Expired = "expired"
	Aborted = "aborted"
)

// Outcome is what came of main's answer: Accepted once the harness took it,
// or the winner that came first.
type Outcome struct {
	Accepted bool
	Winner   string
}

// Poll is one poll's answer: an answer to give the harness, the request
// settled or expired, or nothing yet.
type Poll struct {
	Answer  *Answer
	Settled bool
	Expired bool
}

// The refusals of the broker.
var (
	ErrUnknown  = errors.New("no request of this run has that id: it was settled, expired or never opened")
	ErrAnswered = errors.New("the request has an answer already")
	ErrLoad     = errors.New("the request belongs to another load of the harness's side")
)

type request struct {
	id       string
	load     string
	offer    Offer
	deadline time.Time
	proposal *Answer
	// wake is closed when the request changes: a proposal, its settling.
	wake    chan struct{}
	settled string
	// outcome hears the settling of a proposal main made.
	outcome chan Outcome
}

// Broker holds a run's open requests.
type Broker struct {
	mu       sync.Mutex
	requests map[string]*request
	// now is the clock, replaced in tests.
	now func() time.Time
}

// NewBroker is an empty broker.
func NewBroker() *Broker {
	return &Broker{requests: map[string]*request{}, now: time.Now}
}

// Open takes a request of the harness's load in and answers its id and
// deadline.
func (b *Broker) Open(load string, offer Offer) (string, time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	// A request whose harness never settled it — a load replaced, a crash —
	// goes a while after its deadline, so no poll is cut short by its going.
	for id, r := range b.requests {
		if b.now().After(r.deadline.Add(time.Minute)) {
			delete(b.requests, id)
		}
	}
	id := newID()
	for b.requests[id] != nil {
		id = newID()
	}
	deadline := b.now().Add(Lifetime)
	b.requests[id] = &request{id: id, load: load, offer: offer, deadline: deadline, wake: make(chan struct{})}
	return id, deadline
}

// Drop forgets a request that was opened and never shown: its letter could
// not be written.
func (b *Broker) Drop(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.requests, id)
}

// Offer answers what a request shows, for main's answer to be read against.
func (b *Broker) Offer(id string) (Offer, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	r := b.requests[id]
	if r == nil {
		return Offer{}, ErrUnknown
	}
	return r.offer, nil
}

// Poll waits up to hold for main's answer to a request of load, its settling
// or its deadline.
func (b *Broker) Poll(ctx context.Context, load, id string, hold time.Duration) (Poll, error) {
	b.mu.Lock()
	r := b.requests[id]
	if r == nil {
		b.mu.Unlock()
		return Poll{}, ErrUnknown
	}
	if r.load != load {
		b.mu.Unlock()
		return Poll{}, ErrLoad
	}
	if poll, done := b.pollLocked(r); done {
		b.mu.Unlock()
		return poll, nil
	}
	wake := r.wake
	left := r.deadline.Sub(b.now())
	b.mu.Unlock()
	timer := time.NewTimer(min(hold, max(left, 0)))
	defer timer.Stop()
	select {
	case <-wake:
	case <-timer.C:
	case <-ctx.Done():
		return Poll{}, ctx.Err()
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	poll, _ := b.pollLocked(r)
	return poll, nil
}

func (b *Broker) pollLocked(r *request) (Poll, bool) {
	switch {
	case r.settled != "":
		return Poll{Settled: true}, true
	case r.proposal != nil:
		return Poll{Answer: r.proposal}, true
	case !b.now().Before(r.deadline):
		return Poll{Expired: true}, true
	}
	return Poll{}, false
}

// Propose gives a request main's answer, the first one only, and answers the
// channel that hears what came of it once the harness settles the request.
func (b *Broker) Propose(id string, answer Answer) (<-chan Outcome, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	r := b.requests[id]
	switch {
	case r == nil:
		return nil, ErrUnknown
	case r.settled != "":
		return nil, ErrUnknown
	case r.proposal != nil:
		return nil, ErrAnswered
	case !b.now().Before(r.deadline):
		return nil, ErrUnknown
	}
	r.proposal = &answer
	r.outcome = make(chan Outcome, 1)
	close(r.wake)
	r.wake = make(chan struct{})
	return r.outcome, nil
}

// Settle ends a request of load with the winner the harness's side chose,
// telling main's answer what came of it.
func (b *Broker) Settle(load, id, winner string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	r := b.requests[id]
	if r == nil {
		return ErrUnknown
	}
	if r.load != load {
		return ErrLoad
	}
	r.settled = winner
	close(r.wake)
	delete(b.requests, id)
	if r.outcome != nil {
		r.outcome <- Outcome{Accepted: winner == Main, Winner: winner}
	}
	return nil
}

// newID is a request's id: short, since main types it.
func newID() string {
	var raw [2]byte
	_, _ = rand.Read(raw[:])
	return hex.EncodeToString(raw[:])
}
