package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// recallAnnounceWait bounds the wait for a recall's notice to go out. The
// serving process collects for 150 ms before it announces, so ten seconds is
// room for a loaded machine, not a guess at the usual time.
const recallAnnounceWait = 10 * time.Second

// recallAnnounced waits until the serving process has handed the harness the
// notice of the recall of task — its status says delivered or held — and
// answers why not when it has not.
//
// A scenario that lets the worker read before that has made a race of its
// own: the read takes the tombstone and the recall together, nothing unread is
// left to announce, and the notice the scenario then looks for never comes.
// That is the product doing right, so the scenario waits it out. withdraw
// writes the recall before it returns, so a recall that is not there at all
// is not waited for: that is what a control that silences it looks like.
func recallAnnounced(c *Case, iso *Isolation, worker *scenarioSession, task string) (string, bool) {
	recall := recallFor(iso, worker, task)
	if recall == "" {
		return fmt.Sprintf("no recall of %s was in %s's mailbox when the gate opened", task, worker.name), false
	}
	var state string
	if waitFor(c, recallAnnounceWait, func() bool {
		state = statusState(iso, worker, recall)
		return state == "delivered" || state == "held"
	}) {
		return "", true
	}
	return fmt.Sprintf("the recall %s of %s was still %q after %s, and the gate was opened anyway", recall, task, state, recallAnnounceWait), false
}

// recallFor is the id of the letter in a session's mailbox that recalls task:
// queued in the mailbox itself before its notice goes, in unread/ once it is
// readable, in done/ once read.
func recallFor(iso *Isolation, session *scenarioSession, task string) string {
	for _, box := range []string{"", "unread", "done"} {
		paths, _ := filepath.Glob(mailboxPath(iso, session, box, "*.json"))
		for _, path := range paths {
			raw, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			var letter sentView
			if json.Unmarshal(raw, &letter) == nil && letter.Recall != nil && letter.Recall.ID == task {
				return letter.ID
			}
		}
	}
	return ""
}

// statusState is the state a letter's status file holds, or "" while it has
// none.
func statusState(iso *Isolation, session *scenarioSession, id string) string {
	raw, err := os.ReadFile(mailboxPath(iso, session, id+".status"))
	if err != nil {
		return ""
	}
	var status struct {
		State string `json:"state"`
	}
	_ = json.Unmarshal(raw, &status)
	return status.State
}

// orAnnounced is what a finding says of the wait: why it ended without the
// notice, or that the gate opened after it.
func orAnnounced(unannounced string) string {
	if unannounced == "" {
		return "the gate opened after the recall's notice went out"
	}
	return unannounced
}
