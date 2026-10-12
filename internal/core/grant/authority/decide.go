package authority

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// The decision on a report whose outcome is unknown, verified, is made by the
// wrapper of the run the report was addressed to, whatever its role
// (docs/v2/stage3-decision.md#authority, Q2 of
// docs/v2/stage3-decision-build.md). A command below that wrapper asks it over
// the run's address; the wrapper checks who asks and runs the decision's final
// transaction itself, through the handler its host supplies, so no verified
// decision is written after the run that vouches for it ended. The command
// holds no mailbox lock while it asks.

const opDecide = "decide"

// DecideHandler runs the decision's final transaction in the wrapper. The
// host supplies it; it answers for its own run only.
type DecideHandler func(ctx context.Context, ask mail.DecideRequest) (mail.DecideAnswer, error)

// decideLimit bounds one decision's exchange: the sender's lock wait, the
// recipient's, the record, its installation and one barrier, with room to
// spare. The handler runs on a shorter bound than the asker waits, so a
// transaction the asker gave up on is rare and is answered uncertain.
const (
	decideLimit  = 60 * time.Second
	handlerLimit = 45 * time.Second
	// maxDecisions bounds the transactions one wrapper runs at once; one
	// past it is refused busy, never queued behind the others' locks.
	maxDecisions = 4
)

// decideAsk is a decision asked for: what the command found, and nothing of
// who decides, which the wrapper says itself.
type decideAsk struct {
	Sender string       `json:"sender"`
	Report string       `json:"report"`
	Choice string       `json:"choice"`
	Expect mail.Subject `json:"expect"`
}

// decided is the answer without the record's observations, which can be
// large: the record is on disk at its path for whoever needs it whole.
type decided struct {
	ID       string               `json:"id,omitempty"`
	Choice   string               `json:"choice,omitempty"`
	Subject  mail.Subject         `json:"subject"`
	By       mail.DecidedBy       `json:"by"`
	Path     string               `json:"path,omitempty"`
	Before   bool                 `json:"before,omitempty"`
	Barrier  string               `json:"barrier,omitempty"`
	Owed     []string             `json:"owed,omitempty"`
	Outcome  mail.DecisionOutcome `json:"outcome,omitempty"`
	Recorded bool                 `json:"recorded,omitempty"`
	Error    string               `json:"error,omitempty"`
}

// ListenDecisions binds the address of a run that holds no grants: every
// wrapper answers for the decisions on reports addressed to its run, and only
// a main's for grants (Listen).
func ListenDecisions(address string, self int) (*Authority, error) {
	a, err := Listen(address, self, 0)
	if err == nil {
		a.grants = false
	}
	return a, err
}

// HandleDecide installs the host's handler. Until it is installed, and once
// the authority is closed, a decision asked for is refused: the run cannot
// answer for one.
func (a *Authority) HandleDecide(handler DecideHandler) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.decide = handler
}

// admitDecision takes one more decision unless the authority is closing, has
// no handler or runs as many as it takes at once.
func (a *Authority) admitDecision() (DecideHandler, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	select {
	case <-a.done:
		return nil, errors.New("this session's wrapper is shutting down and takes no decision")
	default:
	}
	switch {
	case a.decide == nil:
		return nil, errors.New("this session's wrapper does not take decisions yet")
	case a.deciding >= maxDecisions:
		return nil, fmt.Errorf("this session's wrapper is making %d decisions already; ask again", maxDecisions)
	}
	a.deciding++
	return a.decide, nil
}

func (a *Authority) decisionDone() {
	a.mu.Lock()
	a.deciding--
	a.mu.Unlock()
}

// answerDecide runs one decision for a command below this wrapper.
func (a *Authority) answerDecide(conn *net.UnixConn, ask *decideAsk) decided {
	refuse := func(outcome mail.DecisionOutcome, err error) decided {
		return decided{Outcome: outcome, Error: err.Error()}
	}
	if err := a.fromBelow(conn); err != nil {
		return refuse(mail.DecisionUnauthorized, err)
	}
	if ask == nil {
		return refuse(mail.DecisionNoSubject, errors.New("a decision names its report and its choice"))
	}
	handler, err := a.admitDecision()
	if err != nil {
		return refuse(mail.DecisionBusy, err)
	}
	defer a.decisionDone()
	_ = conn.SetDeadline(time.Now().Add(decideLimit))
	ctx, cancel := context.WithTimeout(context.Background(), handlerLimit)
	defer cancel()
	answer, err := handler(ctx, mail.DecideRequest{Sender: ask.Sender, Report: ask.Report, Choice: ask.Choice, Expect: ask.Expect})
	if err != nil {
		var refusal *mail.DecisionError
		if !errors.As(err, &refusal) {
			return refuse(mail.DecisionUncertain, err)
		}
		return decided{Outcome: refusal.Outcome, Recorded: refusal.Recorded, Path: refusal.Path, Error: refusal.Error()}
	}
	state.Step("decision answered")
	d := answer.Decision
	reply := decided{ID: d.ID, Choice: d.Choice, Subject: d.Subject, By: d.By, Path: answer.Path, Before: answer.Before, Owed: answer.Owed}
	if answer.Barrier != nil {
		reply.Barrier = answer.Barrier.Error()
	}
	return reply
}

// Run is the wrapper a decision is asked of: the subject's recipient run as
// its epoch names it.
type Run struct {
	PID   int
	Start uint64
	Boot  string
}

// RunOf reads the wrapper an epoch names.
func RunOf(epoch string) (Run, bool) {
	pid, start, boot, ok := registry.ParseRun(epoch)
	return Run{PID: pid, Start: start, Boot: boot}, ok
}

// Decide asks the wrapper of the subject's recipient run to make a decision,
// verified, and answers what it made. The caller must run below that
// wrapper; the wrapper must be the process the run names — its user, pid,
// start, boot and namespaces, read from the connection, not from what it
// answers. A refusal is a *mail.DecisionError; a wrapper that could not be
// asked is ErrUnreachable, and one that may have decided without the answer
// arriving is uncertain, naming the record that may exist.
func Decide(address string, run Run, ask mail.DecideRequest, path string) (mail.DecideAnswer, error) {
	refuse := func(outcome mail.DecisionOutcome, format string, args ...any) error {
		return &mail.DecisionError{Outcome: outcome, Err: fmt.Errorf(format, args...)}
	}
	boot, err := registry.CurrentBoot()
	if err != nil {
		return mail.DecideAnswer{}, refuse(mail.DecisionUnreadable, "this machine's boot cannot be read: %v", err)
	}
	if run.Boot != boot || run.PID <= 0 {
		return mail.DecideAnswer{}, refuse(mail.DecisionUnauthorized, "run %d of another boot has ended and decides nothing", run.PID)
	}
	if err := proc.Default.Descends(os.Getpid(), run.PID); err != nil {
		return mail.DecideAnswer{}, refuse(mail.DecisionUnauthorized, "this process does not run below the wrapper of the run the report was addressed to: %v", err)
	}
	conn, err := dial(address)
	if err != nil {
		return mail.DecideAnswer{}, err
	}
	defer func() { _ = conn.Close() }()
	if err := (Expect{PID: run.PID, Start: run.Start}).answeredBy(conn); err != nil {
		return mail.DecideAnswer{}, err
	}
	_ = conn.SetDeadline(time.Now().Add(decideLimit))
	state.Step("decision asked")
	// The answer is the checked wrapper's: the connection's peer is fixed when
	// it is made, and the listener's socket is no child's to inherit.
	answer, err := exchange(conn, request{Op: opDecide, Decide: &decideAsk{Sender: ask.Sender, Report: ask.Report, Choice: ask.Choice, Expect: ask.Expect}})
	if err != nil {
		if errors.Is(err, ErrNotConfirmed) {
			return mail.DecideAnswer{}, &mail.DecisionError{Outcome: mail.DecisionUncertain, Path: path, Err: fmt.Errorf("the wrapper's answer does not read, so whether it decided is unknown: %w", err)}
		}
		return mail.DecideAnswer{}, &mail.DecisionError{Outcome: mail.DecisionUncertain, Path: path, Err: fmt.Errorf("the question went out and no answer came, so whether the wrapper decided is unknown: %w", err)}
	}
	reply := answer.Decided
	switch {
	case answer.Error != "":
		// The wrapper did not take the question at all: nothing was decided.
		return mail.DecideAnswer{}, refuse(mail.DecisionUnreadable, "the wrapper did not take the question: %s", answer.Error)
	case reply == nil:
		return mail.DecideAnswer{}, &mail.DecisionError{Outcome: mail.DecisionUncertain, Path: path, Err: errors.New("the wrapper answered no decision")}
	case reply.Outcome != "":
		return mail.DecideAnswer{}, &mail.DecisionError{Outcome: reply.Outcome, Recorded: reply.Recorded, Path: reply.Path, Err: errors.New(reply.Error)}
	}
	made := mail.DecideAnswer{
		Decision: mail.Decision{ID: reply.ID, Choice: reply.Choice, Subject: reply.Subject, By: reply.By},
		Path:     reply.Path, Before: reply.Before, Owed: reply.Owed,
	}
	if reply.Barrier != "" {
		made.Barrier = errors.New(reply.Barrier)
	}
	return made, nil
}
