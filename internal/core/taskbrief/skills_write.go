package taskbrief

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// The writes of a brief's skills, each under the brief lock: a snapshot is
// copied whole into a temporary folder and renamed to its edition, so a
// reader finds it whole or absent.

// writeSnapshot copies the planned skills and their skills.json into the
// folder into, which it makes. Files are written without an executable bit:
// a skill's script is read in the copy, not run from it. Each file and
// folder is flushed before the caller renames the snapshot into place.
func writeSnapshot(plan []planned, listed []byte, into string, outside bool) error {
	mkdir := state.EnsureSubdir
	if outside {
		mkdir = state.EnsureOutsideDir
	}
	if err := mkdir(into); err != nil {
		return err
	}
	for _, skill := range plan {
		if err := copyTree(skill.resolved, filepath.Join(into, skill.Name), mkdir); err != nil {
			return fmt.Errorf("%w: copying %s: %v", ErrSkill, skill.Name, err)
		}
	}
	if err := writeFlushed(filepath.Join(into, skillsFile), strings.NewReader(string(listed)), int64(len(listed))); err != nil {
		return err
	}
	return state.SyncDir(into)
}

// copyTree copies the folder from into to, as measure walked it, and keeps
// to the bound again: the skill may have grown since it was measured.
func copyTree(from, to string, mkdir func(string) error) error {
	entries, size := 0, int64(0)
	var walk func(from, to string) error
	walk = func(from, to string) error {
		if err := mkdir(to); err != nil {
			return err
		}
		listed, err := os.ReadDir(from)
		if err != nil {
			return err
		}
		for _, entry := range listed {
			source, target := filepath.Join(from, entry.Name()), filepath.Join(to, entry.Name())
			info, err := regularOrDir(source)
			if err != nil {
				return err
			}
			if entries++; entries > maxSkillEntries {
				return fmt.Errorf("more than %d entries, past the bound of a skill's copy", maxSkillEntries)
			}
			if info.IsDir() {
				if err := walk(source, target); err != nil {
					return err
				}
				continue
			}
			left := maxSkillBytes - size
			if info.Size() > left {
				return fmt.Errorf("more than %d bytes, past the bound of a skill's copy", maxSkillBytes)
			}
			file, err := os.Open(source)
			if err != nil {
				return err
			}
			err = writeFlushed(target, file, left)
			_ = file.Close()
			if err != nil {
				return err
			}
			size += info.Size()
		}
		return state.SyncDir(to)
	}
	return walk(from, to)
}

// writeFlushed writes a new file at path from r, at most limit bytes, and
// flushes it.
func writeFlushed(path string, r io.Reader, limit int64) error {
	file, err := state.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return err
	}
	written, err := io.Copy(file, io.LimitReader(r, limit+1))
	if err == nil && written > limit {
		err = fmt.Errorf("%s grew past the bound of a skill's copy while it was copied", path)
	}
	if err == nil {
		err = file.Sync()
	}
	if closed := file.Close(); err == nil {
		err = closed
	}
	return err
}

// stageSnapshot writes edition's snapshot of an open brief at path, before
// the edit that commits it: a snapshot of this edition or a later one left by
// an edit cut short, never committed and so read by nobody, goes first, and
// so does a staging folder left behind.
func stageSnapshot(path string, edition int, plan []planned, listed []byte, outside bool) error {
	dir := filepath.Join(path, skillsDir)
	if err := dropSnapshots(dir, func(n int) bool { return n >= edition }); err != nil {
		return err
	}
	if len(plan) == 0 {
		return nil
	}
	mkdir := state.EnsureSubdir
	if outside {
		mkdir = state.EnsureOutsideDir
	}
	if err := mkdir(dir); err != nil {
		return err
	}
	staged := filepath.Join(dir, tmpPrefix+rand.Text())
	if err := writeSnapshot(plan, listed, staged, outside); err != nil {
		_ = state.RemoveAll(staged)
		return err
	}
	if err := state.Rename(staged, snapshotPath(path, edition)); err != nil {
		return err
	}
	return state.SyncDir(dir)
}

// dropSnapshots removes the snapshots under dir whose edition drop selects,
// and every staging folder. A dir that is missing holds none.
func dropSnapshots(dir string, drop func(edition int) bool) error {
	entries, err := state.ReadPlainDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		edition, ok := ParseID(name)
		if strings.HasPrefix(name, tmpPrefix) || (ok && drop(edition)) {
			if err := state.RemoveAll(filepath.Join(dir, name)); err != nil {
				return err
			}
		}
	}
	return nil
}
