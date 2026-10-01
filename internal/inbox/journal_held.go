package inbox

import (
	"context"
	"errors"
	"path/filepath"
	"slices"

	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

// deliverReport takes one report of a journal to its recipient, and records
// in the journal what came of it before anything else is done:
//
//   - to a running run of this build it is published, under its marks;
//   - to a run of this build that has ended or was replaced it is moot: it
//     can reach nobody;
//   - to a run of the earlier build it is held, since this build writes
//     nothing into that build's mailbox, and the name's successor decides it
//     (docs/protocol-cutover.md): held while none is bound, while it starts
//     and while its state cannot be read, published to it once it is ready,
//     and moot only once it is gone, with main told once.
func (w world) deliverReport(ctx context.Context, name string, save func() error, journal *TurnJournal, report Message) error {
	if !registry.EarlierBuildEpoch(report.ToEpoch) {
		published, err := w.publishReport(name, report)
		if err != nil {
			return err
		}
		if published {
			journal.Published = append(journal.Published, report.ID)
		} else {
			journal.Moot = append(journal.Moot, report.ID)
		}
		return save()
	}
	peer, err := w.lookup(report.To)
	if err != nil && !errors.Is(err, registry.ErrNotFound) {
		return err
	}
	if err == nil && peer.Epoch() == report.ToEpoch {
		return hold(save, journal, report)
	}
	recorded := journal.Successors[report.ID]
	state, successor := heldSuccessor(w.dir, report, recorded)
	switch state {
	case registry.SuccessorGone:
		return w.mootHeld(ctx, name, save, journal, report)
	case registry.SuccessorReady:
	default:
		return hold(save, journal, report)
	}
	if recorded == "" {
		if journal.Successors == nil {
			journal.Successors = map[string]string{}
		}
		journal.Successors[report.ID] = successor
		if err := save(); err != nil {
			return err
		}
	}
	taken := report
	taken.HeldFor, taken.ToEpoch = report.ToEpoch, successor
	published, err := w.publishReport(name, taken)
	if err != nil {
		return err
	}
	if !published {
		// The successor's session was not there to publish to: it ended
		// between the look and the publication, which makes the report moot,
		// or its state cannot be told now, which keeps it held for the next
		// look.
		if state, _, _ := registry.Successor(w.dir, report.To); state == registry.SuccessorGone {
			return w.mootHeld(ctx, name, save, journal, report)
		}
		return hold(save, journal, report)
	}
	journal.Published = append(journal.Published, report.ID)
	journal.Held = slices.DeleteFunc(journal.Held, func(id string) bool { return id == report.ID })
	return save()
}

// heldSuccessor decides which successor takes a report held for a run of the
// earlier build now, and in what state it is. The plan asks it as the
// effect will, so the mark of the recipient chosen during an attempt is read
// before the first report of it goes out. A
// recipient recorded by an attempt that did not finish proves nothing about
// its state now: it may have ended since, so the table decides it again, and
// only a successor still ready takes the report.
func heldSuccessor(dir string, report Message, recorded string) (registry.SuccessorState, string) {
	state, successor, _ := registry.Successor(dir, report.To)
	if recorded != "" && successor != recorded {
		state = registry.SuccessorUnknown
	}
	return state, successor
}

// hold keeps a report in the journal for its recipient's successor.
func hold(save func() error, journal *TurnJournal, report Message) error {
	if slices.Contains(journal.Held, report.ID) {
		return nil
	}
	journal.Held = append(journal.Held, report.ID)
	return save()
}

// mootHeld records that a held report can reach nobody, since the successor
// ended before it was published, and tells main once. Telling main is part
// of the decision, recorded with it: the note owed stays in the journal until
// it is out, so a note that failed, or a crash before it, is sent by the next
// attempt, under the same id (tellMain).
func (w world) mootHeld(ctx context.Context, name string, save func() error, journal *TurnJournal, report Message) error {
	journal.Moot = append(journal.Moot, report.ID)
	journal.Held = slices.DeleteFunc(journal.Held, func(id string) bool { return id == report.ID })
	journal.Notices = append(journal.Notices, report.ID)
	if err := save(); err != nil {
		return err
	}
	return w.tellNotices(ctx, name, save, journal)
}

// tellNotices sends the notes to main the journal owes, dropping each once it
// is out.
func (w world) tellNotices(ctx context.Context, name string, save func() error, journal *TurnJournal) error {
	for len(journal.Notices) > 0 {
		id := journal.Notices[0]
		index := slices.IndexFunc(journal.Reports, func(report Message) bool { return report.ID == id })
		if index >= 0 {
			if err := w.tellMainHeldMoot(ctx, name, journal.Reports[index]); err != nil {
				return err
			}
		}
		journal.Notices = journal.Notices[1:]
		if err := save(); err != nil {
			return err
		}
	}
	return nil
}

// publishReport leaves one report in its recipient's mailbox unless it is
// there already, and answers false, writing nothing, when the recipient's run
// has ended: the report was addressed to that run, not to whoever holds the
// name now.
func (w world) publishReport(name string, report Message) (bool, error) {
	peer, err := w.lookup(report.To)
	if errors.Is(err, registry.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if peer.Epoch() != report.ToEpoch {
		return false, nil
	}
	if report.To == name && len(report.InReplyTo) == 0 {
		// A failed main must not wake itself into another failing turn.
		return true, w.publishMarked(report, world.putLocal)
	}
	return true, w.publishMarked(report, world.putOnce)
}

// publishMarked puts a report under the marks its recipient's mailbox keeps
// of what was published to its run (once.go): "intent" before the report is
// written, "published" after, and the sweep turns an intent into published
// before it removes a report the mark names. So the mailbox proves a report
// was written for as long as its run lives, which is as long as a journal
// could publish to it: a journal that put the report and could not record it
// does not put it again once the recipient has read it and the sweep has
// removed it.
//
// A report found already written, by an attempt that died before its mark, is
// marked published before the journal goes on: the letter is proof only until
// the sweep removes it.
//
// Without the recipient's lock, which a turn end holding its own must not
// wait on: the report is looked for before the mark is read, so a sweep
// removing it in between has already marked it published, or left no mark and
// this one writes it.
func (w world) publishMarked(report Message, put func(world, Message) error) error {
	path, ok := oncePath(w.dir, report.To, report.ToEpoch, report.ID)
	if !ok {
		return put(w, report)
	}
	found, err := w.present(report.To, report.ID)
	if err != nil {
		return err
	}
	mark, err := w.readOnceMark(path)
	if err != nil || mark == oncePublished {
		return err
	}
	if mark == "" {
		if err := w.ensureDir(filepath.Join(state.InboxPath(w.dir, report.To), "once")); err != nil {
			return err
		}
		if err := w.ensureDir(filepath.Dir(path)); err != nil {
			return err
		}
		if !found {
			if err := w.writeFile(path, []byte(onceIntent)); err != nil {
				return err
			}
		}
	}
	if !found {
		if err := put(w, report); err != nil {
			return err
		}
	}
	return w.writeFile(path, []byte(oncePublished))
}
