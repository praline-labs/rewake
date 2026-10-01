package inbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/praline-labs/rewake/internal/state"
)

// WithdrawnNotice marks a tombstone. A message its sender took back after its
// notice went out is rewritten under the same id as a note saying so: the
// notice, with its preview, is already on the recipient's screen, and the
// reader it sends to rewake inbox must find why there is nothing to do. A
// preview can itself be an instruction, and a message that silently vanished
// would leave it standing.
type WithdrawnNotice struct {
	// Kind is what the message was before it was withdrawn.
	Kind Kind      `json:"kind"`
	At   time.Time `json:"at"`
	// ReplacedBy names the message rewake edit sent in its place.
	ReplacedBy string `json:"replacedBy,omitempty"`
}

// Withdrawal is what withdrawing a message did.
type Withdrawal int

const (
	// WithdrawnUnseen says no notice had gone out, so the recipient saw nothing.
	WithdrawnUnseen Withdrawal = iota
	// WithdrawnAnnounced says its notice had gone out; the tombstone waits unread.
	WithdrawnAnnounced
	// WithdrawnHeld says the harness still holds its notice for approval, and
	// rewake cannot take that back; the tombstone waits unread.
	WithdrawnHeld
	// AlreadyWithdrawn says an earlier withdraw did it.
	AlreadyWithdrawn
)

var (
	// ErrAlreadyRead refuses a message its recipient has read: read is final.
	ErrAlreadyRead = errors.New("already read")
	// ErrNotDelivered refuses a message that failed and will never be read.
	ErrNotDelivered = errors.New("not delivered")
	// ErrNoLongerKept refuses a message whose file is gone from the mailbox.
	ErrNoLongerKept = errors.New("no longer kept")
	// ErrAlreadyWithdrawn refuses to replace a message withdrawn already.
	ErrAlreadyWithdrawn = errors.New("already withdrawn")
)

// Withdraw takes back a message nobody has read: its waiting copy goes, its
// status becomes failed and withdrawn — final, like read — and, when a notice
// may have gone out, a tombstone takes its place in unread/. With a
// replacement, that message is written first, so a failure leaves either the
// old message as it was or the tombstone and the replacement together, never a
// tombstone pointing at nothing. The caller holds the recipient's mailbox
// lock: the reader, the serving process and every status write take it too, so
// a message is either read before this or withdrawn before it is read, never
// both, and a replacement written here cannot be made readable before the
// tombstone is in place.
//
// A withdrawal that failed halfway — its status written, its tombstone not —
// is finished by the next call, withdraw or edit alike; only a finished one
// answers that it was withdrawn already. The waiting copy is not a step: once
// the tombstone is in place, the serving process drops a copy left behind as
// it settles the withdrawn status (settle), so a copy that would not go does
// not make a finished withdrawal look unfinished.
//
// Every copy and record it decides by is looked up before anything is written,
// and one that cannot be read stops it with nothing changed: the letter may be
// read already, or being read in parts.
func Withdraw(dir string, message Message, replacement *Message) (Withdrawal, error) {
	to, id := message.To, message.ID
	waitingDir, unreadDir, doneDir := state.InboxPath(dir, to), state.UnreadPath(dir, to), state.DonePath(dir, to)
	status, known, err := ReadStatus(dir, to, id)
	if err != nil {
		return 0, err
	}
	stages, err := lookUp(id, waitingDir, unreadDir, doneDir)
	if err != nil {
		return 0, err
	}
	waiting, readable, archived := stages[0], stages[1], stages[2]
	stoneUnread, err := tombstoneIn(unreadDir, id)
	if err != nil {
		return 0, err
	}
	stoneDone, err := tombstoneIn(doneDir, id)
	if err != nil {
		return 0, err
	}
	already := known && status.Withdrawn || message.Withdrawn != nil
	finished := stoneUnread || !readable && (!waiting || stoneDone)
	switch {
	case already && finished:
		if waiting {
			dropWaiting(waitingDir, id)
		}
		if replacement != nil {
			return 0, ErrAlreadyWithdrawn
		}
		return AlreadyWithdrawn, nil
	case already:
		// Halfway: the status says withdrawn, and the original may still be
		// read by hard link or served from its waiting copy.
	case known && status.State == Read:
		return 0, ErrAlreadyRead
	case readable && claimed(dir, to, id):
		return 0, ErrReadInProgress
	case readable:
	case waiting && (!known || status.State != Failed):
	case known && status.State == Failed:
		return 0, ErrNotDelivered
	case archived:
		// Moved on without a status left to say how: read, or refused, and
		// either way final.
		return 0, ErrAlreadyRead
	default:
		return 0, ErrNoLongerKept
	}

	replacedBy := ""
	if replacement != nil {
		if err := putMessage(dir, *replacement); err != nil {
			return 0, err
		}
		replacedBy = replacement.ID
	}
	result, err := withdrawSteps(dir, message, replacedBy, readable, waiting)
	if err != nil {
		if replacement != nil {
			_ = state.Remove(filepath.Join(waitingDir, replacement.ID+".json"))
			_ = state.SyncDir(waitingDir)
		}
		return 0, err
	}
	if readable && known && status.State == Held {
		result = WithdrawnHeld
	}
	return result, nil
}

// withdrawSteps writes the withdrawal: the status, the tombstone where the
// reader would find the message, and the waiting copy gone.
func withdrawSteps(dir string, message Message, replacedBy string, readable, waiting bool) (Withdrawal, error) {
	to, id := message.To, message.ID
	stone := tombstoneOf(message, replacedBy)
	detail := "withdrawn by " + message.From
	if replacedBy != "" {
		detail += ", replaced by " + replacedBy
	}
	// The status first: from here on nothing the serving process learns about
	// the notice can undo the withdrawal (outcome.go), and a reader that finds
	// the original still in unread/ reads it as withdrawn (PeekUnread).
	if err := writeStatus(dir, to, id, Result{State: Failed, Detail: detail, Withdrawn: true}); err != nil {
		return 0, err
	}
	result, where := WithdrawnUnseen, state.DonePath(dir, to)
	if readable {
		result, where = WithdrawnAnnounced, state.UnreadPath(dir, to)
	}
	encoded, err := json.MarshalIndent(stone, "", "  ")
	if err != nil {
		return 0, err
	}
	if err := state.EnsureSubdir(where); err != nil {
		return 0, err
	}
	// Written over the readable copy, which was a link to the waiting one:
	// a new file, so the waiting copy keeps the old text until it goes next.
	if err := state.WriteAtomic(filepath.Join(where, id+".json"), append(encoded, '\n')); err != nil {
		return 0, err
	}
	if waiting {
		dropWaiting(state.InboxPath(dir, to), id)
	}
	return result, nil
}

// dropWaiting removes the waiting copy of a withdrawn message. A failure is
// not the withdrawal's: the status is final and the tombstone in place, and the
// serving process removes a copy left behind when it settles that status.
func dropWaiting(waitingDir, id string) {
	if err := removeWaiting(filepath.Join(waitingDir, id+".json")); err == nil {
		_ = state.SyncDir(waitingDir)
	}
}

// tombstoneOf is a message rewritten as the note a reader finds in its place.
// A tombstone passed in stays one, with its kind and time kept.
func tombstoneOf(message Message, replacedBy string) Message {
	notice := WithdrawnNotice{Kind: KindOf(message), At: time.Now()}
	if message.Withdrawn != nil {
		notice.Kind, notice.At = message.Withdrawn.Kind, message.Withdrawn.At
	}
	notice.ReplacedBy = replacedBy
	stone := message
	stone.Kind, stone.GrantGit, stone.AddendumTo, stone.Withdrawn = Note, false, "", &notice
	// A note carries no grant: one left on it would be asked for again,
	// confirmed, and refused by a harness that takes a grant only with work.
	stone.GrantDirs, stone.GrantBroad = nil, nil
	at := message.CreatedAt.Local().Format("15:04:05")
	stone.Text = fmt.Sprintf("Rewake: %s withdrew its %s of %s before you read it; disregard its notice.", message.From, notice.Kind, at)
	if replacedBy != "" {
		stone.Text = fmt.Sprintf("Rewake: %s replaced its %s of %s before you read it; disregard its notice and read the replacement, %s.", message.From, notice.Kind, at, replacedBy)
	}
	return stone
}

// tombstoneIn says the copy of a message in one directory is a tombstone. An
// error says the copy is there and could not be read.
func tombstoneIn(directory, id string) (bool, error) {
	raw, err := state.ReadFile(filepath.Join(directory, id+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var message Message
	if err := json.Unmarshal(raw, &message); err != nil {
		return false, fmt.Errorf("the copy of %s in %s is not readable: %w", id, directory, err)
	}
	return message.Withdrawn != nil, nil
}

// lookUp says, for each directory, whether it holds a message's file, or that
// one of them could not tell.
func lookUp(id string, directories ...string) ([]bool, error) {
	found := make([]bool, len(directories))
	for index, directory := range directories {
		in, err := isIn(directory, id)
		if err != nil {
			return nil, err
		}
		found[index] = in
	}
	return found, nil
}

// putMessage writes a replacement; a test makes it fail.
var putMessage = Put
