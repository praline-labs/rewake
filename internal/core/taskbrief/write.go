package taskbrief

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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
// It goes in the project's folder when the setting names one, in dir when
// that is given (which then wins), else in the room. A brief outside the room
// is entered in the index, the project's folder's included, so a move of its
// folder reads moved and its id stays taken. The skills its text names are
// found through catalog and copied into it as edition 1's snapshot, all
// checked before anything is written.
func New(home Home, author Author, text, dir string, catalog Catalog) (Brief, error) {
	if err := CheckText(text); err != nil {
		return Brief{}, err
	}
	plan, listed, err := planSkills(text, catalog)
	if err != nil {
		return Brief{}, err
	}
	var made Brief
	err = state.WithBriefLock(home.Room, func() error {
		id, err := nextID(home)
		if err != nil {
			return err
		}
		parent := dir
		if parent == "" {
			parent = home.Folder
		}
		if parent == "" {
			made, err = build(home, author, text, id, state.BriefsPath(home.Room), openPath(home.Room, id), false, plan, listed)
			return err
		}
		return state.WithOutside(home.Room, parent, func() error {
			final := folderPath(parent, id)
			if _, err := os.Lstat(final); err == nil {
				return fmt.Errorf("%s exists already: a brief of another room, or a folder of the project's own; name another directory with --dir", final)
			}
			if err := state.EnsureOutsideDir(parent); err != nil {
				return err
			}
			if made, err = build(home, author, text, id, parent, final, true, plan, listed); err != nil {
				return err
			}
			idx, err := readIndex(home.Room)
			if err != nil {
				return err
			}
			idx.Briefs[FormatID(id)] = indexed{Path: final}
			return writeIndex(home.Room, idx)
		})
	})
	return made, err
}

// build writes brief id into a temporary directory of parent and renames it
// to final. outside says final lies in a folder of the project, which
// records the room in the brief.
func build(home Home, author Author, text string, id int, parent, final string, outside bool, plan []planned, listed []byte) (Brief, error) {
	mkdir, room := state.EnsureSubdir, ""
	if outside {
		mkdir, room = state.EnsureOutsideDir, home.roomName()
	}
	now := time.Now().UTC()
	tmp := filepath.Join(parent, tmpPrefix+rand.Text())
	for _, sub := range []string{logDir, textDir} {
		if err := mkdir(filepath.Join(tmp, sub)); err != nil {
			return Brief{}, err
		}
	}
	if len(plan) > 0 {
		if err := mkdir(filepath.Join(tmp, skillsDir)); err != nil {
			return Brief{}, err
		}
		if err := writeSnapshot(plan, listed, snapshotPath(tmp, 1), outside); err != nil {
			return Brief{}, err
		}
	}
	if err := state.WriteAtomic(textPath(tmp, 1), []byte(text)); err != nil {
		return Brief{}, err
	}
	edit := Entry{N: 1, At: now, From: author.Name, Epoch: author.Epoch, Kind: KindEdit, Edition: 1}
	if err := publish(tmp, edit); err != nil {
		return Brief{}, err
	}
	if err := writeRecord(tmp, Record{ID: id, Created: now, Bound: []Binding{}, Room: room}); err != nil {
		return Brief{}, err
	}
	if err := state.Rename(tmp, final); err != nil {
		return Brief{}, err
	}
	if err := state.SyncDir(parent); err != nil {
		return Brief{}, err
	}
	return loadAt(final, id)
}

// Edit replaces the text of an open brief, refused unless revision is still
// the brief's: neither a tick nor an edit made since main read it is written
// over. The skills the text names are copied again as the new edition's
// snapshot, then the text is written, and its entry last, so the entry is
// the commit point of both: an edit cut short leaves the previous edition
// and its snapshot whole. The older snapshots go only after it.
func Edit(home Home, id int, author Author, revision int, text string, catalog Catalog) (Brief, error) {
	if err := CheckText(text); err != nil {
		return Brief{}, err
	}
	plan, listed, err := planSkills(text, catalog)
	if err != nil {
		return Brief{}, err
	}
	var edited Brief
	err = changeOpen(home, id, func(path string, brief Brief) error {
		if revision != brief.Revision {
			return &StaleRevisionError{ID: id, Given: revision, Current: brief.Revision}
		}
		edition := brief.Edition + 1
		outside := path != openPath(home.Room, id)
		if err := stageSnapshot(path, edition, plan, listed, outside); err != nil {
			return err
		}
		if err := state.WriteAtomic(textPath(path, edition), []byte(text)); err != nil {
			return err
		}
		entry := Entry{N: next(brief), At: time.Now().UTC(), From: author.Name, Epoch: author.Epoch, Kind: KindEdit, Edition: edition}
		if err := publish(path, entry); err != nil {
			return err
		}
		var err error
		if edited, err = loadAt(path, id); err != nil {
			return err
		}
		// Committed: an older snapshot nobody selects now; one this fails to
		// remove goes at the next edit or the close.
		_ = dropSnapshots(filepath.Join(path, skillsDir), func(n int) bool { return n < edition })
		return nil
	})
	return edited, err
}

// Tick sets item n of edition done or not, for a bound run: refused once
// main has edited since that edition, so a number read in an older text
// never ticks another item. It answers the item as it now stands.
func Tick(home Home, id int, author Author, edition, n int, done bool) (Item, error) {
	var ticked Item
	err := changeOpen(home, id, func(path string, brief Brief) error {
		binding, bound := brief.BindingOf(author)
		if !bound {
			return &NotBoundError{ID: id, Name: author.Name}
		}
		if binding.Kind.Of() == Review {
			return &ReviewOnlyError{ID: id, Name: author.Name}
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
		entry := Entry{N: next(brief), At: time.Now().UTC(), From: author.Name, Epoch: author.Epoch, Kind: kind, Edition: edition, Item: n, ItemText: echo(ticked.Text)}
		return publish(path, entry)
	})
	return ticked, err
}

// Comment adds a bound run's comment and answers its entry and its number
// among the brief's comments, 1 for the first: the number a worker names, apart
// from the log's, which edits and ticks share.
func Comment(home Home, id int, author Author, text string) (Entry, int, error) {
	if err := CheckComment(text); err != nil {
		return Entry{}, 0, err
	}
	var entry Entry
	number := 0
	err := changeOpen(home, id, func(path string, brief Brief) error {
		binding, bound := brief.BindingOf(author)
		if !bound {
			return &NotBoundError{ID: id, Name: author.Name}
		}
		number = len(brief.Comments()) + 1
		entry = Entry{N: next(brief), At: time.Now().UTC(), From: author.Name, Epoch: author.Epoch, Kind: KindComment, Edition: brief.Edition, Text: text, Capacity: binding.Kind.Of()}
		return publish(path, entry)
	})
	return entry, number, err
}

// Bind gives the brief to a run with the right kind names, or keeps it
// given; given before with the other kind, the run's binding is replaced. An
// earlier run of the same name stays listed as it was.
func Bind(home Home, id int, run Author, kind BindingKind) error {
	_, err := bind(home, id, Binding{Name: run.Name, Epoch: run.Epoch, Kind: kind.Of()})
	return err
}

// BindReview gives the brief to a run to review in a checkout of its own at
// commit, and answers the checkout the run's binding named before, if it was
// another, for the caller to remove.
func BindReview(home Home, id int, run Author, checkout, commit string) (string, error) {
	return bind(home, id, Binding{Name: run.Name, Epoch: run.Epoch, Kind: Review, Checkout: checkout, Commit: commit})
}

func bind(home Home, id int, given Binding) (previous string, err error) {
	err = changeOpen(home, id, func(path string, brief Brief) error {
		record := brief.Record
		record.Bound = slices.Clone(record.Bound)
		given.Since = time.Now().UTC()
		for i, binding := range record.Bound {
			if binding.Name != given.Name || binding.Epoch != given.Epoch {
				continue
			}
			if binding.Kind.Of() == given.Kind && binding.Checkout == given.Checkout {
				return nil
			}
			previous = binding.Checkout
			record.Bound[i] = given
			return writeRecord(path, record)
		}
		record.Bound = append(record.Bound, given)
		return writeRecord(path, record)
	})
	if err != nil || previous == given.Checkout {
		return "", err
	}
	return previous, nil
}

// changeOpen runs change under the lock on brief id as it is, refused when
// the brief is closed. A brief in a folder of the project is changed with
// that folder admitted to the seam.
func changeOpen(home Home, id int, change func(path string, brief Brief) error) error {
	return state.WithBriefLock(home.Room, func() error {
		path, outside, err := find(home, id)
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
		if outside {
			return state.WithOutside(home.Room, path, func() error { return change(path, brief) })
		}
		return change(path, brief)
	})
}

// nextID is past the highest id the room has: open, archived, in the
// project's folder whatever room made them, and in the index.
func nextID(home Home) (int, error) {
	highest := 0
	for _, path := range []string{state.BriefsPath(home.Room), state.ClosedBriefsPath(home.Room)} {
		taken, err := ids(path)
		if err != nil {
			return 0, err
		}
		for _, id := range taken {
			highest = max(highest, id)
		}
	}
	scanned, err := foldersIn(home.Folder)
	if err != nil {
		return 0, err
	}
	idx, err := readIndex(home.Room)
	if err != nil {
		return 0, err
	}
	for _, id := range append(scanned, idx.ids()...) {
		highest = max(highest, id)
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
	raw, err := encode(entry)
	if err != nil {
		return err
	}
	err = state.PublishExclusive(filepath.Join(path, logDir, FormatID(entry.N)+".json"), raw)
	if errors.Is(err, state.ErrNameTaken) {
		return fmt.Errorf("entry %d of %s exists already", entry.N, path)
	}
	return err
}

func writeRecord(path string, record Record) error {
	raw, err := encode(record)
	if err != nil {
		return err
	}
	return state.WriteAtomic(filepath.Join(path, recordFile), raw)
}

// encode answers a record as it is written, refused past maxRecordBytes
// before anything is written: no accepted write may leave a brief its
// readers refuse.
func encode(record any) ([]byte, error) {
	raw, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return nil, err
	}
	raw = append(raw, '\n')
	if len(raw) > maxRecordBytes {
		return nil, fmt.Errorf("%w: a record of %d bytes, past the bound of %d", ErrTooLarge, len(raw), maxRecordBytes)
	}
	return raw, nil
}
