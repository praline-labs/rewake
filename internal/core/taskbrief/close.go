package taskbrief

import (
	"errors"
	"path/filepath"
	"time"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// Close records the brief closed. A brief of the room is moved to the
// archive, which ends every binding; one in a folder of the project is
// marked in place, the folder being the project's. A close already done
// answers already; one cut short between its two steps is finished. A brief
// whose folder has moved is marked closed in the index and stays there, so
// its id is not taken again. The copies of its skills go first, a close cut
// short taking the rest at the next.
func Close(home Home, id int) (already bool, err error) {
	dir := home.Room
	err = state.WithBriefLock(dir, func() error {
		path, outside, err := find(home, id)
		var moved *MovedError
		if errors.As(err, &moved) {
			already, err = markMoved(dir, id)
			return err
		}
		if err != nil {
			return err
		}
		if outside {
			return state.WithOutside(dir, path, func() error {
				brief, err := loadAt(path, id)
				if err != nil {
					return err
				}
				if err := state.RemoveAll(filepath.Join(path, skillsDir)); err != nil {
					return err
				}
				if brief.IsClosed() {
					already = true
					return nil
				}
				now := time.Now().UTC()
				record := brief.Record
				record.Closed = &now
				return writeRecord(path, record)
			})
		}
		brief, err := loadAt(path, id)
		if err != nil {
			return err
		}
		if path != openPath(dir, id) {
			already = true
			return nil
		}
		if err := state.RemoveAll(filepath.Join(path, skillsDir)); err != nil {
			return err
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

// markMoved sets the index entry of brief id closed, answering whether it
// was.
func markMoved(room string, id int) (already bool, err error) {
	idx, err := readIndex(room)
	if err != nil {
		return false, err
	}
	entry := idx.Briefs[FormatID(id)]
	if entry.Closed {
		return true, nil
	}
	entry.Closed = true
	idx.Briefs[FormatID(id)] = entry
	return false, writeIndex(room, idx)
}
