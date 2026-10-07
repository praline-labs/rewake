package cli

import (
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"

	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

// holdTurn decides, under the mailbox lock, whether this turn end is held for
// the session to confirm, and keeps its answer when it is. It answers the
// reason the hook hands the model, or "" when the turn end is published as
// usual.
//
// The case it catches: a turn ended pending, and a later one — woken by a
// subagent or a background task that finished — ends with no mark while the
// senders still wait. The work may be done, or the model may have forgotten
// the mark, and a report sent too early closes the task; so it is asked once.
// Every failure here falls to publishing, which is the behavior without the
// hold (docs/turn-outcomes.md); a record that cannot be read is such a
// failure, and a kept answer that cannot be read is never written over.
//
// op is the end's operation: the kept answer records it with the reason, so
// the same end confirmed again is answered from the record (endTurnContext).
func holdTurn(dir string, self registry.Session, event inbox.TurnEnd, op string, holdable bool, waiters []inbox.Waiter, marked bool) string {
	if !holdable || event.Failed || event.Stopped {
		return ""
	}
	line, interim, err := inbox.LastInterim(dir, self.Name, self.Epoch())
	if err != nil || !interim {
		return ""
	}
	if _, held, err := inbox.KeptAnswer(dir, self.Name, self.Epoch()); err != nil || held {
		// A turn end held before whose continuation was never heard of — an
		// Esc where the plugin did not load. This end publishes that answer
		// with its own, and asks nothing again.
		return ""
	}
	senders, err := liveSenders(dir, waiters)
	if err != nil || len(senders) == 0 || marked {
		return ""
	}
	reason := holdReason(line, senders)
	if inbox.KeepAnswer(dir, self.Name, self.Epoch(), event.Text, op, reason) != nil {
		return ""
	}
	return reason
}

// liveSenders names the waiters whose sessions still run the run that sent the
// task: the ones a report would reach. A sender proven gone — no record, or a
// record of another run — is left out; a lookup that could not be completed
// fails the whole check, since the hold would then be asked of a set the check
// never saw. The fault seam is asked first, so a test fails the lookup like
// the check's other reads.
func liveSenders(dir string, waiters []inbox.Waiter) ([]string, error) {
	var names []string
	for _, waiter := range waiters {
		if err := state.AskRead(state.SessionPath(dir, waiter.Name)); err != nil {
			return nil, err
		}
		peer, err := registry.Lookup(dir, waiter.Name)
		if errors.Is(err, registry.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if peer.Epoch() == waiter.Epoch && !slices.Contains(names, peer.Name) {
			names = append(names, peer.Name)
		}
	}
	return names, nil
}

// holdReason is what the model reads after its turn was held. Claude Code
// shows it as a hook's blocking error, and it is the only thing the model sees
// of rewake there, so it names both ways on and what each sends.
func holdReason(line string, senders []string) string {
	line, _, _ = strings.Cut(line, "\n")
	return "Rewake: your previous turn ended pending (\"" + line + "\"), and " + strings.Join(senders, ", ") +
		" still wait for your report. If the work still waits on something, run rewake pending \"<what it waits for>\" and end the turn. " +
		"If it is done, end the turn: the answer you just gave goes to them as the report, followed by whatever you say now, so do not repeat it."
}

// printHold tells the harness to hold the turn: Claude Code's Stop hook
// decision, read from the hook's stdout.
func printHold(out io.Writer, reason string) {
	raw, err := json.Marshal(struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}{"block", reason})
	if err == nil {
		_, _ = out.Write(append(raw, '\n'))
	}
}

// joinTurnText puts two parts of one outcome's text together, either of which
// may be empty.
func joinTurnText(first, second string) string {
	first, second = strings.TrimSpace(first), strings.TrimSpace(second)
	switch {
	case first == "":
		return second
	case second == "":
		return first
	}
	return first + "\n\n" + second
}
