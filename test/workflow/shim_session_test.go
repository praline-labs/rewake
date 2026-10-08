package workflow

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// What a harness program of the suite remembers about the session it plays:
// its conversation, the turn it is working, and the records it writes for the
// scenario to read. The fixture's session half embeds it.

// mailboxNotice mirrors the payload the wrapper sends. It is spelled out here
// rather than imported: test/workflow is outside internal, and a fixture that
// shared the product's struct would agree with it by construction.
type mailboxNotice struct {
	Notice  string `json:"notice"`
	Members []struct {
		ID        string `json:"id"`
		From      string `json:"from"`
		FromEpoch string `json:"fromEpoch"`
		To        string `json:"to"`
		ToEpoch   string `json:"toEpoch"`
		// Recalls is the message a recall member tells the session not to
		// act on.
		Recalls string `json:"recalls"`
		// Replaces is the message a replacement member takes the place of.
		Replaces string `json:"replaces"`
		// Grants are the directories the fixture's adapter offered with the
		// letter.
		Grants []string `json:"grants"`
	} `json:"members"`
}

// recordGroup writes one delivery down as a group: which turn carried it, how
// many members it named, which, and the notice text as it arrived.
func (s *shimSession) recordGroup(turn string, notice mailboxNotice) {
	target := os.Getenv(shimGroupsFile)
	if target == "" {
		return
	}
	ids := make([]string, 0, len(notice.Members))
	for _, member := range notice.Members {
		ids = append(ids, member.ID)
	}
	// Five fields, the last two of which this column leaves as they are: the
	// count is len(members) here, and the announcement carries no id of its
	// own. The other column fills them the other way round.
	appendLine(target, fmt.Sprintf("%s\t%d\t%s\t%s\t%s", turn, len(ids), strings.Join(ids, ","), strconv.Quote(notice.Notice), ""))
}

func appendLine(target, line string) {
	file, err := os.OpenFile(target, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }()
	_, _ = fmt.Fprintln(file, line)
}

// holdOpen keeps the turn in progress until a delivery is steered into it, and
// answers what happened for the turn's own text. It returns "" when this
// session is not holding turns at all.
//
// The pending operation is the session's, not the test's: the wait is recorded
// by the session before it begins and after it ends, so a scenario can see
// that the arrival fell between the two rather than inferring it from a clock.
func (s *shimSession) holdOpen(id string) string {
	s.turn.mu.Lock()
	holding, steered, abort := s.turn.open == id, s.turn.steered, s.turn.abort
	s.turn.mu.Unlock()
	if !holding {
		return ""
	}
	s.recordTurnEvent("operation-open", id, "")
	arrived := false
	select {
	case <-steered:
		arrived = true
	case <-abort:
		// Interrupted: the turn ends here, with nothing more read.
		s.turn.mu.Lock()
		s.turn.open, s.turn.steered = "", nil
		s.turn.mu.Unlock()
		s.recordTurnEvent("operation-aborted", id, "")
		return ""
	case <-time.After(holdWindow):
	}
	// Whatever arrived mid-turn is read inside this same turn, the way an
	// agent reads the mail a steer brought it before answering. A read that
	// fails is recorded and the turn still ends: a session that says nothing
	// leaves its sender waiting for ever.
	second, err := s.readMailbox()
	if err != nil {
		second = "second read failed: " + err.Error()
	}
	s.turn.mu.Lock()
	s.turn.open, s.turn.steered, s.turn.abort = "", nil, nil
	s.turn.mu.Unlock()
	s.recordTurnEvent("operation-closed", id, fmt.Sprintf("steered=%v", arrived))
	return second
}

// recordSteer writes down that a delivery was steered into an open turn, with
// the message it named. This line exists only on the steer branch, so its
// presence is what separates "a delivery reached a working session" from "a
// delivery started a turn of its own".
func (s *shimSession) recordSteer(turn, messageID string) {
	s.recordTurnEvent("steered", turn, messageID)
}

// recordRecalls writes down each member of a delivery that recalls another
// message, with the turn it reached and the message it names: whether a recall
// stopped work in progress is a question of which turn it came into.
func (s *shimSession) recordRecalls(turn string, notice mailboxNotice) {
	for _, member := range notice.Members {
		if member.Recalls != "" {
			s.recordTurnEvent("recall", turn, member.ID+" "+member.Recalls)
		}
	}
}

// turnEventMark opens every structured line of the turn log. The older
// free-text lines stay beside these — other scenarios read them — so the two
// have to be told apart, and the mark is what does it. Separating them by the
// presence of a tab would have worked until a message body carried one: the
// free text is the first line of what rewake inbox printed, so its content is
// somebody else's to choose, and a record whose grammar depends on the mail it
// reports is a record that fails on the wrong day.
const turnEventMark = "event"

// recordTurnEvent appends one structured line to the session's turn log.
func (s *shimSession) recordTurnEvent(kind, turn, detail string) {
	s.recordTurn(fmt.Sprintf("%s\t%s\t%s\t%s", turnEventMark, kind, turn, detail))
}

// turnOutcome is what the session records about how its turn ended.
func turnOutcome() string {
	if turnFailed() {
		return "failed"
	}
	return "completed"
}

// takeAborted says whether the turn that just left its hold was aborted, and
// clears the mark for the next one.
func (s *shimSession) takeAborted() bool {
	s.turn.mu.Lock()
	defer s.turn.mu.Unlock()
	aborted := s.turn.aborted
	s.turn.aborted = false
	return aborted
}

// shimSession is the conversation a harness program plays and the turn it is
// working.
type shimSession struct {
	mu     sync.Mutex
	thread string
	turn   turnState
	// interrupted says the one interrupted turn shimInterruptFirst asks for
	// has been played.
	interrupted bool
}

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
	// abort is closed when the open turn is interrupted or a compaction
	// replaces it, and aborted says so to the work that held it.
	abort   chan struct{}
	aborted bool
}

// interruptNow reports whether this turn is the first one, to be interrupted
// under shimInterruptFirst; later turns end as usual.
func (s *shimSession) interruptNow() bool {
	if os.Getenv(shimInterruptFirst) == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.interrupted {
		return false
	}
	s.interrupted = true
	return true
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

// holdWindow bounds the wait for a second delivery to be steered in. It is a
// ceiling on a wait that a healthy run ends in milliseconds — the sender
// watches the recipient's telemetry and sends as soon as it says working — and
// a run that reaches it has already failed: nothing arrived mid-turn. Four
// seconds and no more because the controls that steer nothing wait it out in
// full, and the suite pays that wait once per control and crosswise pair.
const holdWindow = 4 * time.Second

// turnFailed reports whether this turn must end in a failure. Two switches
// reach it: one fails a turn after its answer was sent, the other fails the
// held operation a mid-turn scenario is built on.
func turnFailed() bool {
	return os.Getenv(shimLateFailure) != "" || os.Getenv(shimFailHeldTurn) != ""
}
