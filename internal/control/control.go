// Package control carries a request from a main session to a running one —
// compact its conversation, interrupt its turn — and the answer back, through
// files in the target run's control directory (docs/remote-control.md).
//
// The asking side is `rewake compact` and `rewake interrupt`; the answering
// side is whatever serves the target: rewake's function-hooks module in a
// Claude Code session, which can read and write files but not delete them, and
// on Codex the wrapper. So the protocol is files only, the asker owns every
// removal, and an answer is written once and never rewritten.
package control

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"syscall"
	"time"
)

// Actions a request may carry.
const (
	Compact   = "compact"
	Interrupt = "interrupt"
)

// Outcomes of a request. A compaction is answered Started once the served
// side saw it begin, or Requested once it was asked for and its start was not
// seen within the served side's bound; either way the command has ended, and
// the compaction's own outcome reaches the asker later as a letter
// (docs/remote-control.md). Done, Refused and Failed are final.
const (
	Done      = "done"
	Refused   = "refused"
	Failed    = "failed"
	Started   = "started"
	Requested = "requested"
)

// Reasons of a refusal, the same words whichever harness answered.
const (
	InTurn             = "in a turn"
	NoTurn             = "no turn running"
	CompactionOff      = "compaction switched off"
	NothingToCompact   = "nothing to compact"
	RemoteConversation = "remote conversation"
	NotAnswering       = "not answering"
	CutShort           = "cut short"
	NoControl          = "no control directory"
	Withdrawn          = "withdrawn before it was taken"
	Busy               = "another request in flight"
)

// Request is what the asker writes. From is the asking session's name: the
// target's report of an interrupted turn names who interrupted it.
type Request struct {
	ID     string `json:"id"`
	Action string `json:"action"`
	Focus  string `json:"focus,omitempty"`
	From   string `json:"from"`
}

// Answer is what the served side writes: the outcome, a fixed reason word for
// a refusal, the host's own error text, and for a compaction the token counts
// around it. Never the summary or any other text of the conversation.
//
// Open marks a failure that leaves the outcome undecided: the request went
// out and may still be carried out, so a compaction's letter is still owed.
// Without it a failure is final, like a refusal — nothing was carried out.
type Answer struct {
	ID           string `json:"id"`
	Outcome      string `json:"outcome"`
	Reason       string `json:"reason,omitempty"`
	Detail       string `json:"detail,omitempty"`
	Open         bool   `json:"open,omitempty"`
	TokensBefore *int64 `json:"tokensBefore,omitempty"`
	TokensAfter  *int64 `json:"tokensAfter,omitempty"`
}

// The files of one control directory. The request has a fixed name, so the
// served side polls one path; taken and the answer carry the request's id, so
// a leftover of an earlier request is never read as this one's.
const (
	requestFile = "request.json"
	lockFile    = "lock"
	takenSuffix = ".taken"
	answerSuff  = ".result"
	startSuffix = ".started"
)

// RequestPath is the file the served side polls.
func RequestPath(dir string) string { return filepath.Join(dir, requestFile) }

// TakenPath is written by the served side the moment it picks a request up.
func TakenPath(dir, id string) string { return filepath.Join(dir, id+takenSuffix) }

// AnswerPath is written by the served side once the request is carried out.
func AnswerPath(dir, id string) string { return filepath.Join(dir, id+answerSuff) }

// StartedPath is written by the Claude Code wrapper's collector when the
// PreCompact hook of a compaction a main asked for reaches it: the module,
// which never sees the hooks of its own compaction, reads it as the start.
func StartedPath(dir, id string) string { return filepath.Join(dir, id+startSuffix) }

// ValidID is the shape of a request id; the served side builds paths from it
// and refuses any other.
var ValidID = regexp.MustCompile(`^[0-9a-f]{32}$`)

// Prepare makes a run's control directory, readable and writable by its user
// only. The wrapper calls it before the harness starts and removes the
// directory when the session ends.
func Prepare(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.Chmod(dir, 0o700)
}

// Limits bounds a request: Pickup is how long the served side has to take it
// before it counts as not answering, Outcome how long it then has to carry it
// out, and Poll how often the asker looks.
type Limits struct {
	Pickup  time.Duration
	Outcome time.Duration
	Poll    time.Duration
}

// ErrNoControl says the run has no control directory: it was started by a
// rewake that made none, or its wrapper is gone.
var ErrNoControl = errors.New("the session has no control directory")

// Ask writes a request into a run's control directory and waits for its
// answer, within limits. One request per run at a time: a second asker is
// refused as Busy rather than queued, since a queued compaction would run
// against a conversation the asker no longer sees. Whatever happens, the
// request and its files are gone when Ask returns — the served side cannot
// delete — except what the served side writes after the asker gave up, which
// the next Ask clears.
func Ask(ctx context.Context, dir string, request Request, limits Limits) (Answer, error) {
	if _, err := os.Stat(dir); err != nil {
		return Answer{}, ErrNoControl
	}
	lock, err := os.OpenFile(filepath.Join(dir, lockFile), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return Answer{}, err
	}
	defer func() { _ = lock.Close() }()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return Answer{Outcome: Refused, Reason: Busy, Detail: "another rewake compact or interrupt is waiting on this session"}, nil
	}
	defer func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) }()
	clearStale(dir)

	if request.ID == "" {
		request.ID = NewID()
	}
	if err := writeRequest(dir, request); err != nil {
		return Answer{}, err
	}
	defer func() {
		_ = os.Remove(RequestPath(dir))
		_ = os.Remove(TakenPath(dir, request.ID))
		_ = os.Remove(StartedPath(dir, request.ID))
	}()

	if !waitFile(ctx, limits.Pickup, limits.Poll, TakenPath(dir, request.ID), AnswerPath(dir, request.ID)) {
		// Withdraw the request first and look for the mark after: the served
		// side marks first and looks for the request after, so one of the two
		// always sees the other. A mark found now belongs to a side that either
		// saw the request still in place and carries it out, or saw it gone
		// and answers that it was withdrawn (docs/remote-control.md).
		_ = os.Remove(RequestPath(dir))
		lastLook(dir, request.ID)
		if !exists(TakenPath(dir, request.ID)) && !exists(AnswerPath(dir, request.ID)) {
			// The asker's own call was interrupted: nothing is known about
			// the target, so this is not the target's silence.
			if ctx.Err() != nil {
				return Answer{ID: request.ID, Outcome: Refused, Reason: CutShort, Detail: "the wait was cut short before anything took the request, and it is withdrawn"}, nil
			}
			detail := fmt.Sprintf("nothing took the request within %s", limits.Pickup)
			return Answer{ID: request.ID, Outcome: Refused, Reason: NotAnswering, Detail: detail}, nil
		}
	}
	// The request stays in place while it is carried out: the served side
	// checks it after marking it taken, and its mark keeps it from being
	// carried out twice.
	deadline := time.Now().Add(limits.Outcome)
	for {
		if answer, ok := readAnswer(dir, request.ID); ok {
			_ = os.Remove(AnswerPath(dir, request.ID))
			return answer, nil
		}
		if !time.Now().Before(deadline) || ctx.Err() != nil {
			detail := fmt.Sprintf("the request was taken and no outcome came within %s; it may still be carried out", limits.Outcome)
			if ctx.Err() != nil {
				detail = "the request was taken and the wait was cut short; it may still be carried out"
			}
			return Answer{ID: request.ID, Outcome: Failed, Detail: detail, Open: true}, nil
		}
		sleep(ctx, limits.Poll)
	}
}

// lastLook runs between withdrawing a request and the last look for its
// mark; a test puts the served side's move of that instant there.
var lastLook = func(_, _ string) {}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// NewID is a request id: random, and of the one shape ValidID accepts.
func NewID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// writeRequest puts the request in place whole: the served side must never
// read half of one.
func writeRequest(dir string, request Request) error {
	encoded, err := json.Marshal(request)
	if err != nil {
		return err
	}
	temporary := filepath.Join(dir, "."+request.ID+".tmp")
	if err := os.WriteFile(temporary, encoded, 0o600); err != nil {
		return err
	}
	if err := os.Rename(temporary, RequestPath(dir)); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

// readAnswer reads the answer to one request. The served side writes it in
// place, not by rename, so a file that does not parse yet is one still being
// written and is read again at the next look.
func readAnswer(dir, id string) (Answer, bool) {
	raw, err := os.ReadFile(AnswerPath(dir, id))
	if err != nil {
		return Answer{}, false
	}
	var answer Answer
	if json.Unmarshal(raw, &answer) != nil || answer.ID != id {
		return Answer{}, false
	}
	switch answer.Outcome {
	case Done, Refused, Failed, Started, Requested:
		return answer, true
	}
	return Answer{}, false
}

// clearStale removes what an asker that died, or one whose request outlived
// its limit, left behind. Called under the lock, so nothing it removes belongs
// to a request still being asked.
func clearStale(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.Name() != lockFile {
			_ = os.Remove(filepath.Join(dir, entry.Name()))
		}
	}
}

// waitFile waits for any of paths to appear.
func waitFile(ctx context.Context, limit, poll time.Duration, paths ...string) bool {
	deadline := time.Now().Add(limit)
	for {
		for _, path := range paths {
			if exists(path) {
				return true
			}
		}
		if !time.Now().Before(deadline) || ctx.Err() != nil {
			return false
		}
		sleep(ctx, poll)
	}
}

func sleep(ctx context.Context, poll time.Duration) {
	timer := time.NewTimer(poll)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	}
}
