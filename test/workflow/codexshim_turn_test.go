package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Receiving a turn, reading the mail, and reporting — the half of the shim that
// makes a delivery scenario mean anything.
//
// The wrapper delivers by calling turn/start with an empty input and the
// mailbox notice as the output of an external operation. A fixture that
// accepted any turn at all would let a scenario pass while the notice was
// mangled on the way, so what arrives is checked: the operation's name, the
// message identities, the epochs.
//
// After that the shim calls the built rewake itself. That is not politeness:
// the model keeps no record of who performed a read, so "the session read it,
// not the test" is a property of the fixture and of nothing else.

// turnState is what the shim remembers about the turn it is running.
type turnState struct {
	mu      sync.Mutex
	counter int
	// lastNotice is what arrived with the turn, for the scenario to compare
	// against what it sent.
	lastNotice mailboxNotice
	// deferred are members left unread by an earlier delivery under
	// shimReadEach, to be read at the next one.
	deferred []string
	// open is the turn this session is working, empty when it is idle. A
	// turn/start arriving while it is set is steered into that turn, which is
	// what the real server does: start_or_steer_turn tries to steer first and
	// starts only when there is no active turn.
	open string
	// steered is closed when a delivery has been steered into the open turn,
	// so the held work can finish instead of waiting for its deadline.
	steered chan struct{}
}

// deliveredTurn checks an incoming turn, decides between starting one and
// steering it into the turn already running, and answers accordingly. The
// check is a separate function on purpose, and in another file: it is the part
// a contract test wants, and running the work would start a goroutine that
// outlives the test and calls rewake — in whatever environment happens to be
// current by then. See checkedDelivery.
func (s *shimSession) deliveredTurn(params json.RawMessage) (any, any, error) {
	notice, err := s.checkedDelivery(params)
	if err != nil {
		return nil, nil, err
	}
	s.recordDelivered(notice)

	s.turn.mu.Lock()
	// The start-or-steer fork, as the server decides it: a thread with a turn
	// in progress is steered and answers with that turn's id, an idle one
	// starts a new turn. The reply is the same object in both branches — only
	// the id differs — and a steer emits no second turn/started, because that
	// event belongs to a task beginning and no task begins here.
	if open := s.turn.open; open != "" {
		steered := s.turn.steered
		s.turn.mu.Unlock()
		s.recordSteer(open, messageIDOf(params))
		s.recordGroup(open, notice)
		if steered != nil {
			select {
			case <-steered:
			default:
				close(steered)
			}
		}
		return turnReply(open), nil, nil
	}
	s.turn.counter++
	// A new turn gets a new id. Reusing one is how an acknowledgement ends up
	// belonging to the wrong turn without anybody noticing.
	id := fmt.Sprintf("turn-%d", s.turn.counter)
	s.turn.lastNotice = notice
	if os.Getenv(shimHoldTurn) != "" {
		s.turn.open = id
		s.turn.steered = make(chan struct{})
	}
	s.turn.mu.Unlock()
	s.recordGroup(id, notice)

	go s.workTurn(id, notice)
	return turnReply(id), nil, nil
}

// messageIDOf reads the id a delivery names, for the record of what was
// steered. The request has already been accepted by checkedDelivery, so a
// value that cannot be read here means the record is wrong rather than the
// request.
func messageIDOf(params json.RawMessage) string {
	var asked struct {
		MessageID string `json:"clientUserMessageId"`
	}
	if json.Unmarshal(params, &asked) != nil {
		return ""
	}
	return asked.MessageID
}

// The messages of the delivery path, built in one place. The shape check reads
// them from here, so what it checks is what the shim actually sends — a check
// against a copy written for the occasion would agree with itself.

// turnObject is a turn as the protocol describes one: an id, the items it
// holds so far, and where it stands. A turn carrying only an id is not a turn,
// and a fixture that sent one would be teaching the wrapper a shape the real
// server never produces.
func turnObject(id, status string, items ...map[string]any) map[string]any {
	// Never nil: an absent list and an empty one are different things on the
	// wire, and only the empty one is a turn that has produced nothing yet.
	held := []map[string]any{}
	held = append(held, items...)
	return map[string]any{"id": id, "items": held, "status": status}
}

// agentMessageItem is the one kind of item this shim produces.
func agentMessageItem(id, text string) map[string]any {
	return map[string]any{"id": "item-" + id, "type": "agentMessage", "text": text}
}

// turnReply answers a turn/start. The turn has just begun, so it holds nothing.
func turnReply(id string) map[string]any {
	return map[string]any{"turn": turnObject(id, "inProgress")}
}

// turnStartedEvent announces that the turn is running.
func (s *shimSession) turnStartedEvent(id string) map[string]any {
	return map[string]any{
		"method": "turn/started",
		"params": map[string]any{"threadId": s.thread, "turn": turnObject(id, "inProgress")},
	}
}

// itemCompletedEvent carries what the session has to say.
func (s *shimSession) itemCompletedEvent(id, text string) map[string]any {
	return map[string]any{
		"method": "item/completed",
		"params": map[string]any{
			"threadId": s.thread,
			"turnId":   id,
			"item":     agentMessageItem(id, text),
			// When the item finished, which the protocol requires and which a
			// client may use to order what it received.
			"completedAtMs": time.Now().UnixMilli(),
		},
	}
}

// turnCompletedEvent ends the turn, either with the answer or with a failure.
// Either way the turn carries the item it produced: the content was sent, and
// a terminal event that forgot it would describe a different turn.
func (s *shimSession) turnCompletedEvent(id, text string, failed bool) map[string]any {
	turn := turnObject(id, "completed", agentMessageItem(id, text))
	if failed {
		turn = turnObject(id, "failed", agentMessageItem(id, text))
		turn["error"] = map[string]any{"message": "a failure after the answer"}
	}
	return map[string]any{
		"method": "turn/completed",
		"params": map[string]any{"threadId": s.thread, "turn": turn},
	}
}

// workTurn is what a session does with a delivery: read the mail, then finish
// the turn with something to say.
func (s *shimSession) workTurn(id string, notice mailboxNotice) {
	// Announced first, the way a server does. Not for correlation: a report is
	// tied to a read by the obligations that read recorded — sender, epoch,
	// message id, and a monotonic read number compared against the counter
	// captured when the turn ends — not by whether the read fell inside the
	// turn's interval.
	s.mu.Lock()
	s.broadcast(s.turnStartedEvent(id))
	// The thread is working now, and a client learns that from the status
	// notification rather than from the turn events: the wrapper's telemetry
	// reads thread/status/changed, so a fixture that never sent one left every
	// session looking idle while it worked.
	s.broadcast(s.threadStatusChangedEvent(activeStatus()))
	s.mu.Unlock()
	time.Sleep(20 * time.Millisecond)

	var text string
	var err error
	if os.Getenv(shimReadEach) != "" {
		text, err = s.readEach(notice)
	} else {
		text, err = s.readMailbox()
	}
	if err != nil {
		// Still a terminal turn, but one that reports the failure — a session
		// that says nothing leaves the sender waiting for ever.
		text = "could not read the mailbox: " + err.Error()
	}
	// A record of the turns this session accepted, for a scenario that has to
	// tell "no turn arrived" from "a turn arrived and produced no report".
	s.recordTurn(id + " " + firstLine(text))
	if held := s.holdOpen(id); held != "" {
		text += "; " + held
	}
	markPendingOnce()
	s.mu.Lock()
	// The content first, then the terminal event: a report without content is
	// not a report, and the wrapper assembles one from what it saw in order.
	s.broadcast(s.itemCompletedEvent(id, text))
	// The turn fails after its content was sent: a session that says something
	// and then breaks has not answered.
	s.broadcast(s.turnCompletedEvent(id, text, turnFailed()))
	if os.Getenv(shimSecondTerminal) != "" {
		// The same turn ends twice. Recorded as well as sent: a scenario that
		// only counted reports could not tell "the second was ignored" from
		// "the second was never sent".
		s.broadcast(s.turnCompletedEvent(id, text, false))
		s.recordTurn(id + " terminal-again")
	}
	s.broadcast(s.threadStatusChangedEvent(idleStatus()))
	s.mu.Unlock()
	s.recordTurnEvent("completed", id, turnOutcome())
}

// recordTurn appends one line to the session's record of what it did with the
// turns it accepted.
func (s *shimSession) recordTurn(line string) {
	turns := os.Getenv(shimTurnsFile)
	if turns == "" {
		return
	}
	file, err := os.OpenFile(turns, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }()
	_, _ = fmt.Fprintln(file, line)
}

// readMailbox runs the built rewake, as the session, and returns what it read.
//
// The guard comes first, and it is not a precaution against a mistake that has
// not happened: a turn accepted inside a test starts this work in a goroutine,
// and by the time the goroutine runs the test may have restored the ambient
// environment — the one belonging to a session somebody is working in. The
// shim refuses to run anything outside its own case, so leaving the isolation
// is impossible rather than unlikely.
func (s *shimSession) readMailbox() (string, error) {
	if err := insideACase(); err != nil {
		return "", err
	}
	if os.Getenv(shimReadFails) != "" {
		return "", errors.New("the mailbox read was made to fail")
	}
	arguments := []string{"inbox"}
	if os.Getenv(shimInboxJSON) != "" {
		arguments = append(arguments, "--json")
	}
	command := exec.Command("rewake", arguments...)
	command.Env = os.Environ()
	out, err := command.Output()
	if err != nil {
		return "", err
	}
	read := strings.TrimSpace(string(out))
	if target := os.Getenv(shimMailboxFile); target != "" {
		// Appended, not replaced: a session is delivered more than one thing
		// — availability notices arrive before any task — and a scenario
		// looking for its own message must be able to find it among them.
		if file, err := os.OpenFile(target, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
			_, _ = file.WriteString(read + "\n---\n")
			_ = file.Close()
		}
	}
	if read == "" {
		return "", errors.New("the mailbox was empty when the session read it")
	}
	// The whole of what was read, not its first line: an inbox reply opens
	// with a header naming the sender, the kind and the time, and the message
	// itself comes after it. A report quoting only the header would say
	// nothing about which message it answers.
	return "read: " + read, nil
}

// recordDelivered writes down the id of every message a delivery named. The
// report that follows says which messages it settles, and that link is the
// only thing tying a report to this message rather than to any message — so
// what arrived has to be known independently of what the report claims.
func (s *shimSession) recordDelivered(notice mailboxNotice) {
	target := os.Getenv(shimDeliveredFile)
	if target == "" {
		return
	}
	file, err := os.OpenFile(target, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }()
	for _, member := range notice.Members {
		id := member.ID
		if os.Getenv(shimWrongReportID) != "" {
			// The delivery was real; what the scenario is told about it is
			// not. This is how an observation that only looks at "a report
			// came back" is told apart from one that checks which message the
			// report settles.
			id = "0000-not-the-message-that-arrived"
		}
		_, _ = fmt.Fprintln(file, id)
	}
}

// insideACase says whether this process is running as a shim inside a case
// that owns its own state directory. Both marks are set by the case and by
// nothing else; the ambient environment of a live session has the state
// directory but never the shim mark.
func insideACase() error {
	if os.Getenv(shimEnv) == "" {
		return errors.New("refusing to run rewake: this process is not a shim inside a case")
	}
	if os.Getenv(stateDirEnv) == "" {
		return errors.New("refusing to run rewake: no case state directory in the environment")
	}
	return nil
}

func firstLine(text string) string {
	if cut := strings.IndexByte(text, '\n'); cut >= 0 {
		return text[:cut]
	}
	return text
}
