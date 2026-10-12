package mail

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// removeWaiting drops the waiting copy of an announced message; replaceable in
// tests, which need that step to fail.
var removeWaiting = state.Discard

// SettleLocked takes a message with an outcome out of the waiting set. One the harness
// was told about stays readable in unread/ — or has been read already — so only
// the waiting copy goes. A refused one is taken back from unread/ and archived.
// The caller holds the mailbox lock: a server whose lock cannot be taken
// settles nothing (the delivery server's lockOrAlone). The delivery server's
// settleOutcome and settleUnreadOnly call it inside their mailbox section; it
// takes no lock of its own, since the mailbox lock is not reentrant. Its
// callers are held by internal/layout_callers_test.go.
//
// Settling never removes a letter's last copy. The letter is the proof that a
// publication landed while its mark may still say only intent, and retiring
// that proof is the sweep's alone, under the lock and by the live run
// (docs/v2/stage3-publication.md#the-contract), which settling is not. So a
// copy goes only beside another that stands, and a copy whose archive failed
// stays where it is for the next pass.
//
// Nor does it remove or move a copy an open occurrence of the mailbox's stop
// names, or one inside a directory an occurrence names (stop.go): that copy
// is what the occurrence waits to read again, and one settled away would
// leave its stop to an operator. The letter stays where it is, its outcome
// recorded, and a pass after the stop is resolved settles it; letters the
// stop does not name settle as ever.
//
// It answers whether the letter now stands where its outcome puts it. A
// letter it left — named by a stop, or its archive or removal failed — is
// still owed, and the passes find it again by its status and its copies
// (settleRecorded), not by anything remembered here.
func SettleLocked(dir, name, id string, outcome State) bool {
	kept := keptByStopOf(dir, name)
	waiting := filepath.Join(state.InboxPath(dir, name), id+".json")
	switch outcome {
	case Held:
		// Neither delivered nor refused yet, so both copies stay: its status
		// keeps it from being announced again, and the readable copy is there
		// for an agent that reads its mail on its own.
		return true
	case Delivered, Read, withdrawn:
		// A withdrawal takes the waiting copy itself; one left behind goes
		// here, and the tombstone stays where its reader will look.
		if kept.keeps(waiting) || !standsIn(id, state.UnreadPath(dir, name), state.DonePath(dir, name)) {
			return Gone(waiting)
		}
		if err := removeWaiting(waiting); err == nil {
			_ = state.SyncDir(state.InboxPath(dir, name))
		}
		return Gone(waiting)
	default:
		unread := filepath.Join(state.UnreadPath(dir, name), id+".json")
		if Claimed(dir, name, id) {
			// Only a read takes a letter being read out of unread/; a failed
			// status an earlier run left behind does not. The read settles it.
			return true
		}
		if kept.keeps(waiting) || kept.keeps(unread) || kept.names(filepath.Join(state.DonePath(dir, name), id+".json")) {
			return false
		}
		// A notice taken back after it was reported delivered has lost its
		// waiting copy; the readable one is then the last, and goes to done/.
		if err := move(id, state.InboxPath(dir, name), state.DonePath(dir, name)); errors.Is(err, os.ErrNotExist) {
			_ = move(id, state.UnreadPath(dir, name), state.DonePath(dir, name))
		}
		if standsIn(id, state.InboxPath(dir, name), state.DonePath(dir, name)) {
			dropUnread(dir, name, id)
		}
		return Gone(waiting) && Gone(unread)
	}
}

// Gone says whether nothing stands at the path. One that cannot be looked at
// may still be there. The delivery server asks it whether a letter left in
// unread/ still has its waiting copy.
func Gone(path string) bool {
	_, err := os.Lstat(path)
	return errors.Is(err, os.ErrNotExist)
}

// standsIn says whether a copy of the letter stands in one of the stages:
// a letter, its tombstone, or the link unread/ shares with the waiting copy,
// each a regular file. A directory or a symbolic link at the path is no copy,
// whatever it points at, and neither is a stage that cannot be looked at.
func standsIn(id string, stages ...string) bool {
	for _, stage := range stages {
		if info, err := os.Lstat(filepath.Join(stage, id+".json")); err == nil && info.Mode().IsRegular() {
			return true
		}
	}
	return false
}
