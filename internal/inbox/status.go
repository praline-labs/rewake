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

// Every lookup a mail path decides by answers one of three things: found,
// proven absent, or an error saying it could not tell (rule 6 of
// docs/mail-bridge-cli.md#the-rules-the-code-holds). Only "no such file" is
// absence: a file that cannot be read or does not parse may hold exactly what
// the caller was looking for, and each caller decides what an unknown stops.

// isIn says whether a directory holds a message's file.
func isIn(directory, id string) (bool, error) {
	_, err := os.Stat(filepath.Join(directory, id+".json"))
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, os.ErrNotExist):
		return false, nil
	}
	return false, err
}

// Answered reports whether this message has left the mailbox — delivered or
// refused — even if its status is no longer kept. Answers are swept after a
// while, and a sender that came back later would otherwise be told its message
// is still on its way. An error says the mailbox could not tell.
func Answered(dir, to, id string) (bool, error) {
	waiting, err := isIn(state.InboxPath(dir, to), id)
	if err != nil || waiting {
		return false, err
	}
	// Gone from done/ as well is answered long ago and swept since.
	if _, err := isIn(state.DonePath(dir, to), id); err != nil {
		return false, err
	}
	return true, nil
}

// ReadStatus returns the status of a message, and whether one has been
// written. An error says it could not tell: a status that cannot be read may
// say read or withdrawn, and nothing takes it for one never written.
func ReadStatus(dir, to, id string) (Status, bool, error) {
	raw, err := state.ReadFile(statusPath(dir, to, id))
	if errors.Is(err, os.ErrNotExist) {
		return Status{}, false, nil
	}
	if err != nil {
		return Status{}, false, err
	}
	var status Status
	if err := json.Unmarshal(raw, &status); err != nil {
		return Status{}, false, fmt.Errorf("the status of %s is not readable: %w", id, err)
	}
	return status, true, nil
}

// statusPoll is how often a sender looks for its answer. It is short because the
// wait is short and bounded by --wait: the server's own tick is the slow one, and
// borrowing it here made every delivery look like it took a second.
const statusPoll = 25 * time.Millisecond

// runningPoll is how often a waiting sender asks whether the recipient still
// runs: a registry read and two process checks, too much for every statusPoll.
const runningPoll = 500 * time.Millisecond

// Await waits for a status until the deadline, and returns the last one seen.
// A message with no status yet is not lost: the mailbox is durable, and the
// serving process writes one as soon as it can. running, when given, cuts the
// wait short once it says the recipient no longer runs: a run that ended will
// write no status, and a long wait for one outlived it by up to an hour. A
// status that cannot be read is none seen yet here, and only here: the wait
// goes on in case it reads again, and a caller that gets no status reads it
// once more with ReadStatus, which says whether it was absent or unknown.
func Await(dir, to, id string, timeout time.Duration, running func() bool) (Status, bool) {
	deadline := time.Now().Add(timeout)
	nextCheck := time.Now().Add(runningPoll)
	for {
		// Held is not an answer yet: a release or an expiry usually follows
		// within the wait, and the last word is the one worth printing.
		status, ok, err := ReadStatus(dir, to, id)
		ok = ok && err == nil
		if ok && status.State != Pending && status.State != Held {
			return status, true
		} else if ok && time.Now().After(deadline) {
			return status, true
		}
		if time.Now().After(deadline) {
			return Status{}, false
		}
		if running != nil && time.Now().After(nextCheck) {
			if !running() {
				// One last look: the status may have been written just
				// before the run ended.
				status, ok, err := ReadStatus(dir, to, id)
				return status, ok && err == nil
			}
			nextCheck = time.Now().Add(runningPoll)
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
	return writeStatusFile(statusPath(dir, to, id), append(encoded, '\n'))
}

// writeStatusFile writes a status; a test makes it fail while the status on
// disk stays readable.
var writeStatusFile = state.WriteAtomic

func statusPath(dir, to, id string) string {
	return filepath.Join(state.InboxPath(dir, to), id+".status")
}
