package claude

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// The service folder (the owner's decision, October 19, 2026): rewake makes
// the profile's directory on a worker's launch, so the person only drops
// things in. The directory is a plugin of its own, so a skill placed or
// linked into skills/ loads as worker:<skill> with no plugin layout to learn.
// Whatever is there already is the person's and stays as it is, but for
// settings.json, which workerSettings writes and brings up to date.

// profilePlugin names the plugin the directory is.
const profilePlugin = "worker"

// profileFiles are the files the folder starts with, by path within it.
var profileFiles = map[string]string{
	"CLAUDE.md":                  "",
	".claude-plugin/plugin.json": `{"name": "` + profilePlugin + `", "description": "The skills of rewake's Claude Code workers: each folder in skills/ is one.", "version": "1.0.0"}` + "\n",
}

// makeProfileDir makes what of the folder is missing, never replacing a file
// or a directory the person has.
func makeProfileDir(dir string) error {
	for _, sub := range []string{"skills", ".claude-plugin"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o700); err != nil {
			return fmt.Errorf("the worker profile's directory %s could not be made: %w", dir, err)
		}
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
