package state

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

// PlaceExecutable puts an executable at path, a file of a room, by rename
// (docs/rules/state.md#the-rooms-own-copy): a hard link to from where the
// filesystem and its owner allow one, else a copy of image with mode 0700.
// verify reads the file before it takes the name; one it refuses is dropped —
// a link to an install path an upgrade has already replaced falls back to the
// copy, and a copy it refuses fails the call. A link keeps the mode of the
// file it names, which is never changed here: that file is the person's
// install. Only path's end is guarded; from and image are read.
func PlaceExecutable(path, from, image string, verify func(string) error) error {
	if err := guard(OpWrite, path); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	temp, err := linkTemp(dir, from)
	if err == nil {
		if verify(temp) == nil {
			return placeTemp(temp, path)
		}
		_ = os.Remove(temp)
	}
	temp, err = copyTemp(dir, image)
	if err != nil {
		return err
	}
	if err := verify(temp); err != nil {
		_ = os.Remove(temp)
		return err
	}
	return placeTemp(temp, path)
}

// linkTemp gives from a new temporary name in dir.
func linkTemp(dir, from string) (string, error) {
	if from == "" {
		return "", errors.New("nothing to link")
	}
	file, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return "", err
	}
	name := file.Name()
	_ = file.Close()
	// The name is only reserved; link wants it free.
	if err := os.Remove(name); err != nil {
		return "", err
	}
	if err := os.Link(from, name); err != nil {
		return "", err
	}
	return name, nil
}

// copyTemp copies image into a new temporary file in dir, executable by its
// owner only, flushed.
func copyTemp(dir, image string) (string, error) {
	source, err := os.Open(image)
	if err != nil {
		return "", err
	}
	defer func() { _ = source.Close() }()
	temp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return "", err
	}
	name := temp.Name()
	failed := func(err error) (string, error) {
		_ = temp.Close()
		_ = os.Remove(name)
		return "", err
	}
	if _, err := io.Copy(temp, source); err != nil {
		return failed(err)
	}
	if err := temp.Chmod(0o700); err != nil {
		return failed(err)
	}
	if err := temp.Sync(); err != nil {
		return failed(err)
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	return name, nil
}

// placeTemp renames temp to path and flushes the directory entry.
func placeTemp(temp, path string) error {
	if err := os.Rename(temp, path); err != nil {
		_ = os.Remove(temp)
		return err
	}
	return syncDir(filepath.Dir(path))
}
