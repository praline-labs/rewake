package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"
)

// The fixture program's turns: a notice starts one, or is steered into the one
// held open; the turn reads the mail as the other fixtures do; its end goes to
// the adapter, which asks the core, and an end the core holds is continued
// with the core's reason, as the Claude Code shim continues a held Stop — but
// the decision to go on is the product's, asked through the contract.

// fixtureAskBound is how long the program waits for the adapter's answer to a
// turn's start or end; an end waits for the core's decision.
const fixtureAskBound = 40 * time.Second

// delivered takes one notice: steered into the open turn when one is held,
// a turn of its own otherwise.
func (s *fixtureSession) delivered(frame, reply fixtureFrame) fixtureFrame {
	if frame.Thread != s.thread {
		reply.Error = fmt.Sprintf("no such conversation %q", frame.Thread)
		return reply
	}
	var notice mailboxNotice
	notice.Notice = frame.Notice
	if err := json.Unmarshal(frame.Members, &notice.Members); err != nil || len(notice.Members) == 0 || frame.Letter == "" {
		reply.Error = "a notice names its letters"
		return reply
	}
	s.recordDelivered(notice)
	s.turn.mu.Lock()
	if open := s.turn.open; open != "" {
		steered := s.turn.steered
		s.turn.mu.Unlock()
		s.recordSteer(open, frame.Letter)
		s.recordGroup(open, notice)
		s.recordRecalls(open, notice)
		if steered != nil {
			select {
			case <-steered:
			default:
				close(steered)
			}
		}
		reply.OK, reply.Steered = true, true
		return reply
	}
	s.turn.counter++
	id := fmt.Sprintf("turn-%d", s.turn.counter)
	s.turn.lastNotice = notice
	if os.Getenv(shimHoldTurn) != "" {
		s.turn.open = id
		s.turn.steered = make(chan struct{})
		s.turn.abort = make(chan struct{})
	}
	s.turn.mu.Unlock()
	s.recordGroup(id, notice)
	s.recordRecalls(id, notice)
	go s.work(id, notice)
	reply.OK = true
	return reply
}

// work is what the session does with a turn: say it started, read the mail,
// and end it through the adapter.
func (s *fixtureSession) work(id string, notice mailboxNotice) {
	if answer, err := s.ask(fixtureFrame{Op: "turn-started", Turn: id}, fixtureAskBound); err != nil || !answer.OK {
		s.recordTurnEvent("start-refused", id, refusal(answer, err))
	}
	s.reportActivity("working")
	time.Sleep(20 * time.Millisecond)

	var text string
	var err error
	// The calls the scenario asks for: a read through the tool is the turn's
	// read; any other call comes after the shell's, as a task's work would.
	calls := s.toolCalls()
	switch {
	case readsThroughTool(calls):
		text = s.callTools(id, calls)
	case os.Getenv(shimReadEach) != "":
		text, err = s.readEach(notice)
	default:
		text, err = s.readMailbox()
	}
	if err != nil {
		text = "could not read the mailbox: " + err.Error()
	}
	if len(calls) > 0 && !readsThroughTool(calls) {
		s.callTools(id, calls)
	}
	s.recordTurn(id + " " + firstLine(text))
	if held := s.holdOpen(id); held != "" {
		text += "; " + held
	}
	recordOwedOnce()
	markPendingOnce()
	outcome := turnOutcome()
	if s.takeAborted() || s.interruptNow() {
		outcome = "interrupted"
	}
	if reason := os.Getenv(shimEndReason); reason != "" && outcome != "completed" {
		text = reason
	}
	s.end(id, text, outcome)
	s.reportActivity("idle")
	if outcome == "interrupted" {
		s.recordTurn(id + " interrupted")
	}
	s.recordTurnEvent("completed", id, outcome)
	s.dropIfDone()
}

// end hands the turn's end to the adapter, and continues a held end with the
// core's reason until an end is published or the bound is reached — the
// bound the Claude Code shim's harness keeps (maxStopHolds).
func (s *fixtureSession) end(id, text, outcome string) {
	holdable := os.Getenv(shimNoHold) == ""
	for n := 1; ; n++ {
		frame := fixtureFrame{Op: "turn-ended", Turn: id, End: id + "/end-" + strconv.Itoa(n), Outcome: outcome, Text: text, Hold: holdable && n <= maxStopHolds+1}
		answer, err := s.ask(frame, fixtureAskBound)
		if err == nil && answer.OK && n == 1 && os.Getenv(shimDropAnswer) != "" && fixtureDropOnce() {
			// The answer is lost: the same end goes again, under its name,
			// and must be answered the same.
			again, againErr := s.ask(frame, fixtureAskBound)
			same := againErr == nil && again.OK && again.Reason == answer.Reason
			s.recordTurnEvent("answer-lost", id, fmt.Sprintf("same=%v", same))
			answer, err = again, againErr
		}
		if err != nil || !answer.OK {
			s.recordTurnEvent("end-refused", id, refusal(answer, err))
			return
		}
		if answer.Reason == "" {
			return
		}
		s.recordTurn(id + " held: " + answer.Reason)
		if n > maxStopHolds {
			return
		}
		// The next end is the continuation's: one more turn of the model's,
		// with the reason as its input.
		text, outcome = (&claudeSession{}).continueHeld(), "completed"
	}
}

var fixtureDropped sync.Once

// fixtureDropOnce answers true the first time only.
func fixtureDropOnce() bool {
	dropped := false
	fixtureDropped.Do(func() { dropped = true })
	return dropped
}

func refusal(answer fixtureFrame, err error) string {
	if err != nil {
		return err.Error()
	}
	return answer.Error
}

// reportActivity tells the adapter the session works or waits.
func (s *fixtureSession) reportActivity(state string) {
	_ = s.send(fixtureFrame{Op: "activity", State: state})
}

// dropIfDone closes the socket once the turns shimDropAfter allows are worked.
func (s *fixtureSession) dropIfDone() {
	limit, err := strconv.Atoi(os.Getenv(shimDropAfter))
	if err != nil {
		return
	}
	s.mu.Lock()
	s.turns++
	done := s.turns >= limit
	s.mu.Unlock()
	if done {
		s.recordTurnEvent("dropped", "", strconv.Itoa(limit))
		_ = s.conn.Close()
	}
}
