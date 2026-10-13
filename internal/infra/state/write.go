package state

import (
	"errors"
	"os"
	"path/filepath"
)

// The writes of a record: replaced in one step, or published under a name
// that must be free, each through a temporary file flushed before it is
// placed and a directory flushed after.

// WriteAtomic replaces a file in one step: a reader sees either the previous
// content or the new one, never a half-written file. The content is flushed
// before it is published and the directory after, so a crash cannot leave a
// name pointing at bytes that were never written.
func WriteAtomic(path string, data []byte) error {
	return noteReach(writeAtomic(path, data, nil), true)
}

func writeAtomic(path string, data []byte, points Points) error {
	if err := guard(OpWrite, path); err != nil {
		return err
	}
	return replace(path, data, points)
}

// replace is writeAtomic past the guard: the build record's write, which a
// launch makes under the exclusive lock before any lease authorizes the room.
func replace(path string, data []byte, points Points) error {
	if err := fault(OpWrite, path); err != nil {
		return err
	}
	name, err := writeTemp(filepath.Dir(path), data, points, path)
	if err != nil {
		return err
	}
	defer points.discard(name, path)

	if err := points.at(PointPlace, path); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	if err := points.at(PointDirSync, path); err != nil {
		return err
	}
	return syncDir(filepath.Dir(path))
}

// writeTemp writes data to a new file in dir, to be placed at path, and
// returns its name.
func writeTemp(dir string, data []byte, points Points, path string) (string, error) {
	if err := points.at(PointTemp, path); err != nil {
		return "", err
	}
	temp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return "", err
	}
	name := temp.Name()
	failed := func(err error) (string, error) {
		_ = temp.Close()
		points.discard(name, path)
		return "", err
	}

	if _, err := temp.Write(data); err != nil {
		return failed(err)
	}
	if err := points.at(PointTempSync, path); err != nil {
		return failed(err)
	}
	if err := flushFault(name); err != nil {
		return failed(err)
	}
	if err := temp.Sync(); err != nil {
		return failed(err)
	}
	if err := temp.Close(); err != nil {
		points.discard(name, path)
		return "", err
	}
	if err := os.Chmod(name, 0o600); err != nil {
		points.discard(name, path)
		return "", err
	}
	return name, nil
}

// SyncDir flushes a directory entry, so a rename or a link survives a crash.
func SyncDir(path string) error { return syncDir(path) }

// syncDir flushes a directory entry, so a rename or a link survives a crash.
func syncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = dir.Close() }()
	if err := flushFault(path); err != nil {
		return err
	}
	return dir.Sync()
}

// ErrNameTaken is returned when a name is already published.
var ErrNameTaken = errors.New("name taken")

// PublishExclusive writes a file only if the name is free. It writes a temporary
// file and links it into place: link fails when the target exists, so two
// processes claiming one name cannot both believe they won.
func PublishExclusive(path string, data []byte) error {
	return publishExclusive(path, data, nil)
}

func publishExclusive(path string, data []byte, points Points) error {
	if err := guard(OpPublish, path); err != nil {
		return err
	}
	if err := fault(OpPublish, path); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	name, err := writeTemp(dir, data, points, path)
	if err != nil {
		return err
	}
	defer points.discard(name, path)

	if err := points.at(PointPlace, path); err != nil {
		return err
	}
	if err := os.Link(name, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return ErrNameTaken
		}
		return err
	}
	if err := points.at(PointDirSync, path); err != nil {
		return err
	}
	return syncDir(dir)
}

// Points is told each point inside one write of a record before it is
// reached, with the record's path, so a test can stop or fail the write
// there rather than only before or after the whole of it: the temporary file
// written, its contents flushed, the file placed under its name — linked for
// a publication, renamed for a replacement — and the directory flushed. An
// error fails the point as the file system would, and the write returns it;
// at PointDiscard it keeps the temporary file, as a process stopped there
// leaves it. Nil, as every product caller passes, is told nothing.
type Points func(point, path string) error

// The points of one write, in their order.
const (
	PointTemp     = "temp"
	PointTempSync = "temp-sync"
	PointPlace    = "place"
	PointDirSync  = "dir-sync"
	// PointDiscard comes before the temporary file is removed, once the
	// write is over or has failed; it is no step of the write.
	PointDiscard = "discard"
)

func (p Points) at(point, path string) error {
	if p == nil {
		return nil
	}
	return p(point, path)
}

func (p Points) discard(name, path string) {
	if p.at(PointDiscard, path) == nil {
		_ = os.Remove(name)
	}
}

// PublishExclusiveWith is PublishExclusive told each of its points.
func PublishExclusiveWith(path string, data []byte, points Points) error {
	return publishExclusive(path, data, points)
}

// WriteAtomicWith is WriteAtomic told each of its points.
func WriteAtomicWith(path string, data []byte, points Points) error {
	return noteReach(writeAtomic(path, data, points), true)
}
