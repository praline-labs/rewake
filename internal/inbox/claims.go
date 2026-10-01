package inbox

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/state"
)

// A letter a tool call started showing in parts is being read: part of its
// text may be in front of the agent already, so it is no longer unread text a
// sender may take back or replace, and it is not read either until every part
// was shown whole (docs/mail-bridge.md#bounded-output-and-the-read-boundary).
// The claim says so to withdraw, edit and the sweep; reading the letter to the
// end, by the tool or the shell, removes it.

// ErrReadInProgress refuses to withdraw a letter being read in parts. It is a
// kind of ErrAlreadyRead: whatever treats a read letter as final treats this
// one so too.
var ErrReadInProgress = fmt.Errorf("%w, in parts", ErrAlreadyRead)

func claimsPath(dir, name string) string { return filepath.Join(state.InboxPath(dir, name), "claims") }

// ClaimRead marks a letter as being read, by the read with this receipt. The
// caller holds the mailbox lock.
func ClaimRead(dir, name, id, token string) error {
	if !safeID(id) {
		return fmt.Errorf("invalid letter id %q", id)
	}
	if err := state.EnsureSubdir(claimsPath(dir, name)); err != nil {
		return err
	}
	return state.WriteAtomic(filepath.Join(claimsPath(dir, name), id), []byte(token))
}

// ReadClaimed says whether a letter is being read in parts, and by which read.
// A claim that cannot be read may stand, and everything that asks keeps the
// letter as it keeps a claimed one, so it counts as claimed by a read unnamed.
func ReadClaimed(dir, name, id string) (string, bool) {
	if !safeID(id) {
		return "", false
	}
	raw, err := os.ReadFile(filepath.Join(claimsPath(dir, name), id))
	if errors.Is(err, os.ErrNotExist) {
		return "", false
	}
	if err != nil {
		return "", true
	}
	return strings.TrimSpace(string(raw)), true
}

// claimed says a tool read has started showing a letter in parts, or may have.
func claimed(dir, name, id string) bool {
	_, ok := ReadClaimed(dir, name, id)
	return ok
}

// dropClaim ends a claim once its letter is read.
func dropClaim(dir, name, id string) {
	if !safeID(id) {
		return
	}
	if err := os.Remove(filepath.Join(claimsPath(dir, name), id)); err == nil {
		_ = state.SyncDir(claimsPath(dir, name))
	}
}

// sweepClaims removes claims whose letter is no longer unread and that are
// older than cutoff. A claim on an unread letter stays however old: what the
// agent saw of it is uncertain, and the letter must not become replaceable.
func sweepClaims(dir, name string, cutoff time.Time) {
	entries, err := os.ReadDir(claimsPath(dir, name))
	if err != nil {
		return
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		if _, err := os.Stat(filepath.Join(state.UnreadPath(dir, name), entry.Name()+".json")); !errors.Is(err, os.ErrNotExist) {
			continue
		}
		_ = os.Remove(filepath.Join(claimsPath(dir, name), entry.Name()))
	}
}

// StillUnread says whether a letter is still in unread/. A letter leaves it
// once, by being read or by its delivery's end, and never comes back; one
// shown in part leaves it only by being read, so for such a letter this is
// the durable answer to whether its read is complete, whichever channel or
// receipt completed it. The caller holds the mailbox lock.
func StillUnread(dir, name, id string) bool {
	return stillUnread(dir, name, id)
}

func stillUnread(dir, name, id string) bool {
	if !safeID(id) {
		return false
	}
	// A copy that cannot be looked at is not proven gone.
	_, err := os.Stat(filepath.Join(state.UnreadPath(dir, name), id+".json"))
	return !errors.Is(err, os.ErrNotExist)
}

// ClaimedOwedBy names the senders of the letters being read in parts that owe
// a report: the read is not recorded until the wrapper confirms its last part,
// and a pending mark made meanwhile must know who waits. Each claimed letter is
// looked up by itself, and an error says a claim or its letter could not be
// read, which is no answer that nobody waits.
func ClaimedOwedBy(dir, name, epoch string) ([]string, error) {
	entries, err := os.ReadDir(claimsPath(dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var senders []string
	for _, entry := range entries {
		message, unread, err := UnreadCopy(dir, name, epoch, entry.Name())
		if err != nil {
			return nil, err
		}
		// A claim whose letter left unread/ was read to the end.
		if unread && Owed(message) && !slices.Contains(senders, message.From) {
			senders = append(senders, message.From)
		}
	}
	return senders, nil
}
