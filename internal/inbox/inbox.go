/*
Package inbox carries messages between sessions as files.

A sender writes a file into the receiver's mailbox; the process that serves that
mailbox — the wrapper holding the harness — delivers it and writes back a status
the sender reads. Files, not a socket, because a sandboxed Codex agent may write
into /tmp but may not connect to a socket at all. The same path therefore works
from a plain shell, from a Claude Code tool call and from inside the sandbox.
*/
package inbox

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/praline-labs/rewake/internal/boottime"
	"github.com/praline-labs/rewake/internal/sessionstate"
	"github.com/praline-labs/rewake/internal/state"
)

// Message is one delivery, as it waits on disk.
type Message struct {
	GrantGit bool `json:"grantGit,omitempty"`
	// GrantDirs are the directories main gave the reader write access to for
	// this task, resolved when it was sent; GrantBroad are those of them main
	// confirmed as broad (docs/grants.md).
	GrantDirs    []string               `json:"grantDirs,omitempty"`
	GrantBroad   []string               `json:"grantBroad,omitempty"`
	Compaction   *CompactionNotice      `json:"compaction,omitempty"`
	Departure    *DepartureNotice       `json:"departure,omitempty"`
	SenderState  *sessionstate.Snapshot `json:"senderState,omitempty"`
	Availability *Availability          `json:"availability,omitempty"`
	Undelivered  *UndeliveredNotice     `json:"undelivered,omitempty"`
	// Withdrawn marks a tombstone: its sender took the message back before it
	// was read, and this text, under the same id, says so (withdraw.go).
	Withdrawn *WithdrawnNotice `json:"withdrawn,omitempty"`
	// Recall marks the note telling a recipient not to act on the notice of a
	// message withdrawn after it went out (recall.go).
	Recall *RecallNotice `json:"recall,omitempty"`
	// Replaces names the message this one replaced by rewake edit.
	Replaces string `json:"replaces,omitempty"`
	// AddendumTo names the task this one adds to: an addendum is a task of
	// its own, read into the same wait, so one report settles both.
	AddendumTo string `json:"addendumTo,omitempty"`
	// ID sorts by creation time, so a mailbox is served in order.
	ID string `json:"id"`
	// From is the sender's session name, or "shell" when it has none.
	From string `json:"from"`
	// To is the receiving session.
	To string `json:"to"`
	// ToEpoch names the run of that session the message was written for. A name
	// can be reused once its session ends, and mail addressed to the previous
	// tenant must not be handed to the next one.
	ToEpoch string `json:"toEpoch,omitempty"`
	// FromEpoch names the run of the sending session, so an answer to it — a
	// report that the receiver's turn ended — reaches that run and no other.
	FromEpoch string `json:"fromEpoch,omitempty"`
	// Kind says what the message is about, and it is what the receiver's notice
	// names: a task, a note, a question, or the end of the sender's turn.
	Kind Kind `json:"kind,omitempty"`
	// InReplyTo lists the messages a finished report settles: the tasks and
	// questions its sender read during the turn that ended. A sender blocked on
	// a question recognizes its answer by this.
	InReplyTo []string `json:"inReplyTo,omitempty"`
	// ThreadChanged warns that a report may belong to a different conversation.
	ThreadChanged bool `json:"threadChanged,omitempty"`
	// DeliveryThread is pinned before readability and used by the adapter.
	DeliveryThread string `json:"-"`
	// Text is what the receiving agent will read.
	Text string `json:"text"`
	// CreatedAt is when the sender wrote it.
	CreatedAt time.Time `json:"createdAt"`
	// CreatedBoot is the boot clock's reading when Put wrote it (package
	// boottime), 0 from a build that did not stamp it. The coalescing window
	// counts from it: CreatedAt is another process's wall clock, and a step of
	// that clock between the write and the serving process's look would cut
	// the wait short or stretch it by as much.
	CreatedBoot int64 `json:"createdBoot,omitempty"`
	// Unread is how many messages the receiver will hold once this one lands. It is
	// computed by the serving process right before delivery and never stored.
	Unread int `json:"-"`
	// Latest is the newest available letter shown by an aggregate notification.
	Latest *Message `json:"-"`
	// Batch is transport-only membership; each stored message keeps its own identity.
	Batch []Message `json:"-"`
}

// Kind is the subject of a message.
type Kind string

const (
	// Task is work for the receiver. Its sender is told when the receiver's
	// turn ends, with the last reply. It is the default.
	Task Kind = "task"
	// Note is a heads-up. It asks for nothing back.
	Note Kind = "notify"
	// Question is a task its sender waits for: send blocks until the receiver's
	// turn ends and prints the last reply.
	Question Kind = "question"
	// Finished tells the receiver that a session it wrote to has ended its turn.
	// The text is that session's last reply.
	Finished Kind = "finished"
	// Error reports a failed turn and never owes another report.
	Error Kind = "error"
	// Stopped reports a turn stopped — by the person at the keyboard, or by a
	// main with rewake interrupt — without settling the work.
	Stopped Kind = "stopped"
	// Interim is a turn end marked pending: the receiver ran `rewake pending` before
	// ending a turn it had not finished the work in. It carries the text given
	// there, owes nothing, and settles nothing: the report comes at the next
	// turn end without such a mark.
	Interim Kind = "pending"
)

// KindOf returns the kind of a message, reading a missing one as a task: mail
// written before kinds existed asked for work and was owed a report.
func KindOf(message Message) Kind {
	if message.Kind == "" {
		return Task
	}
	return message.Kind
}

// CarriesGrant says whether a message gives its reader a permission for its
// task. Such a message is announced on its own, and only while its reader is
// idle, so the permission holds from the task's first turn (docs/grants.md).
func CarriesGrant(message Message) bool {
	return message.GrantGit || len(message.GrantDirs) > 0
}

// Owed reports whether reading a message owes its sender a report when the
// reader's turn ends. A note asks for nothing, and a report is an answer
// already: waiting on one would have two sessions report to each other forever.
func Owed(message Message) bool {
	return AsksForWork(message) && message.FromEpoch != ""
}

// AsksForWork reports whether a message's kind asks its reader for work — a
// task or a question, and a kind this build does not know, as Owed takes it —
// whoever sent it: a task from a plain shell asks for work too, though nobody
// waits for its report.
func AsksForWork(message Message) bool {
	switch KindOf(message) {
	case Note, Finished, Error, Stopped, Interim:
		return false
	}
	return true
}

// State is what happened to a message.
type State string

const (
	// Delivered means the harness was told the message is waiting.
	Delivered State = "delivered"
	// Read means the receiving agent fetched the text.
	Read State = "read"
	// Pending means it is accepted and still waiting for a chance to land.
	Pending State = "pending"
	// Failed means it will not be delivered.
	Failed State = "failed"
	// Held means the harness took the notice and keeps it from the agent until
	// somebody releases it. It is not delivered: it may still be delivered,
	// expire or be refused, and the serving process writes that down when the
	// harness says so.
	Held State = "held"
)

// Result is what a delivery attempt says about itself.
type Result struct {
	State State
	// ReportAvailable preserves an accepted report when only its notice failed.
	ReportAvailable bool
	// Via names the path used, such as "socket" or "codex queue".
	Via string
	// Detail explains a pending or failed result in one sentence.
	Detail string
	// Withdrawn marks a failed result its sender caused by withdrawing the
	// message: final like a read, whatever the harness says afterwards.
	Withdrawn bool
	// GrantApplied lists the granted directories the harness took with the
	// notice; Detail says what became of the others.
	GrantApplied []string
}

// Status is the Result as the sender reads it back.
type Status struct {
	ReportAvailable bool      `json:"reportAvailable,omitempty"`
	State           State     `json:"state"`
	Via             string    `json:"via,omitempty"`
	Detail          string    `json:"detail,omitempty"`
	Withdrawn       bool      `json:"withdrawn,omitempty"`
	GrantApplied    []string  `json:"grantApplied,omitempty"`
	At              time.Time `json:"at"`
}

// withdrawn is what the server takes a withdrawn message's outcome for. It is
// never written: on disk the status is failed, which is what a sender waiting
// on it has to hear, but a failed one is settled by archiving its readable
// copy, and the tombstone there has to stay until it is read.
const withdrawn State = "withdrawn"

// final says nothing said about the notice afterwards can change this status:
// the agent has the text, or its sender took it back.
func (s Status) final() bool { return s.State == Read || s.Withdrawn }

// outcome is the state the server settles a status by.
func (s Status) outcome() State {
	if s.Withdrawn {
		return withdrawn
	}
	return s.State
}

func (s Status) result() Result {
	return Result{State: s.State, Via: s.Via, Detail: s.Detail, ReportAvailable: s.ReportAvailable, Withdrawn: s.Withdrawn, GrantApplied: s.GrantApplied}
}

// isFinal is final for an outcome the server already holds.
func isFinal(outcome State) bool { return outcome == Read || outcome == withdrawn }

// NewID returns an identifier that sorts by time and never repeats. The time
// prefix gives mailbox order; the random tail keeps two senders in the same
// millisecond apart. The tail is six bytes rather than four because a short
// piece of it is shown to the receiver, and that piece is what keeps two
// identical messages from being taken for a repeat.
func NewID() string {
	var tail [6]byte
	_, _ = rand.Read(tail[:])
	// Nanoseconds, not milliseconds: two messages written in the same
	// millisecond would otherwise be ordered by the random tail, and a mailbox
	// served in that order delivers "do it" before "here is what to do".
	return fmt.Sprintf("%019d-%s", time.Now().UnixNano(), hex.EncodeToString(tail[:]))
}

// Put writes a message into the mailbox of its receiver.
func Put(dir string, message Message) error {
	if message.CreatedBoot == 0 {
		message.CreatedBoot = boottime.Now()
	}
	mailbox := state.InboxPath(dir, message.To)
	if err := state.EnsureSubdir(mailbox); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(message, "", "  ")
	if err != nil {
		return err
	}
	return state.WriteAtomic(filepath.Join(mailbox, message.ID+".json"), append(encoded, '\n'))
}

// PutOnce writes a message unless one with its id already exists in the
// mailbox, whatever became of it since. A message moves forward only — waiting,
// unread, done — so looking in that order cannot miss one on its way.
func PutOnce(dir string, message Message) error {
	for _, directory := range []string{
		state.InboxPath(dir, message.To),
		state.UnreadPath(dir, message.To),
		state.DonePath(dir, message.To),
	} {
		if _, err := os.Stat(filepath.Join(directory, message.ID+".json")); err == nil {
			return nil
		}
	}
	return Put(dir, message)
}

// Answered reports whether this message has left the mailbox — delivered or
// refused — even if its status is no longer kept. Answers are swept after a
// while, and a sender that came back later would otherwise be told its message
// is still on its way.
func Answered(dir, to, id string) bool {
	if _, err := os.Stat(filepath.Join(state.InboxPath(dir, to), id+".json")); err == nil {
		return false
	}
	_, err := os.Stat(filepath.Join(state.DonePath(dir, to), id+".json"))
	return err == nil || os.IsNotExist(err)
}

// ReadStatus returns the status of a message, if one has been written.
func ReadStatus(dir, to, id string) (Status, bool) {
	raw, err := os.ReadFile(statusPath(dir, to, id))
	if err != nil {
		return Status{}, false
	}
	var status Status
	if err := json.Unmarshal(raw, &status); err != nil {
		return Status{}, false
	}
	return status, true
}

// statusPoll is how often a sender looks for its answer. It is short because the
// wait is short and bounded by --wait: the server's own tick is the slow one, and
// borrowing it here made every delivery look like it took a second.
const statusPoll = 25 * time.Millisecond

// Await waits for a status until the deadline, and returns the last one seen.
// A message with no status yet is not lost: the mailbox is durable, and the
// serving process writes one as soon as it can.
func Await(dir, to, id string, timeout time.Duration) (Status, bool) {
	deadline := time.Now().Add(timeout)
	for {
		// Held is not an answer yet: a release or an expiry usually follows
		// within the wait, and the last word is the one worth printing.
		if status, ok := ReadStatus(dir, to, id); ok && status.State != Pending && status.State != Held {
			return status, true
		} else if ok && time.Now().After(deadline) {
			return status, true
		}
		if time.Now().After(deadline) {
			return Status{}, false
		}
		time.Sleep(statusPoll)
	}
}

// writeStatus records what happened to a message.
func writeStatus(dir, to, id string, result Result) error {
	status := Status{State: result.State, Via: result.Via, Detail: result.Detail, ReportAvailable: result.ReportAvailable, Withdrawn: result.Withdrawn, GrantApplied: result.GrantApplied, At: time.Now()}
	encoded, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	return state.WriteAtomic(statusPath(dir, to, id), append(encoded, '\n'))
}

func statusPath(dir, to, id string) string {
	return filepath.Join(state.InboxPath(dir, to), id+".status")
}

// list returns the waiting messages of a mailbox, oldest first.
func list(dir, to string) ([]Message, error) {
	return listIn(state.InboxPath(dir, to))
}

// archive moves a finished message out of the waiting set, keeping it for
// diagnosis rather than deleting it.
func archive(dir, to, id string) error {
	err := move(id, state.InboxPath(dir, to), state.DonePath(dir, to))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// IsReport includes every turn outcome — successful, failed, stopped and
// interim — for what a report shares with the others: a notice that failed
// leaves it readable rather than lost.
func IsReport(message Message) bool {
	switch KindOf(message) {
	case Finished, Error, Stopped, Interim:
		return true
	}
	return false
}

// Settles reports whether a report ends the wait of whoever sent the work: a
// blocked --question takes it as its answer. An interim one does not; the
// question waits on for the report that follows it.
func Settles(message Message) bool {
	return IsReport(message) && KindOf(message) != Interim
}
