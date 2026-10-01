/*
Package receipt is the journal of the mail operations a run performed through
the words a tool call may carry (docs/mail-bridge.md#receipts-retries-and-deadlines).

An operation is journaled before it changes anything, and its record holds what
it has done so far and what it answered. A repeated call finds the record by its
key — the run, the native conversation and turn, and the digest of the
normalized words — and a timed-out one by its token, so the effect happens once
whatever the transport lost. Native call ids are not the key: a retry of the
same words gets a new one.

Records live beside the mailbox, one directory per run: a token names a record
of this run and no other, so it cannot reach a replacement's.
*/
package receipt

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/praline-labs/rewake/internal/state"
)

// Key scopes an operation. Two calls with the same key are one operation.
type Key struct {
	Epoch        string
	Conversation string
	Turn         string
	Digest       string
}

// Scoped says whether the key can join a repeated call. A shell call knows no
// native turn, so its operations are journaled but never joined by words.
func (k Key) Scoped() bool {
	return k.Epoch != "" && k.Conversation != "" && k.Turn != "" && k.Digest != ""
}

func (k Key) index() string {
	sum := sha256.Sum256([]byte(k.Epoch + "\x00" + k.Conversation + "\x00" + k.Turn + "\x00" + k.Digest))
	return "key-" + hex.EncodeToString(sum[:16])
}

// Phase is how far an operation got.
type Phase string

const (
	// Open is journaled and not finished: its effect may be done in part.
	Open Phase = "open"
	// Done holds its answer.
	Done Phase = "done"
)

// Transports that journal.
const (
	Shell = "shell"
)

// Record is one operation.
type Record struct {
	Version      int      `json:"version"`
	Token        string   `json:"token"`
	Epoch        string   `json:"epoch"`
	Conversation string   `json:"conversation,omitempty"`
	Turn         string   `json:"turn,omitempty"`
	Digest       string   `json:"digest"`
	Words        []string `json:"words"`
	Transport    string   `json:"transport"`
	Calls        []string `json:"calls,omitempty"`
	CalledBoot   int64    `json:"calledBoot,omitempty"`
	Phase        Phase    `json:"phase"`
	// Uncertain marks a finished operation whose effect nobody can prove
	// either way any more; it is kept while its run lives.
	Uncertain bool      `json:"uncertain,omitempty"`
	Created   time.Time `json:"created"`
	Updated   time.Time `json:"updated"`
	// Outcome is the answer of a finished operation, replayed to a repeat.
	Outcome *Outcome `json:"outcome,omitempty"`
	// Notify, Pending, Read and Output hold the steps of each kind.
	Notify  *NotifyStep  `json:"notify,omitempty"`
	Pending *PendingStep `json:"pending,omitempty"`
	Read    *ReadBatch   `json:"read,omitempty"`
	Output  *FrozenText  `json:"output,omitempty"`
}

// version is the record format this build writes and reads.
const version = 1

// Outcome is what an operation answered.
type Outcome struct {
	Exit   int    `json:"exit"`
	Stdout string `json:"stdout,omitempty"`
	Stderr string `json:"stderr,omitempty"`
}

// NotifyStep is a heads-up's progress: its id and text are fixed before it is
// published, and the recipient's run is pinned, so a retry publishes the
// same letter to the same run or nothing.
type NotifyStep struct {
	MessageID string `json:"messageId"`
	To        string `json:"to"`
	ToEpoch   string `json:"toEpoch"`
	// Text is the letter as first read, stdin included: a retry never reads
	// its input again.
	Text      string `json:"text"`
	Published bool   `json:"published,omitempty"`
}

// PendingStep is a pending mark's progress, tied to the time of the call
// that made it rather than to the process that resumes it.
type PendingStep struct {
	Text string `json:"text"`
	At   int64  `json:"at"`
	// Mark is the file the call's mark is kept in, drawn once, so a retry
	// finds the mark its first attempt wrote and never makes a second.
	Mark   string `json:"mark,omitempty"`
	Marked bool   `json:"marked,omitempty"`
}

// ErrBusy means another process holds the record: the same operation is
// running now.
var ErrBusy = errors.New("the operation is running in another call")

// ErrUnknown means no record answers to a token in this run.
var ErrUnknown = errors.New("no such receipt in this run")

var tokenShape = regexp.MustCompile(`^[0-9a-f]{24}$`)

// ValidToken says whether a string can be a token: a record selector, never a
// path.
func ValidToken(token string) bool { return tokenShape.MatchString(token) }

func newToken() string {
	var raw [12]byte
	_, _ = rand.Read(raw[:])
	return hex.EncodeToString(raw[:])
}

// Path is the journal of one run of a mailbox.
func Path(dir, name, epoch string) (string, error) {
	if epoch == "" || strings.ContainsAny(epoch, `/\`) || strings.HasPrefix(epoch, ".") {
		return "", fmt.Errorf("invalid run %q", epoch)
	}
	return filepath.Join(state.InboxPath(dir, name), "receipts", epoch), nil
}

func recordPath(journal, token string) string { return filepath.Join(journal, token+".json") }

// Begin finds the record of a scoped key, or journals a new one from fresh.
// It answers whether the record existed: a repeat joins the first call's.
func Begin(dir, name string, key Key, fresh Record) (Record, bool, error) {
	journal, err := Path(dir, name, key.Epoch)
	if err != nil {
		return Record{}, false, err
	}
	if err := state.EnsureSubdir(filepath.Dir(journal)); err != nil {
		return Record{}, false, err
	}
	if err := state.EnsureSubdir(journal); err != nil {
		return Record{}, false, err
	}
	if key.Scoped() {
		if token, err := os.ReadFile(filepath.Join(journal, key.index())); err == nil {
			record, err := Load(dir, name, key.Epoch, strings.TrimSpace(string(token)))
			return record, err == nil, err
		}
	}
	now := time.Now()
	fresh.Version, fresh.Token, fresh.Phase, fresh.Created, fresh.Updated = version, newToken(), Open, now, now
	fresh.Epoch, fresh.Conversation, fresh.Turn, fresh.Digest = key.Epoch, key.Conversation, key.Turn, key.Digest
	encoded, err := json.Marshal(fresh)
	if err != nil {
		return Record{}, false, err
	}
	if err := state.PublishExclusive(recordPath(journal, fresh.Token), encoded); err != nil {
		return Record{}, false, err
	}
	if !key.Scoped() {
		return fresh, false, nil
	}
	// The index decides between two calls racing with the same key: the one
	// whose link lands owns the operation, the other drops its record and
	// joins.
	err = state.PublishExclusive(filepath.Join(journal, key.index()), []byte(fresh.Token))
	if errors.Is(err, state.ErrNameTaken) {
		_ = os.Remove(recordPath(journal, fresh.Token))
		token, readErr := os.ReadFile(filepath.Join(journal, key.index()))
		if readErr != nil {
			return Record{}, false, readErr
		}
		record, loadErr := Load(dir, name, key.Epoch, strings.TrimSpace(string(token)))
		return record, loadErr == nil, loadErr
	}
	if err != nil {
		_ = os.Remove(recordPath(journal, fresh.Token))
		return Record{}, false, err
	}
	return fresh, false, nil
}

// Load reads a record of one run by its token.
func Load(dir, name, epoch, token string) (Record, error) {
	if !ValidToken(token) {
		return Record{}, ErrUnknown
	}
	journal, err := Path(dir, name, epoch)
	if err != nil {
		return Record{}, err
	}
	raw, err := os.ReadFile(recordPath(journal, token))
	if errors.Is(err, os.ErrNotExist) {
		return Record{}, ErrUnknown
	}
	if err != nil {
		return Record{}, err
	}
	var record Record
	if err := json.Unmarshal(raw, &record); err != nil {
		return Record{}, fmt.Errorf("the receipt %s is not readable: %w", token, err)
	}
	if record.Version != version || record.Token != token || record.Epoch != epoch {
		return Record{}, fmt.Errorf("the receipt %s was written by another rewake or run", token)
	}
	return record, nil
}

// Save writes a record back. The caller holds its lock.
func Save(dir, name string, record Record) error {
	journal, err := Path(dir, name, record.Epoch)
	if err != nil {
		return err
	}
	record.Updated = time.Now()
	encoded, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return state.WriteAtomic(recordPath(journal, record.Token), encoded)
}

// Lock holds a record against other calls of the same operation until the
// returned release is called, waiting until ctx ends; ErrBusy then.
//
// The sweep removes a lock file while holding it. A call that was waiting on
// the removed file then holds a lock nobody else can find, while the next call
// creates a new file and takes that one too; so a call that got the lock checks
// that its file is still the one at the path, and starts over when it is not.
func Lock(ctx context.Context, dir, name, epoch, token string) (func(), error) {
	journal, err := Path(dir, name, epoch)
	if err != nil {
		return nil, err
	}
	path := lockPath(journal, token)
	for {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
		if err != nil {
			return nil, err
		}
		if err := flock(ctx, file); err != nil {
			_ = file.Close()
			return nil, err
		}
		same, err := samePath(file, path)
		if same {
			return func() {
				_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
				_ = file.Close()
			}, nil
		}
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
		if err != nil {
			return nil, err
		}
	}
}

func lockPath(journal, token string) string { return filepath.Join(journal, "."+token+".lock") }

// flock takes an exclusive lock on file, polling until ctx ends.
func flock(ctx context.Context, file *os.File) error {
	for {
		err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return nil
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EINTR {
			return err
		}
		select {
		case <-ctx.Done():
			return ErrBusy
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// samePath says whether an open file is still the one its path names. A path
// removed since is another file's to create, and the caller opens it again; a
// lookup that failed otherwise tells nothing, and the lock is not taken.
func samePath(file *os.File, path string) (bool, error) {
	held, err := file.Stat()
	if err != nil {
		return false, err
	}
	current, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return os.SameFile(held, current), nil
}

// Unresolved lists this run's operations with the digest of the given words
// whose effect is not settled: still open — begun, not proven done or absent —
// or finished with an effect recorded as uncertain. A shell call with the same
// words cannot tell whether it repeats one of them, so it does not run beside
// them. An error says the journal could not be read, which proves no more
// than a match would: the caller does not run either.
func Unresolved(dir, name, epoch, digest string) ([]Record, error) {
	journal, err := Path(dir, name, epoch)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(journal)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var found []Record
	for _, entry := range entries {
		token, ok := strings.CutSuffix(entry.Name(), ".json")
		if !ok || !ValidToken(token) {
			continue
		}
		record, err := Load(dir, name, epoch, token)
		if errors.Is(err, ErrUnknown) {
			// Swept since it was listed: settled.
			continue
		}
		if err != nil {
			return nil, err
		}
		if record.Digest != digest || record.Read != nil || record.Output != nil {
			continue
		}
		if record.Phase == Open || record.Uncertain {
			found = append(found, record)
		}
	}
	return found, nil
}
