package mainhost

import (
	"fmt"

	"github.com/praline-labs/rewake/internal/infra/parts"
)

// evKind is what reached the loop.
type evKind int

const (
	kHello        evKind = iota // a shell said hello
	kRefused                    // a shell's hello was refused
	kMsg                        // a shell's message
	kExit                       // a shell exited
	kHelloLate                  // a shell's hello bound passed
	kReadyLate                  // a shell's ready bound passed
	kReleaseLate                // a released shell's exit bound passed
	kFirstWait                  // the bound for the first shell's colors passed
	kCrashCheck                 // a crash settled
	kDoor                       // a restart asked at the room's door
	kParentGone                 // the host's parent left
	kRegistered                 // main's session is registered and its stream served
	kMainExited                 // main's wrapper exited
	kMark                       // main's closing mark went by
	kEndLate                    // the shell's bound to finish after main passed
	kTerminalGone               // main alone met the end of the person's terminal
	kReloadLate                 // a reload asked of the shell had no answer in time
)

// ev is one event; the kind says which fields it carries.
type ev struct {
	kind  evKind
	gen   int
	msg   Message
	why   string
	reply chan Message
	code  int
	ref   *MainRef
}

// handle takes one event; true ends the host.
func (h *Host) handle(e ev) bool {
	switch e.kind {
	case kHello:
		h.hello(e.gen)
	case kRefused:
		if p := h.byGen(e.gen); p != nil {
			p.refused = e.why
			p.kill()
		}
	case kMsg:
		return h.message(e.gen, e.msg)
	case kExit:
		return h.exited(e.gen)
	case kHelloLate:
		if p := h.byGen(e.gen); p != nil && !p.hello {
			p.refused = "the new shell said nothing within " + helloWithin.String()
			p.kill()
		}
	case kReadyLate:
		if p := h.byGen(e.gen); p != nil && p.given && !p.ready {
			p.kill()
		}
	case kReleaseLate:
		if p := h.byGen(e.gen); p != nil && p.releasing {
			p.kill()
		}
	case kFirstWait:
		if h.main == nil && h.phase == starting {
			return h.startMain()
		}
	case kCrashCheck:
		return h.act(decide(evCrash, h.situation()), "the shell stopped")
	case kDoor:
		if e.msg.T == kindParts {
			h.running(e.reply)
		} else {
			h.restart(e.msg, e.reply)
		}
	case kReloadLate:
		if p := h.byGen(e.gen); p != nil && p == h.shell {
			h.reloadLate()
		}
	case kParentGone:
		return h.act(decide(evLost, h.situation()), "the window closed")
	case kTerminalGone:
		h.lost = true
		return h.act(decide(evLost, h.situation()), e.why)
	case kRegistered:
		h.ref = e.ref
		for _, p := range []*shellProc{h.shell, h.candidate} {
			if p != nil && p.hello {
				p.send(Message{T: kindMain, Main: h.ref})
			}
		}
	case kMainExited:
		h.code, h.ending = e.code, true
		h.main.exited = true
		return h.act(decide(evMainEnded, h.situation()), "main ended")
	case kMark:
		h.ending = true
	case kEndLate:
		return true
	}
	return false
}

// hello welcomes a shell, and gives it the terminal when it is the one to
// have it; a restart's new shell waits until the old one has let go.
func (h *Host) hello(gen int) {
	p := h.byGen(gen)
	if p == nil {
		return
	}
	p.hello = true
	p.send(Message{T: kindWelcome, Generation: p.gen, Why: p.why, Main: h.ref})
	switch {
	case p == h.shell && (h.phase == starting || h.phase == recovering):
		h.give(p, h.phase != starting)
	case p == h.candidate && h.shell != nil:
		h.shell.releasing = true
		h.shell.send(Message{T: kindRelease})
		h.after(releaseWithin, ev{kind: kReleaseLate, gen: h.shell.gen})
	case p == h.candidate:
		h.handOver()
	}
}

// give sends a shell go: from now it has the person's terminal, and has to
// draw within its bound. Keys typed while no shell had them are dropped,
// not queued for the new one.
func (h *Host) give(p *shellProc, flush bool) {
	if flush {
		h.flushInput()
	}
	p.given = true
	p.send(Message{T: kindGo, State: h.released})
	h.released = nil
	h.after(readyWithin, ev{kind: kReadyLate, gen: p.gen})
}

// handOver makes a restart's new shell the one, once the old has let go.
func (h *Host) handOver() {
	h.solo.leave()
	h.solo = nil
	h.gap("restarting rewake's shell…")
	h.shell, h.candidate = h.candidate, nil
	h.give(h.shell, true)
}

// message takes a shell's message; true ends the host.
func (h *Host) message(gen int, msg Message) bool {
	p := h.byGen(gen)
	if p == nil {
		return false
	}
	switch msg.T {
	case kindReady:
		if p.ready || !p.given {
			return false
		}
		p.ready = true
		switch h.phase {
		case starting:
			h.colors = msg.Colors
			if msg.Cols > 0 && msg.Rows > 0 {
				h.size.Cols, h.size.Rows = msg.Cols, msg.Rows
			}
			h.phase = running
			if h.main == nil {
				return h.startMain()
			}
		case recovering:
			h.phase = running
		case swapping:
			h.phase = running
			h.answer(Message{T: kindDone, Generation: p.gen})
		}
	case kindRestart:
		h.restart(msg, nil)
	case kindClose:
		p.said = kindClose
		return h.act(decide(evClose, h.situation()), "the shell asked to close")
	case kindLost:
		p.said, h.lost = kindLost, true
		return h.act(decide(evLost, h.situation()), "the shell lost the terminal")
	case kindReleased:
		p.said, h.released = kindReleased, msg.State
	case kindReloaded:
		h.reloaded(Message{T: kindDone, Generation: p.gen, Part: msg.Part})
	default:
		p.send(Message{T: kindError, Why: fmt.Sprintf("no message %q in version %d", msg.T, Version)})
	}
	return false
}

// restart takes a restart asked for, by the shell's key (reply nil) or at the
// door.
func (h *Host) restart(msg Message, reply chan Message) {
	if msg.Part == parts.Face {
		h.reload(msg.Part, reply)
		return
	}
	if msg.Part != "" && msg.Part != parts.ShellPart {
		answer(reply, Message{T: kindFailed, Why: fmt.Sprintf("no part %q to restart here: the host restarts the shell and reloads the face", msg.Part)})
		return
	}
	if decide(evRestart, h.situation()) != doSwap {
		answer(reply, Message{T: kindFailed, Why: h.whyNot()})
		return
	}
	if reply != nil {
		h.waiting = append(h.waiting, reply)
	}
	back := h.phase
	h.phase = swapping
	p, err := h.startShell("requested")
	if err != nil {
		h.phase = back
		h.answer(Message{T: kindFailed, Why: err.Error() + "; the running shell stays"})
		return
	}
	h.candidate = p
}

// whyNot says why a restart is not taken now.
func (h *Host) whyNot() string {
	switch {
	case h.phase == closing:
		return "rewake is closing"
	case h.ending:
		return "main is ending"
	case h.phase == swapping:
		return "a restart is under way"
	}
	return "the shell is starting"
}

// exited takes a shell's exit; true ends the host.
func (h *Host) exited(gen int) bool {
	p := h.byGen(gen)
	if p == nil {
		return false
	}
	if p == h.candidate {
		// The new shell left before it had the terminal: the old one keeps it.
		h.candidate = nil
		if h.phase == swapping {
			why := p.refused
			if why == "" {
				why = "the new shell exited before it said hello"
				if words := p.lastWords(); words != "" {
					why += ": " + words
				}
			}
			switch {
			case h.solo != nil:
				h.phase = alone
				h.answer(Message{T: kindFailed, Why: why + "; main stays alone in the terminal"})
			case h.shell == nil || h.shell.releasing:
				// The old shell has let go already: nothing draws.
				return h.act(doAlone, why)
			default:
				h.phase = running
				h.answer(Message{T: kindFailed, Why: why + "; the running shell stays"})
			}
		}
		return false
	}
	h.shell = nil
	switch {
	case h.phase == closing:
		// Main's end — the hangup, SIGTERM, SIGKILL — runs in the host, so
		// the host outlives a shell gone with the window until main has ended.
		return h.main == nil || h.main.exited
	case p.said == kindClose || p.said == kindLost:
		return false
	case p.releasing || h.phase == swapping && p.ready:
		// The old shell let go, or died while the new one was starting.
		if !p.releasing {
			h.dirty = true
		}
		if h.candidate != nil && h.candidate.hello {
			h.handOver()
		}
		return false
	case !p.ready:
		h.dirty = true
		why := "the shell did not start"
		if words := p.lastWords(); words != "" {
			why += ": " + words
		}
		return h.act(decide(evNotReady, h.situation()), why)
	}
	h.dirty = true
	h.after(crashSettle, ev{kind: kCrashCheck})
	return false
}

// answer gives every restart waiting its outcome.
func (h *Host) answer(msg Message) {
	for _, reply := range h.waiting {
		answer(reply, msg)
	}
	h.waiting = nil
}

func answer(reply chan Message, msg Message) {
	if reply != nil {
		reply <- msg
	}
}

// act does what decide said; true ends the host.
func (h *Host) act(a action, why string) bool {
	switch a {
	case doRecover:
		h.crashed = true
		h.phase = recovering
		h.gap("the shell stopped; starting it again…")
		p, err := h.startShell("crashed")
		if err != nil {
			return h.act(doClose, err.Error())
		}
		h.shell = p
	case doAlone:
		h.candidate.kill()
		h.shell.kill()
		h.candidate, h.shell = nil, nil
		h.phase = alone
		h.answer(Message{T: kindFailed, Why: why + "; main runs alone in the terminal"})
		h.enterAlone(why)
	case doClose, doCloseLost:
		return h.close(a == doCloseLost)
	case doEnd:
		return h.end()
	}
	return false
}

// close ends main and with it its workers: the shell shows its closing,
// main hears its terminal hang up, then the bound, SIGTERM and SIGKILL.
func (h *Host) close(lost bool) bool {
	h.lost = h.lost || lost
	if h.phase == closing {
		return h.lost && (h.main == nil || h.main.exited)
	}
	h.phase = closing
	h.answer(Message{T: kindFailed, Why: "rewake is closing"})
	h.candidate.kill()
	if h.solo != nil {
		h.solo.leave()
		h.solo = nil
		h.gap("closing rewake…")
	}
	if !h.lost && h.shell != nil && h.shell.given {
		h.shell.send(Message{T: kindClosing})
	}
	if h.main == nil {
		h.shell.kill()
		return true
	}
	go h.main.end()
	return false
}

// end follows main's end: the shell finishes its closing — the workers
// seen closed — and leaves, within a bound.
func (h *Host) end() bool {
	h.phase = closing
	h.answer(Message{T: kindFailed, Why: "main ended"})
	h.candidate.kill()
	if h.lost || h.shell == nil || !h.shell.ready {
		h.shell.kill()
		return true
	}
	code := h.code
	h.shell.send(Message{T: kindMainEnded, Code: &code})
	h.after(endWait, ev{kind: kEndLate})
	return false
}
