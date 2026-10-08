package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/praline-labs/rewake/internal/sessionstate"
)

// LaunchIntent is what the launch asked the terminal to open. A launch that
// resumes names its conversation by id, or leaves it to be chosen — --last, a
// name, the picker — and then the terminal's first resume names it. A fresh
// launch and a fork carry none: a fork has its own proof (StartupFork).
type LaunchIntent struct {
	Resume bool
	// Thread is the conversation named by id; empty until the terminal's
	// first resume names one.
	Thread string
}

// ErrUnintended refuses a delivery into a conversation the launch did not ask
// for: its resume failed or was left, and the terminal went on in another one,
// or in none. Not a failure: the delivery waits until the intended
// conversation is selected or the person accepts the one that is
// (Gateway.Accept).
var ErrUnintended = errors.New("the selected conversation is not the one the launch asked to resume")

// HoldError carries why a delivery waits, for the refusal the sender reads.
type HoldError struct{ Hold sessionstate.DeliveryHold }

func (e *HoldError) Error() string { return e.Hold.Detail }
func (e *HoldError) Unwrap() error { return ErrUnintended }

// Accept's refusals.
var (
	ErrNothingHeld = errors.New("deliveries are not held for the launch's conversation")
	ErrNotSelected = errors.New("not the selected conversation")
	ErrNoSelection = errors.New("no conversation is selected for delivery")
)

// intent keeps the launch's conversation until it is resumed once, or the
// person accepts another; after that, the terminal's ordinary navigation
// selects (docs/delivery-conversation.md). It lives on the gateway, not on a
// connection, so a reconnect does not forget it. Its lock is a leaf: nothing
// else is taken while it is held.
type intent struct {
	mu sync.Mutex
	// waiting until the first successful resume of thread, or an acceptance.
	// It only ever turns false.
	waiting bool
	thread  string
	// refused is the server's answer to the terminal's resume of thread when
	// it selected nothing: refused, or open without the right to write.
	refused string
	// admitErr is why the last attempt to open the session's mail failed;
	// nil when none failed or the wait is over.
	admitErr error
}

// pin names the intended conversation by the terminal's first primary
// resume, before its reply: a resume refused still names what was asked for.
func (g *Gateway) pin(m meta) {
	if m.method != "thread/resume" || m.reconnect || isHelper(m) || !recognized(m) {
		return
	}
	g.intent.mu.Lock()
	defer g.intent.mu.Unlock()
	if g.intent.waiting && g.intent.thread == "" {
		g.intent.thread = m.thread
	}
}

// selected lifts the hold once the intended conversation is accepted for
// delivery: from then on the terminal navigates as it always did.
func (g *Gateway) selected(b Binding) {
	g.intent.mu.Lock()
	defer g.intent.mu.Unlock()
	if g.intent.waiting && b.Ready && g.intent.thread != "" && b.Thread == g.intent.thread {
		// A lift not recorded stays a hold, published with its cause; the
		// next reply or publication tries again.
		g.intent.admitErr = g.liftLocked()
	}
}

// liftLocked ends the wait, after recording that the session's mail may be
// read: the record comes first, so no reader is refused after deliveries go,
// and none admitted while they wait. Called under intent.mu.
func (g *Gateway) liftLocked() error {
	if g.cfg.Admit != nil {
		if err := g.cfg.Admit(); err != nil {
			return fmt.Errorf("the session's mail could not be opened: %w", err)
		}
	}
	g.intent.waiting, g.intent.admitErr = false, nil
	return nil
}

// refusedResume records that the terminal's resume of the intended
// conversation selected nothing, and why.
func (g *Gateway) refusedResume(thread, why string) {
	g.intent.mu.Lock()
	defer g.intent.mu.Unlock()
	if g.intent.waiting && thread != "" && thread == g.intent.thread {
		g.intent.refused = why
	}
}

// launchHold says why a binding may not take a delivery while the launch's
// resume is still owed; nil once it is not, and nil while the intended resume
// is only on its way.
func (g *Gateway) launchHold(b Binding) *sessionstate.DeliveryHold {
	g.intent.mu.Lock()
	defer g.intent.mu.Unlock()
	if !g.intent.waiting {
		return nil
	}
	intended := g.intent.thread
	if b.Ready && b.Thread == intended {
		// Selected, and the mail not opened yet: a delivery now would reach a
		// worker whose rewake inbox still refuses.
		detail := "the launch's conversation " + intended + " is selected, and the session's mail is being opened; deliveries wait for it"
		if g.intent.admitErr != nil {
			detail = "the launch's conversation " + intended + " is selected, and " + g.intent.admitErr.Error() + "; deliveries wait while the wrapper tries again, and rewake inbox stays closed until it succeeds"
		}
		return &sessionstate.DeliveryHold{Reason: sessionstate.HoldMailClosed, Expected: intended, Selected: intended, Detail: detail}
	}
	if !b.Ready {
		if g.intent.refused == "" {
			return nil
		}
		return &sessionstate.DeliveryHold{Reason: sessionstate.HoldUnintended, Expected: intended, Detail: "the launch asked to resume " + intended + ", and the server answered: " + g.intent.refused + "; no conversation is selected, and deliveries wait: resume " + intended + " with /resume once nothing else holds it"}
	}
	accept := "rewake accept " + g.cfg.Name + " " + b.Thread
	hold := &sessionstate.DeliveryHold{Reason: sessionstate.HoldUnintended, Expected: intended, Selected: b.Thread}
	if intended == "" {
		hold.Detail = "the launch asked to resume a conversation, and the terminal went on in " + b.Thread + " without resuming one; deliveries wait: resume one with /resume, or have the person accept this one from a shell outside the session: " + accept
	} else {
		hold.Detail = "the launch asked to resume " + intended + ", and the terminal selected " + b.Thread + "; deliveries wait: resume " + intended + " with /resume, or have the person accept " + b.Thread + " from a shell outside the session: " + accept
	}
	return hold
}

// resumeOwed is why a delivery waits for a resume still on its way, which is
// not published as a hold; nil when none is owed.
func (g *Gateway) resumeOwed() *HoldError {
	g.intent.mu.Lock()
	defer g.intent.mu.Unlock()
	if !g.intent.waiting {
		return nil
	}
	intended := g.intent.thread
	if intended == "" {
		intended = "a conversation"
	}
	return &HoldError{Hold: sessionstate.DeliveryHold{Reason: sessionstate.HoldUnintended, Expected: g.intent.thread, Detail: "the launch asked to resume " + intended + ", and the terminal has not selected it yet; deliveries wait for the resume"}}
}

// Hold is why deliveries into the selected conversation wait, nil when
// nothing holds them.
func (g *Gateway) Hold() *sessionstate.DeliveryHold { return g.hold(g.Binding()) }

func (g *Gateway) hold(b Binding) *sessionstate.DeliveryHold { return g.launchHold(b) }

// Accept takes the selected conversation for the launch's own, at the
// person's word. It must be the one selected now: an acceptance of any other
// — one seen before a switch, or in an earlier run — is refused. The check
// and the lift are one step under the connection's and the gateway's locks,
// which every change of the selection takes.
func (g *Gateway) Accept(thread string) error {
	c := g.currentConnection()
	if c == nil {
		return g.acceptNothing()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	g.mu.Lock()
	defer g.mu.Unlock()
	owned := c.ctx.Err() == nil && !g.closed && g.current == c && len(g.owners) == 1
	b := c.state.Binding
	g.intent.mu.Lock()
	defer g.intent.mu.Unlock()
	switch {
	case !g.intent.waiting:
		return ErrNothingHeld
	case !owned || !b.Ready:
		return ErrNoSelection
	case b.Thread != thread:
		return ErrNotSelected
	}
	return g.liftLocked()
}

func (g *Gateway) acceptNothing() error {
	g.intent.mu.Lock()
	defer g.intent.mu.Unlock()
	if !g.intent.waiting {
		return ErrNothingHeld
	}
	return ErrNoSelection
}

// warnHold shows the person at the terminal why deliveries wait, once per
// hold on this connection, as the server's own warning notification: the
// terminal shows one without a turn of the model (tui/src/chatwidget/
// protocol.rs, read in the source of 0.159.0, not seen live). The warning is
// cosmetic: when the terminal has no room for it, main's notice and rewake
// list still say the same. With nothing selected it goes to the intended
// conversation, which the terminal may show read-only.
func (c *connection) warnHold(b Binding, hold *sessionstate.DeliveryHold) {
	thread := b.Thread
	if thread == "" {
		thread = hold.Expected
	}
	if thread == "" {
		return
	}
	key := hold.Reason + "\x00" + hold.Expected + "\x00" + hold.Selected + "\x00" + hold.Detail
	raw, err := json.Marshal(map[string]any{"method": "warning", "params": map[string]any{"threadId": thread, "message": "rewake: " + hold.Detail}})
	if err != nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.owner.mu.Lock()
	defer c.owner.mu.Unlock()
	if c.warned == key || c.ctx.Err() != nil || c.owner.closed || c.owner.current != c || len(c.owner.owners) != 1 || !sameSelection(b, c.state.Binding) {
		return
	}
	if len(c.toUI) >= cap(c.toUI)-4 || c.responseBytes.Load() > transportQueueBytes-(1<<20) {
		return
	}
	if c.queueResponse(raw) {
		c.warned = key
	}
}

// sameSelection is sameBinding for a selection that may hold nothing.
func sameSelection(a, b Binding) bool {
	return a.Epoch == b.Epoch && a.Connection == b.Connection && a.Generation == b.Generation && a.Thread == b.Thread && a.Ready == b.Ready
}
