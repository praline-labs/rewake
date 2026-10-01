package inbox

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/state"
)

// Waiter is a session run waiting for the end of this turn.
type Waiter struct {
	ReadSequences []uint64
	Name          string
	Epoch         string
	// Since is when the wait was recorded, in nanoseconds. It tells one wait
	// from the next for the same run, and names the report that settles it.
	Since int64
	// Messages are the ids of what was read from that run since its last report.
	Messages []string
	// ReadAt is when each of Messages was read, in nanoseconds: a wait
	// gathers what is read until the report, and each task may be resumed
	// for its own time from its reading (adopt.go).
	ReadAt []int64
}

// readAt is when the message at index was read.
func (w Waiter) readAt(index int) int64 {
	if index < len(w.ReadAt) && w.ReadAt[index] != 0 {
		return w.ReadAt[index]
	}
	// legacy(rewake <2026-09-28): a wait record written before read times were kept has only when its wait began; remove when no session started by an earlier build is registered
	return w.Since
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
	return markAwaitingSequence(dir, name, epoch, from, fromEpoch, messageID, 0, 0)
}

// markAwaitingSequence is markAwaiting with the message's read sequence, and
// when it was read: now when readAt is 0, the earlier run's reading for a
// wait taken over (AdoptWaits), since the resume window counts from the read.
// A record there that cannot be read is an error: written over, it would drop
// what the run owes for the messages it names.
func markAwaitingSequence(dir, name, epoch, from, fromEpoch, messageID string, sequence uint64, readAt int64) error {
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
	now := time.Now().UnixNano()
	waiter := Waiter{Name: from, Epoch: fromEpoch, Since: now}
	raw, err := state.ReadFile(file)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil {
		existing, err := parseWaiter(from, string(raw))
		if err != nil {
			return err
		}
		if existing.Epoch == fromEpoch {
			waiter = existing
		}
	}
	for _, id := range waiter.Messages {
		if id == messageID {
			return nil
		}
	}
	if messageID != "" {
		for len(waiter.ReadSequences) < len(waiter.Messages) {
			waiter.ReadSequences = append(waiter.ReadSequences, 0)
		}
		for len(waiter.ReadAt) < len(waiter.Messages) {
			waiter.ReadAt = append(waiter.ReadAt, 0)
		}
		waiter.Messages = append(waiter.Messages, messageID)
		waiter.ReadSequences = append(waiter.ReadSequences, sequence)
		if readAt == 0 {
			readAt = now
		}
		waiter.ReadAt = append(waiter.ReadAt, readAt)
	}
	return writeWaiter(file, waiter)
}

// writeWaiter writes a wait record: the run, when the wait began, the
// messages, and for each message its read sequence and when it was read. The
// read times follow the sequences, so a record with times has sequences too;
// a sequence of 0 is one not known, as a missing one is.
func writeWaiter(file string, waiter Waiter) error {
	return world{files: osAccess{}}.writeWaiter(file, waiter)
}

func (w world) writeWaiter(file string, waiter Waiter) error {
	record := waiter.Epoch + " " + strconv.FormatInt(waiter.Since, 10) + " " + strings.Join(waiter.Messages, ",")
	timed := len(waiter.ReadAt) == len(waiter.Messages) && len(waiter.Messages) > 0
	if timed || len(waiter.ReadSequences) == len(waiter.Messages) && len(waiter.Messages) > 0 {
		seqs := make([]string, len(waiter.Messages))
		for i := range seqs {
			var seq uint64
			if i < len(waiter.ReadSequences) {
				seq = waiter.ReadSequences[i]
			}
			seqs[i] = strconv.FormatUint(seq, 10)
		}
		record += " " + strings.Join(seqs, ",")
	}
	if timed {
		times := make([]string, len(waiter.ReadAt))
		for i, at := range waiter.ReadAt {
			times[i] = strconv.FormatInt(at, 10)
		}
		record += " " + strings.Join(times, ",")
	}
	return w.writeFile(file, []byte(strings.TrimSpace(record)))
}

// Waiters is ReadWaiters without its error, for tests that look at records
// they wrote themselves. Nothing in the mail paths calls it (a test says so):
// it passes over a record it could not read, which is no answer that nobody
// waits.
func Waiters(dir, name, epoch string) []Waiter {
	waiters, _ := ReadWaiters(dir, name, epoch)
	return waiters
}

// ReadWaiters lists who waits for the end of this run's turn. Nothing is
// forgotten here: a waiter is cleared only once the report to it is written.
// It answers the waiters it read and an error for any it could not, since a
// record that cannot be read may be a waiter.
func ReadWaiters(dir, name, epoch string) ([]Waiter, error) {
	return live(dir).readWaiters(name, epoch)
}

func (w world) readWaiters(name, epoch string) ([]Waiter, error) {
	path, ok := awaitingPath(w.dir, name, epoch)
	if !ok {
		return nil, nil
	}
	entries, err := w.readDir(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var unknown error
	waiters := make([]Waiter, 0, len(entries))
	for _, entry := range entries {
		peer := entry.Name()
		if entry.IsDir() || !state.ValidName(peer) {
			continue
		}
		raw, err := w.readFile(filepath.Join(path, peer))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			unknown = err
			continue
		}
		waiter, err := parseWaiter(peer, string(raw))
		if err != nil {
			unknown = unknownRecord(filepath.Join(path, peer), err)
			continue
		}
		waiters = append(waiters, waiter)
	}
	sort.Slice(waiters, func(i, j int) bool { return waiters[i].Name < waiters[j].Name })
	return waiters, unknown
}

// parseWaiter reads a wait record as writeWaiter writes it. A record with the
// run alone comes from before the time was kept. Anything else that does not
// parse is an error: read as zeros, a damaged record owes nothing, and the
// task it names would count as reported.
func parseWaiter(name, raw string) (Waiter, error) {
	fields := strings.Fields(raw)
	waiter := Waiter{Name: name}
	if len(fields) == 0 || len(fields) > 5 {
		return Waiter{}, fmt.Errorf("the wait record of %s is not readable: %d fields", name, len(fields))
	}
	waiter.Epoch = fields[0]
	var err error
	if len(fields) > 1 {
		if waiter.Since, err = strconv.ParseInt(fields[1], 10, 64); err != nil {
			return Waiter{}, fmt.Errorf("the wait record of %s is not readable: %w", name, err)
		}
	}
	if len(fields) > 2 {
		waiter.Messages = strings.Split(fields[2], ",")
	}
	if len(fields) > 3 {
		for _, value := range strings.Split(fields[3], ",") {
			seq, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				return Waiter{}, fmt.Errorf("the wait record of %s is not readable: %w", name, err)
			}
			waiter.ReadSequences = append(waiter.ReadSequences, seq)
		}
	}
	if len(fields) > 4 {
		for _, value := range strings.Split(fields[4], ",") {
			at, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return Waiter{}, fmt.Errorf("the wait record of %s is not readable: %w", name, err)
			}
			waiter.ReadAt = append(waiter.ReadAt, at)
		}
	}
	return waiter, nil
}

// ClearAwaiting forgets a waiter once it has been reported to, or once its run
// has ended. Only that run is forgotten: a newer run of the same name that
// wrote in the meantime is still owed its report. Later messages from the same
// wait are retained; only the published subset is removed. The caller holds the mailbox
// lock, which is what keeps the check and the removal together.
//
// An error says the clearing is not done: a record that cannot be read or
// changed still names what was reported, and the next task read from the same
// sender would join it and be reported with it a second time. The caller keeps
// the clearing to finish before the record is used again.
func ClearAwaiting(dir, name, epoch string, peer Waiter) error {
	return live(dir).clearAwaiting(name, epoch, peer)
}

func (w world) clearAwaiting(name, epoch string, peer Waiter) error {
	path, ok := awaitingPath(w.dir, name, epoch)
	if !ok || !state.ValidName(peer.Name) {
		return nil
	}
	file := filepath.Join(path, peer.Name)
	raw, err := w.readFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	current, err := parseWaiter(peer.Name, string(raw))
	if err != nil {
		return unknownRecord(file, err)
	}
	if current.same(peer) {
		return w.remove(file)
	}
	if current.Epoch != peer.Epoch || current.Since != peer.Since {
		return nil
	}
	remaining := make([]string, 0, len(current.Messages))
	sequences := make([]uint64, 0, len(current.Messages))
	times := make([]int64, 0, len(current.Messages))
	for i, id := range current.Messages {
		if !slices.Contains(peer.Messages, id) {
			remaining = append(remaining, id)
			if i < len(current.ReadSequences) {
				sequences = append(sequences, current.ReadSequences[i])
			}
			times = append(times, current.readAt(i))
		}
	}
	if len(remaining) == len(current.Messages) {
		return nil
	}
	if len(remaining) == 0 {
		return w.remove(file)
	}
	current.Messages = remaining
	current.ReadSequences = sequences
	current.ReadAt = times
	return w.writeWaiter(file, current)
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
