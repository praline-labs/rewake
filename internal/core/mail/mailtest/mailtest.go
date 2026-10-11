// Package mailtest gives tests outside core/mail the paths and bytes of mail's
// records that its exported API does not hand out: where a status or a claim
// lies, what a delivery-thread record says, whether a copy is a tombstone, a
// wait written as mail writes one. They are fixtures, not a second mail: each
// is checked by a test in core/mail against mail's own reader or writer
// (mailtest_check_test.go), and nothing here settles, delivers or decides.
//
// The package imports no mail, so mail's own tests may use it too.
package mailtest

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// PastRetention is an age past which the sweep of finished mail retires what
// it keeps: twice the retention, as mail's own tests age their records.
const PastRetention = 48 * time.Hour

// StaleAnswering is an age at which a mark of a command waiting for an answer
// no longer reads as that command alive: twice the freshness mail allows it.
const StaleAnswering = 6 * time.Second

// StatusPath is where a message's status lies.
func StatusPath(dir, name, id string) string {
	return filepath.Join(state.InboxPath(dir, name), id+".status")
}

// ClaimsPath is the directory of a mailbox's reads in parts.
func ClaimsPath(dir, name string) string { return filepath.Join(state.InboxPath(dir, name), "claims") }

// AnswerReceiptsPath is the directory of the receipts of answers taken.
func AnswerReceiptsPath(dir, name string) string {
	return filepath.Join(state.InboxPath(dir, name), "received")
}

// DeliveryThread is the conversation a message was delivered into, "" when no
// record is there.
func DeliveryThread(dir, name, id string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(state.InboxPath(dir, name), "threads", id))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return string(raw), err
}

// WriteDeliveryThread records the conversation a message was delivered into,
// as the delivery server does when it pins one.
func WriteDeliveryThread(dir, name, id, thread string) error {
	threads := filepath.Join(state.InboxPath(dir, name), "threads")
	if err := os.MkdirAll(threads, 0o700); err != nil {
		return err
	}
	return state.WriteAtomic(filepath.Join(threads, id), []byte(thread))
}

// IsTombstone says the copy of a message in a directory is a tombstone, false
// when there is no copy.
func IsTombstone(directory, id string) (bool, error) {
	raw, err := os.ReadFile(filepath.Join(directory, id+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var copy struct {
		Withdrawn json.RawMessage `json:"withdrawn"`
	}
	if err := json.Unmarshal(raw, &copy); err != nil {
		return false, err
	}
	return len(copy.Withdrawn) > 0 && string(copy.Withdrawn) != "null", nil
}

// Owe writes the wait of from's run fromEpoch on name's run epoch for ids, as
// a run that read them now records it: no read sequence known, each read now.
func Owe(dir, name, epoch, from, fromEpoch string, ids ...string) error {
	runs := filepath.Join(state.AwaitingPath(dir, name), epoch)
	if err := os.MkdirAll(runs, 0o700); err != nil {
		return err
	}
	now := strconv.FormatInt(time.Now().UnixNano(), 10)
	seqs, times := make([]string, len(ids)), make([]string, len(ids))
	for i := range ids {
		seqs[i], times[i] = "0", now
	}
	record := strings.Join([]string{fromEpoch, now, strings.Join(ids, ","), strings.Join(seqs, ","), strings.Join(times, ",")}, " ")
	return state.WriteAtomic(filepath.Join(runs, from), []byte(record))
}
