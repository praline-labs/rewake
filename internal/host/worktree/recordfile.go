package worktree

import (
	"errors"
	"os"
	"path/filepath"
)

// A record lives beside the worktrees, outside the state directory
// (docs/v2/stage3-state.md#what-is-a-writer): no room's lease covers it, so it
// is written here rather than through the state's write seam, with the same
// two forms the seam gives the state — a replacement by rename and a
// publication that fails when the name is taken.

// errRecordTaken is a record name another claim published first.
var errRecordTaken = errors.New("record name taken")

// replaceRecord replaces a record in one step: a reader sees the previous
// content or the new, never a half-written file.
func replaceRecord(path string, data []byte) error {
	name, err := writeRecordTemp(filepath.Dir(path), data)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(name) }()
	if err := os.Rename(name, path); err != nil {
		return err
	}
	return syncRecordDir(filepath.Dir(path))
}

// publishRecord writes a record only if its name is free: a link fails on a
// name that exists, so two claims of one name cannot both win.
func publishRecord(path string, data []byte) error {
	name, err := writeRecordTemp(filepath.Dir(path), data)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(name) }()
	if err := os.Link(name, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return errRecordTaken
		}
		return err
	}
	return syncRecordDir(filepath.Dir(path))
}

func writeRecordTemp(dir string, data []byte) (string, error) {
	temp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return "", err
	}
	name := temp.Name()
	_, err = temp.Write(data)
	if err == nil {
		err = temp.Sync()
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Chmod(name, 0o600)
	}
	if err != nil {
		_ = os.Remove(name)
		return "", err
	}
	return name, nil
}

func syncRecordDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = dir.Close() }()
	return dir.Sync()
}
