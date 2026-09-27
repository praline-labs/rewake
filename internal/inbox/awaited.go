package inbox

import (
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/iiiokojiadbi/rewake/internal/state"
)

// Stage is where a task or question stands on its way to the report its sender
// waits for.
type Stage string

const (
	// StageUndelivered is accepted and waiting in the mailbox: no notice yet.
	StageUndelivered Stage = "undelivered"
	// StageHeld is a notice the recipient's harness keeps back until released.
	StageHeld Stage = "held"
	// StageUnread is announced and not read yet.
	StageUnread Stage = "unread"
	// StageOwed is read: the recipient is on it and owes the report.
	StageOwed Stage = "owed"
	// StagePending is read, and an interim report said the work is still going.
	StagePending Stage = "pending"
	// StageStopped is read, and the recipient's turn was stopped since: by the
	// person at the keyboard, or by a main with rewake interrupt.
	StageStopped Stage = "stopped"
	// StageFailed will not be delivered, so no report is coming.
	StageFailed Stage = "failed"
)

// RecipientRun is what became of the run a message was written for.
type RecipientRun int

const (
	// RunLive is still running and may yet report.
	RunLive RecipientRun = iota
	// RunEnded is gone, and no run of that name has started since.
	RunEnded
	// RunReplaced is gone, and a new run holds the name.
	RunReplaced
)

// AwaitedMessage is a task or question one run sent and has no report on yet.
type AwaitedMessage struct {
	Message
	Stage Stage
	// Detail is why a notice is held or a delivery failed, or the text of the
	// interim or stopped report; empty when the stage says it all.
	Detail string
	// Run is what became of the recipient's run. Anything but RunLive means
	// no report is coming, whatever the stage.
	Run RecipientRun
}

// Gone says no report can come, because the run it was written for is over.
func (m AwaitedMessage) Gone() bool { return m.Run != RunLive }

// Awaited lists what one run sent — tasks and questions, never notes — and has
// no report on yet, oldest first. Only that run's mail is listed: an earlier
// run of the same name was answered, or not, as someone else.
//
// runOf says what became of a recipient's run. It decides what a read message
// with no wait record means. The record is cleared only once the report is
// written, and only a new run of the name sweeps a record away, so while no
// new run has started — the recipient's run lives, or ended with nobody after
// it — a missing record means answered, even after the report itself has been
// swept from this run's mailbox. Once a new run has taken the name, the old
// run's records are gone and only a settling report tells answered from lost.
//
// Like OwedMessages, it only reads: no lock, no status, nothing marked. Every
// record read here is written whole or not at all, and a message caught
// between two directories is counted once, at the stage further along.
func Awaited(dir, name, epoch string, runOf func(recipient, run string) RecipientRun) []AwaitedMessage {
	if epoch == "" {
		return nil
	}
	reports := reportsTo(dir, name, epoch)
	var awaited []AwaitedMessage
	for _, recipient := range mailboxes(dir) {
		waits := map[string]Waiter{}
		for _, message := range sentBy(dir, recipient, name, epoch) {
			run := message.ToEpoch
			wait, seen := waits[run]
			if !seen {
				wait = waiterFor(dir, recipient, run, name, epoch)
				waits[run] = wait
			}
			if item, settled := placed(dir, message, reports[message.ID], wait, runOf); !settled {
				awaited = append(awaited, item)
			}
		}
	}
	sort.SliceStable(awaited, func(i, j int) bool { return awaited[i].ID < awaited[j].ID })
	return awaited
}

// AwaitedOne places one message this run sent, as Awaited would: where it
// stands, or that a report has settled it.
func AwaitedOne(dir, name, epoch string, message Message, runOf func(recipient, run string) RecipientRun) (AwaitedMessage, bool) {
	wait := waiterFor(dir, message.To, message.ToEpoch, name, epoch)
	return placed(dir, message, reportsTo(dir, name, epoch)[message.ID], wait, runOf)
}

func placed(dir string, message Message, reports []Message, wait Waiter, runOf func(recipient, run string) RecipientRun) (AwaitedMessage, bool) {
	item, settled := stageOf(dir, message, reports)
	if settled {
		return item, true
	}
	item.Run = runOf(message.To, message.ToEpoch)
	if item.Run != RunLive {
		// A run that resumed the conversation took the wait over, and its
		// turn will report.
		if run, ok := adoptedBy(dir, message); ok {
			item.Run = runOf(message.To, run)
			wait = waiterFor(dir, message.To, run, message.From, message.FromEpoch)
		}
	}
	if item.read() && !slices.Contains(wait.Messages, message.ID) && recordKept(dir, message.To, message.ToEpoch, item.Run) {
		return item, true
	}
	return item, false
}

// recordKept says whether the recipient run's wait records can still be
// trusted to name what it owes. An ended run's are swept by nobody until a new
// run starts, and a directory that is not there says nothing either way.
func recordKept(dir, recipient, run string, what RecipientRun) bool {
	switch what {
	case RunLive:
		return true
	case RunEnded:
		path, ok := awaitingPath(dir, recipient, run)
		if !ok {
			return false
		}
		info, err := os.Stat(path)
		return err == nil && info.IsDir()
	}
	return false
}

// read says the recipient has taken the text: whether it still owes the report
// is for its wait record to say.
func (m AwaitedMessage) read() bool {
	return m.Stage == StageOwed || m.Stage == StagePending || m.Stage == StageStopped
}

// stageOf places one sent message, or says a report has settled it. A read
// message comes back as owed, pending or stopped; whether its recipient still
// records the wait is the caller's question.
func stageOf(dir string, message Message, reports []Message) (AwaitedMessage, bool) {
	item := AwaitedMessage{Message: message}
	var latest *Message
	for index, report := range reports {
		// From any run of the recipient, not only the one the message was
		// written for: a run that resumed the conversation takes the wait
		// over and reports on it (adopt.go), and a run reports only on what
		// its own waits name.
		if report.From != message.To {
			continue
		}
		switch KindOf(report) {
		case Finished, Error:
			return item, true
		case Interim, Stopped:
			// Not by id alone: every report on one wait shares the wait's
			// time prefix, and the tail after it is a hash.
			if latest == nil || report.CreatedAt.After(latest.CreatedAt) ||
				report.CreatedAt.Equal(latest.CreatedAt) && report.ID > latest.ID {
				latest = &reports[index]
			}
		}
	}
	status, known := ReadStatus(dir, message.To, message.ID)
	read := known && status.State == Read
	if !known && isIn(state.DonePath(dir, message.To), message.ID) {
		// The status is swept earlier than the message; one in done/ was
		// read or refused, and a refusal without its status is a read as far
		// as anyone can tell now.
		read = true
	}
	switch {
	case read:
		item.Stage = StageOwed
		if latest != nil {
			item.Stage, item.Detail = StagePending, latest.Text
			if KindOf(*latest) == Stopped {
				item.Stage = StageStopped
			}
		}
	case known && status.State == Failed:
		item.Stage, item.Detail = StageFailed, status.Detail
	case known && status.State == Held:
		item.Stage, item.Detail = StageHeld, status.Detail
	case known && status.State == Delivered:
		item.Stage = StageUnread
	default:
		// A pending status says why it waits: a compaction, a session still
		// starting.
		item.Stage = StageUndelivered
		if known {
			item.Detail = status.Detail
		}
	}
	return item, false
}

// mailboxes names every mailbox in the room.
func mailboxes(dir string) []string {
	entries, err := os.ReadDir(state.InboxesPath(dir))
	if err != nil {
		return nil
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() && state.ValidName(entry.Name()) {
			names = append(names, entry.Name())
		}
	}
	return names
}

// sentBy lists the tasks and questions one run wrote into a mailbox, wherever
// the mailbox keeps them now.
func sentBy(dir, recipient, name, epoch string) []Message {
	var sent []Message
	for _, message := range everywhere(dir, recipient) {
		if message.From == name && message.FromEpoch == epoch && message.To == recipient && Owed(message) {
			sent = append(sent, message)
		}
	}
	return sent
}

// reportsTo gathers the reports this run received, by the message each answers.
func reportsTo(dir, name, epoch string) map[string][]Message {
	byTask := map[string][]Message{}
	for _, report := range everywhere(dir, name) {
		if !IsReport(report) || report.ToEpoch != epoch {
			continue
		}
		for _, id := range report.InReplyTo {
			byTask[id] = append(byTask[id], report)
		}
	}
	return byTask
}

// everywhere lists a mailbox's messages in all three places, each once. The
// walk follows a message's own way — waiting, unread, done, as PutOnce does —
// so one that moves on during the walk is found again further along rather
// than missed in both; a copy found later replaces the one seen before.
func everywhere(dir, name string) []Message {
	var order []string
	found := map[string]Message{}
	for _, directory := range []string{state.InboxPath(dir, name), state.UnreadPath(dir, name), state.DonePath(dir, name)} {
		messages, _ := listIn(directory)
		for _, message := range messages {
			if _, seen := found[message.ID]; !seen {
				order = append(order, message.ID)
			}
			found[message.ID] = message
		}
		afterListing(directory)
	}
	all := make([]Message, 0, len(order))
	for _, id := range order {
		all = append(all, found[id])
	}
	return all
}

// afterListing lets a test move a message on between two listings.
var afterListing = func(string) {}

// waiterFor is what a recipient's run still owes this run, if anything.
func waiterFor(dir, recipient, run, name, epoch string) Waiter {
	for _, waiter := range Waiters(dir, recipient, run) {
		if waiter.Name == name && waiter.Epoch == epoch {
			return waiter
		}
	}
	return Waiter{}
}

func isIn(directory, id string) bool {
	_, err := os.Stat(filepath.Join(directory, id+".json"))
	return err == nil
}
