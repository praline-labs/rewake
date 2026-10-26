package taskbrief

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// The writes. Each runs under the room's brief lock and through the state
// package, so it refuses without the room's lease and in a view. Main writes
// brief.json, the texts and their edits, and moves a brief to the archive; a
// bound run only adds ticks and comments, after checking under the same lock
// that the brief is open, its run is bound and, for a tick, its edition
// current. Who is main is the caller's check.

// New writes a brief of edition 1 and answers it. It is written into a
// temporary directory and renamed into place, so a brief is whole or absent.
func New(dir string, author Author, text string) (Brief, error) {
	if err := CheckText(text); err != nil {
		return Brief{}, err
	}
	var made Brief
	err := state.WithBriefLock(dir, func() error {
		id, err := nextID(dir)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		tmp := filepath.Join(state.BriefsPath(dir), tmpPrefix+rand.Text())
		for _, sub := range []string{logDir, textDir} {
			if err := state.EnsureSubdir(filepath.Join(tmp, sub)); err != nil {
				return err
			}
		}
		if err := state.WriteAtomic(textPath(tmp, 1), []byte(text)); err != nil {
			return err
		}
		edit := Entry{N: 1, At: now, From: author.Name, Epoch: author.Epoch, Kind: KindEdit, Edition: 1}
		if err := publish(tmp, edit); err != nil {
			return err
		}
		if err := writeRecord(tmp, Record{ID: id, Created: now, Bound: []Binding{}}); err != nil {
			return err
		}
		if err := state.Rename(tmp, openPath(dir, id)); err != nil {
			return err
		}
		if err := state.SyncDir(state.BriefsPath(dir)); err != nil {
			return err
		}
		made, err = loadAt(openPath(dir, id), id)
		return err
	})
	return made, err
}

// Edit replaces the text of an open brief, refused unless revision is still
// the brief's: neither a tick nor an edit made since main read it is written
// over. The text is written first and its entry last, so the entry is the
// commit point.
func Edit(dir string, id int, author Author, revision int, text string) (Brief, error) {
	if err := CheckText(text); err != nil {
		return Brief{}, err
	}
	var edited Brief
	err := changeOpen(dir, id, func(path string, brief Brief) error {
		if revision != brief.Revision {
			return &StaleRevisionError{ID: id, Given: revision, Current: brief.Revision}
		}
		edition := brief.Edition + 1
		if err := state.WriteAtomic(textPath(path, edition), []byte(text)); err != nil {
			return err
		}
		entry := Entry{N: next(brief), At: time.Now().UTC(), From: author.Name, Epoch: author.Epoch, Kind: KindEdit, Edition: edition}
		if err := publish(path, entry); err != nil {
			return err
		}
		var err error
		edited, err = loadAt(path, id)
		return err
	})
	return edited, err
}

// Tick sets item n of edition done or not, for a bound run: refused once
// main has edited since that edition, so a number read in an older text
// never ticks another item. It answers the item as it now stands.
func Tick(dir string, id int, author Author, edition, n int, done bool) (Item, error) {
	var ticked Item
	err := changeOpen(dir, id, func(path string, brief Brief) error {
		if !brief.Has(author) {
			return &NotBoundError{ID: id, Name: author.Name}
		}
		if edition != brief.Edition {
			return &StaleEditionError{ID: id, Given: edition, Current: brief.Edition}
		}
		if n < 1 || n > len(brief.items) {
			return &NoItemError{ID: id, Item: n, Items: len(brief.items)}
		}
		kind := KindUntick
		if done {
			kind = KindTick
		}
		ticked = brief.items[n-1]
		ticked.Done = done
		entry := Entry{N: next(brief), At: time.Now().UTC(), From: author.Name, Epoch: author.Epoch, Kind: kind, Edition: edition, Item: n, ItemText: ticked.Text}
		return publish(path, entry)
	})
	return ticked, err
}

// Comment adds a bound run's comment and answers its entry.
func Comment(dir string, id int, author Author, text string) (Entry, error) {
	if err := CheckComment(text); err != nil {
		return Entry{}, err
	}
	var entry Entry
	err := changeOpen(dir, id, func(path string, brief Brief) error {
		if !brief.Has(author) {
			return &NotBoundError{ID: id, Name: author.Name}
		}
		entry = Entry{N: next(brief), At: time.Now().UTC(), From: author.Name, Epoch: author.Epoch, Kind: KindComment, Edition: brief.Edition, Text: text}
		return publish(path, entry)
	})
	return entry, err
}

// Bind gives the brief to a run, or keeps it given; an earlier run of the
// same name stays listed as it was.
func Bind(dir string, id int, run Author) error {
	return changeOpen(dir, id, func(path string, brief Brief) error {
		if brief.Has(run) {
			return nil
		}
		record := brief.Record
		record.Bound = append(record.Bound, Binding{Name: run.Name, Epoch: run.Epoch, Since: time.Now().UTC()})
		return writeRecord(path, record)
	})
}

// Close records the brief closed and moves it to the archive, which ends
// every binding. A close already done answers already; one cut short
// between its two steps is finished.
func Close(dir string, id int) (already bool, err error) {
	err = state.WithBriefLock(dir, func() error {
		path, err := find(dir, id)
		if err != nil {
			return err
		}
		brief, err := loadAt(path, id)
		if err != nil {
			return err
		}
		if path != openPath(dir, id) {
			already = true
			return nil
		}
		if !brief.IsClosed() {
			now := time.Now().UTC()
			record := brief.Record
			record.Closed = &now
			if err := writeRecord(path, record); err != nil {
				return err
			}
		}
		if err := state.EnsureSubdir(state.ClosedBriefsPath(dir)); err != nil {
			return err
		}
		if err := state.Rename(path, filepath.Join(state.ClosedBriefsPath(dir), FormatID(id))); err != nil {
			return err
		}
		if err := state.SyncDir(state.ClosedBriefsPath(dir)); err != nil {
			return err
		}
		return state.SyncDir(state.BriefsPath(dir))
	})
	return already, err
}

// changeOpen runs change under the lock on brief id as it is, refused when
// the brief is closed.
func changeOpen(dir string, id int, change func(path string, brief Brief) error) error {
	return state.WithBriefLock(dir, func() error {
		path, err := find(dir, id)
		if err != nil {
			return err
		}
		brief, err := loadAt(path, id)
		if err != nil {
			return err
		}
		if brief.IsClosed() {
			return &ClosedError{ID: id}
		}
		return change(path, brief)
	})
}

// nextID is the highest id open or archived plus one.
func nextID(dir string) (int, error) {
	highest := 0
	for _, path := range []string{state.BriefsPath(dir), state.ClosedBriefsPath(dir)} {
		taken, err := ids(path)
		if err != nil {
			return 0, err
		}
		for _, id := range taken {
			highest = max(highest, id)
		}
	}
	return highest + 1, nil
}

func next(brief Brief) int {
	if len(brief.Log) == 0 {
		return 1
	}
	return brief.Log[len(brief.Log)-1].N + 1
}

func publish(path string, entry Entry) error {
	raw, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}
	err = state.PublishExclusive(filepath.Join(path, logDir, FormatID(entry.N)+".json"), append(raw, '\n'))
	if errors.Is(err, state.ErrNameTaken) {
		return fmt.Errorf("entry %d of %s exists already", entry.N, path)
	}
	return err
}

func writeRecord(path string, record Record) error {
	raw, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return state.WriteAtomic(filepath.Join(path, recordFile), append(raw, '\n'))
}
