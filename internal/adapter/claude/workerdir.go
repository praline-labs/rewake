package claude

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// The service folder (the owner's decisions, October 19 and 23, 2026): rewake
// makes the profile's directory on a worker's launch, so the person only
// drops things in. It is the workers' Claude Code configuration directory:
// its CLAUDE.md and settings.json are what Claude Code reads as the user's
// own, beside the login, trust and transcripts it keeps there itself. Skills
// reach a worker with a brief (docs/briefs.md#skills), so no skills/ is made
// (the owner's decision, October 28, 2026); one the person made stays.
// Whatever is there already is the person's or Claude Code's and stays as it
// is, but for settings.json, which workerSettings writes and brings up to
// date, and the two keys of .claude.json prepareClaudeState adds.

// profileFiles are the files the folder starts with, by path within it.
var profileFiles = map[string]string{
	"CLAUDE.md": "",
}

// makeProfileDir makes what of the folder is missing, never replacing a file
// or a directory the person has.
func makeProfileDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("the worker profile's directory %s could not be made: %w", dir, err)
	}
	for name, text := range profileFiles {
		path := filepath.Join(dir, name)
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err == nil {
			_, err = file.WriteString(text)
			if closed := file.Close(); err == nil {
				err = closed
			}
		}
		if err != nil {
			return fmt.Errorf("the worker profile's %s could not be made: %w", path, err)
		}
	}
	return nil
}

// makeMemoryDir makes a template's memory directory and its parents private.
func makeMemoryDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("the worker's memory directory %s could not be made: %w", dir, err)
	}
	return nil
}

// replaceFile writes a file of the folder in one step: a launch reading it at
// the same time sees the old content or the new, never half of it.
func replaceFile(path string, data []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	_, err = temp.Write(data)
	if err == nil {
		err = temp.Sync()
	}
	if closed := temp.Close(); err == nil {
		err = closed
	}
	if err == nil {
		err = os.Rename(temp.Name(), path)
	}
	if err != nil {
		_ = os.Remove(temp.Name())
	}
	return err
}
