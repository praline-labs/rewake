package cli

import (
	"encoding/json"
	"io"
	"slices"
	"strings"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
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
// hold (docs/turn-outcomes.md).
func holdTurn(dir string, self registry.Session, event turnResult, waiters []inbox.Waiter) string {
	if !event.Holdable || event.Failed || event.Stopped {
		return ""
	}
	line, interim := inbox.LastInterim(dir, self.Name, self.Epoch())
	if !interim {
		return ""
	}
	if _, held := inbox.KeptAnswer(dir, self.Name, self.Epoch()); held {
		// A turn end held before whose continuation was never heard of — an
		// Esc where the plugin did not load. This end publishes that answer
		// with its own, and asks nothing again.
		return ""
	}
	senders := liveSenders(dir, waiters)
	if len(senders) == 0 || inbox.MarkedWithin(dir, self.Name, self.Epoch(), event.Started, event.Ended) {
		return ""
	}
	if inbox.KeepAnswer(dir, self.Name, self.Epoch(), event.Text) != nil {
		return ""
	}
	return holdReason(line, senders)
}

// liveSenders names the waiters whose sessions still run the run that sent the
// task: the ones a report would reach.
func liveSenders(dir string, waiters []inbox.Waiter) []string {
	var names []string
	for _, waiter := range waiters {
		peer, err := registry.Lookup(dir, waiter.Name)
		if err == nil && peer.Epoch() == waiter.Epoch && !slices.Contains(names, peer.Name) {
			names = append(names, peer.Name)
		}
	}
	return names
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
