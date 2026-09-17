package inbox

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/state"
)

// A harness is told that mail is waiting, not handed the text, so the message
// has to be readable before the notice goes out: an agent that runs rewake inbox
// the moment it is told must find it. It is linked into unread/ first and leaves
// the waiting set once its notice is out. Reading moves it on to done/, records
// the read, and notes who waits for the end of the reader's turn.

// linkUnread makes a waiting message readable. The link is the same file, so
// nothing can be read that was not written, and linking twice is harmless.
func linkUnread(dir, to, id string) error {
	unread := state.UnreadPath(dir, to)
	if err := state.EnsureSubdir(unread); err != nil {
		return err
	}
	err := os.Link(filepath.Join(state.InboxPath(dir, to), id+".json"), filepath.Join(unread, id+".json"))
	if err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	_ = state.SyncDir(unread)
	return nil
}

// dropUnread takes back a message that will not be announced after all.
func dropUnread(dir, to, id string) {
	_ = os.Remove(filepath.Join(state.UnreadPath(dir, to), id+".json"))
}

// noticeContext chooses the newest available letter; active reservations stay
// invisible to ordinary notifications just as they do to inbox reads.
func noticeContext(dir, name, epoch string, message Message) Message {
	messages, err := AvailableUnread(dir, name, epoch)
	if err != nil {
		return message
	}
	message.Unread = len(messages)
	for _, candidate := range messages {
		if message.Latest == nil || candidate.CreatedAt.After(message.Latest.CreatedAt) || candidate.CreatedAt.Equal(message.Latest.CreatedAt) && candidate.ID > message.Latest.ID {
			latest := candidate
			message.Latest = &latest
		}
	}
	return message
}

// PeekUnread returns the unread messages of one run of a session, oldest first,
// without marking anything. The caller shows them, then marks each one read: a
// message marked before its text reached anybody would be lost.
func PeekUnread(dir, name, epoch string) ([]Message, error) {
	messages, err := listIn(state.UnreadPath(dir, name))
	if err != nil {
		return nil, err
	}
	mine := make([]Message, 0, len(messages))
	for _, message := range messages {
		if epoch == "" || message.ToEpoch == epoch {
			mine = append(mine, message)
		}
	}
	return mine, nil
}

// MarkRead records that this run of the session has read a message. The caller
// holds the mailbox lock and has already shown the text.
//
// Everything reading implies is written before the message leaves unread/, and
// leaving it is the last step. A failure anywhere keeps the message unread, so
// the next read shows it again and records it again; the other order lost the
// report its sender was owed.
//
// A reader that owes nobody — the main session — records no waits at all.
func MarkRead(dir, name, epoch string, message Message, reports bool) error {
	// A read status is written only after the waiter, so finding one means
	// this is a retry of a read whose last step failed: the waiter was recorded
	// then, and may have been reported to since. Recording it again owed a
	// second report for one message.
	retry := false
	if status, ok := ReadStatus(dir, name, message.ID); ok && status.State == Read {
		retry = true
	}
	// A note or a report owes nothing, and a sender without a run of its own —
	// a shell, or mail from before runs were recorded — has nowhere a report
	// could go.
	if reports && !retry && Owed(message) {
		if err := markAwaiting(dir, name, epoch, message.From, message.FromEpoch, message.ID); err != nil {
			return err
		}
	}
	if err := writeStatus(dir, name, message.ID, Result{State: Read}); err != nil {
		return err
	}
	err := move(message.ID, state.UnreadPath(dir, name), state.DonePath(dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// Waiter is a session run waiting for the end of this turn.
type Waiter struct {
	Name  string
	Epoch string
	// Since is when the wait was recorded, in nanoseconds. It tells one wait
	// from the next for the same run, and names the report that settles it.
	Since int64
	// Messages are the ids of what was read from that run since its last report.
	Messages []string
}

// same reports whether two records describe the same wait.
func (w Waiter) same(other Waiter) bool {
	return w.Name == other.Name && w.Epoch == other.Epoch && w.Since == other.Since &&
		strings.Join(w.Messages, ",") == strings.Join(other.Messages, ",")
}

// ReportID is the id of the report that settles a wait. The same wait always
// gets the same id, so a report written and not yet forgotten — the waiter
// could not be removed, or the hook died in between — is not written again.
// The time prefix keeps reports in the order their waits began.
func ReportID(name, epoch string, waiter Waiter) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{name, epoch, waiter.Name, waiter.Epoch, strconv.FormatInt(waiter.Since, 10), strings.Join(waiter.Messages, ",")}, "\x00")))
	return fmt.Sprintf("%019d-%s", waiter.Since, hex.EncodeToString(sum[:6]))
}

// awaitingPath holds the waiters of one run. Keyed by run, because a name can
// be taken over, and what the previous run read is not the next one's to report.
func awaitingPath(dir, name, epoch string) (string, bool) {
	if epoch == "" || strings.ContainsAny(epoch, `/\`) || strings.HasPrefix(epoch, ".") {
		return "", false
	}
	return filepath.Join(state.AwaitingPath(dir, name), epoch), true
}

// markAwaiting records that a run of another session waits for this turn, and
// for which message. A run that already waits gets the message added to its
// wait; a different run of that name replaces it.
func markAwaiting(dir, name, epoch, from, fromEpoch, messageID string) error {
	path, ok := awaitingPath(dir, name, epoch)
	if !ok || !state.ValidName(from) {
		return nil
	}
	if err := state.EnsureSubdir(state.AwaitingPath(dir, name)); err != nil {
		return err
	}
	if err := state.EnsureSubdir(path); err != nil {
		return err
	}
	file := filepath.Join(path, from)
	waiter := Waiter{Name: from, Epoch: fromEpoch, Since: time.Now().UnixNano()}
	if raw, err := os.ReadFile(file); err == nil {
		if existing := parseWaiter(from, string(raw)); existing.Epoch == fromEpoch {
			waiter = existing
		}
	}
	for _, id := range waiter.Messages {
		if id == messageID {
			return nil
		}
	}
	if messageID != "" {
		waiter.Messages = append(waiter.Messages, messageID)
	}
	return writeWaiter(file, waiter)
}

func writeWaiter(file string, waiter Waiter) error {
	record := waiter.Epoch + " " + strconv.FormatInt(waiter.Since, 10) + " " + strings.Join(waiter.Messages, ",")
	return state.WriteAtomic(file, []byte(strings.TrimSpace(record)))
}

// Waiters lists who waits for the end of this run's turn. Nothing is forgotten
// here: a waiter is cleared only once the report to it is written.
func Waiters(dir, name, epoch string) []Waiter {
	path, ok := awaitingPath(dir, name, epoch)
	if !ok {
		return nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil
	}
	waiters := make([]Waiter, 0, len(entries))
	for _, entry := range entries {
		peer := entry.Name()
		if entry.IsDir() || !state.ValidName(peer) {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(path, peer))
		if err != nil {
			continue
		}
		waiters = append(waiters, parseWaiter(peer, string(raw)))
	}
	sort.Slice(waiters, func(i, j int) bool { return waiters[i].Name < waiters[j].Name })
	return waiters
}

// parseWaiter reads a wait record: the run, then when the wait began. A record
// with the run alone comes from before the time was kept.
func parseWaiter(name, raw string) Waiter {
	fields := strings.Fields(raw)
	waiter := Waiter{Name: name}
	if len(fields) > 0 {
		waiter.Epoch = fields[0]
	}
	if len(fields) > 1 {
		waiter.Since, _ = strconv.ParseInt(fields[1], 10, 64)
	}
	if len(fields) > 2 {
		waiter.Messages = strings.Split(fields[2], ",")
	}
	return waiter
}

// ClearAwaiting forgets a waiter once it has been reported to, or once its run
// has ended. Only that run is forgotten: a newer run of the same name that
// wrote in the meantime is still owed its report. Later messages from the same
// wait are retained; only the published subset is removed. The caller holds the mailbox
// lock, which is what keeps the check and the removal together.
func ClearAwaiting(dir, name, epoch string, peer Waiter) {
	path, ok := awaitingPath(dir, name, epoch)
	if !ok || !state.ValidName(peer.Name) {
		return
	}
	file := filepath.Join(path, peer.Name)
	raw, err := os.ReadFile(file)
	if err != nil {
		return
	}
	current := parseWaiter(peer.Name, string(raw))
	if current.same(peer) {
		_ = os.Remove(file)
		return
	}
	if current.Epoch != peer.Epoch || current.Since != peer.Since {
		return
	}
	remaining := make([]string, 0, len(current.Messages))
	for _, id := range current.Messages {
		if !slices.Contains(peer.Messages, id) {
			remaining = append(remaining, id)
		}
	}
	if len(remaining) == len(current.Messages) {
		return
	}
	if len(remaining) == 0 {
		_ = os.Remove(file)
		return
	}
	current.Messages = remaining
	_ = writeWaiter(file, current)
}

// sweepAwaiting forgets what earlier runs of this name were waited on for.
func sweepAwaiting(dir, name, epoch string) {
	entries, err := os.ReadDir(state.AwaitingPath(dir, name))
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.Name() != epoch {
			_ = os.RemoveAll(filepath.Join(state.AwaitingPath(dir, name), entry.Name()))
		}
	}
}

// move renames a message file between two directories of one mailbox.
func move(id, from, into string) error {
	if err := state.EnsureSubdir(into); err != nil {
		return err
	}
	source := filepath.Join(from, id+".json")
	target := filepath.Join(into, id+".json")
	if err := os.Rename(source, target); err != nil {
		return err
	}
	// The age that matters is the age of the move: an old message refused at
	// startup would otherwise be swept away in the same breath.
	now := time.Now()
	_ = os.Chtimes(target, now, now)
	// Both ends are flushed: after a crash the message must be in one of the two
	// places, never in both and never in neither.
	_ = state.SyncDir(from)
	_ = state.SyncDir(into)
	return nil
}

// listIn returns the messages in one directory, oldest first.
func listIn(directory string) ([]Message, error) {
	entries, err := os.ReadDir(directory)
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
		raw, err := os.ReadFile(filepath.Join(directory, name))
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
