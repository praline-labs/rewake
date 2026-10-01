package inbox

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/role"
)

// tellMain leaves main a note once: its id is derived from what it is about,
// and it goes under the marks of main's mailbox (publishMarked), so a barrier
// that runs again writes no second copy. A room without a main, or with one of
// the earlier build, which this build does not write to, is told nothing: the
// cause stays in the mailbox, and every call that meets it names it.
func tellMain(dir, from, about, text string) error { return live(dir).tellMain(from, about, text) }

func (w world) tellMain(from, about, text string) error {
	sessions, err := w.sessions()
	if err != nil {
		return err
	}
	note, ok := mainNote(sessions, from, about, text)
	if !ok {
		return nil
	}
	if note.To == from {
		return w.publishMarked(note, world.putLocal)
	}
	return w.publishMarked(note, world.putOnce)
}

// mainNote is the note to the room's main about one thing; false when the
// room has no main of this build.
func mainNote(sessions []registry.Session, from, about, text string) (Message, bool) {
	for _, session := range sessions {
		if role.Of(session.Role).ID != role.Main.ID || session.EarlierBuild() {
			continue
		}
		sum := sha256.Sum256([]byte(from + "\x00" + about))
		return Message{
			ID: fmt.Sprintf("%019d-%x", 0, sum[:6]), From: from, To: session.Name, ToEpoch: session.Epoch(),
			Kind: Note, Text: text, CreatedAt: time.Now(),
		}, true
	}
	return Message{}, false
}

// heldMootAbout is what a note of a moot held report is about.
const heldMootAbout = "held-moot\x00"

// tellMainHeldMoot tells main that a report held for a run of the earlier
// build can reach nobody: the name's successor ended before taking it.
func (w world) tellMainHeldMoot(_ context.Context, name string, report Message) error {
	return w.tellMain(name, heldMootAbout+report.ID, fmt.Sprintf(
		"Rewake: a report of %s for %s's run %s, which rewake held after the upgrade, reached nobody: the run of this build that took over %s ended before taking it. What it answered (%d messages) is not reported again.",
		name, report.To, report.ToEpoch, report.To, len(report.InReplyTo)))
}

// TellMainUpgraded tells main that a run of the earlier build was refused:
// once per run, by a note its epoch names (docs/protocol-cutover.md).
func TellMainUpgraded(dir, name, epoch string) error {
	return tellMain(dir, name, "upgraded\x00"+epoch, fmt.Sprintf(
		"Rewake: %s was started by a rewake build before this one, and rewake was upgraded since: its turn ends, reads and sends are refused until it is restarted by resuming its conversation. Nothing is lost; the resumed run takes over what it read.",
		name))
}
