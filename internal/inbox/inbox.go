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
	"sort"
	"strings"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/state"
)

// Message is one delivery, as it waits on disk.
type Message struct {
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
	// Text is what the receiving agent will read.
	Text string `json:"text"`
	// CreatedAt is when the sender wrote it.
	CreatedAt time.Time `json:"createdAt"`
}

// State is what happened to a message.
type State string

const (
	// Delivered means the harness has it.
	Delivered State = "delivered"
	// Pending means it is accepted and still waiting for a chance to land.
	Pending State = "pending"
	// Failed means it will not be delivered.
	Failed State = "failed"
)

// Result is what a delivery attempt says about itself.
type Result struct {
	State State
	// Via names the path used, such as "socket" or "codex queue".
	Via string
	// Detail explains a pending or failed result in one sentence.
	Detail string
}

// Status is the Result as the sender reads it back.
type Status struct {
	State  State     `json:"state"`
	Via    string    `json:"via,omitempty"`
	Detail string    `json:"detail,omitempty"`
	At     time.Time `json:"at"`
}

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

// Await waits for a status until the deadline, and returns the last one seen.
// A message with no status yet is not lost: the mailbox is durable, and the
// serving process writes one as soon as it can.
func Await(dir, to, id string, timeout time.Duration) (Status, bool) {
	deadline := time.Now().Add(timeout)
	for {
		if status, ok := ReadStatus(dir, to, id); ok && status.State != Pending {
			return status, true
		} else if ok && time.Now().After(deadline) {
			return status, true
		}
		if time.Now().After(deadline) {
			return Status{}, false
		}
		time.Sleep(pollInterval)
	}
}

// writeStatus records what happened to a message.
func writeStatus(dir, to, id string, result Result) error {
	status := Status{State: result.State, Via: result.Via, Detail: result.Detail, At: time.Now()}
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
	entries, err := os.ReadDir(state.InboxPath(dir, to))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") || strings.HasPrefix(name, ".") {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	messages := make([]Message, 0, len(names))
	for _, name := range names {
		raw, err := os.ReadFile(filepath.Join(state.InboxPath(dir, to), name))
		if err != nil {
			continue
		}
		var message Message
		if err := json.Unmarshal(raw, &message); err != nil {
			// A file that is not a message is not ours to interpret; leave it
			// where it is rather than deleting somebody else's data.
			continue
		}
		messages = append(messages, message)
	}
	return messages, nil
}

// archive moves a finished message out of the waiting set, keeping it for
// diagnosis rather than deleting it.
func archive(dir, to, id string) error {
	done := state.DonePath(dir, to)
	if err := state.EnsureSubdir(done); err != nil {
		return err
	}
	mailbox := state.InboxPath(dir, to)
	from := filepath.Join(mailbox, id+".json")
	if err := os.Rename(from, filepath.Join(done, id+".json")); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	// Both ends of the move are flushed: after a crash the message must be in
	// one of the two places, never in both and never in neither.
	_ = state.SyncDir(mailbox)
	_ = state.SyncDir(done)
	return nil
}
