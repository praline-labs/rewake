package inbox

import (
	"errors"
	"path/filepath"

	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

// deliverReport takes one report of a journal to its recipient, and records
// in the journal what came of it before anything else is done: to a running
// run it is published, under its marks; to a run that has ended or was
// replaced it is moot: it can reach nobody.
func (w world) deliverReport(name string, save func() error, journal *TurnJournal, report Message) error {
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
