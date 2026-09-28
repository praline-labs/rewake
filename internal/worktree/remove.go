package worktree

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Remove takes a checkout away with the public `git worktree remove`, and its
// record after it. force removes one with changes as well. A checkout whose
// directory is gone is removed the same way, which on a missing directory
// touches only its own entry — never `git worktree prune`, which would take
// every other missing checkout of the repository along; one the repository
// has forgotten leaves only the record to remove. A directory whose repository
// is gone has no git left to remove it, and only force deletes it. It holds
// the repository's lock, as Create does: git worktree remove reads the
// entries of the others as well. A root that is gone takes no lock, and is not
// made again for one: no checkout is made in it without Create making it
// first, and a root the person removed should stay removed.
func Remove(record Record, force bool) error {
	directory := filepath.Dir(record.Path)
	if _, err := os.Stat(filepath.Dir(directory)); errors.Is(err, fs.ErrNotExist) {
		return remove(record, force)
	}
	return withRepositoryLock(directory, func() error { return remove(record, force) })
}

// RemoveChecked is Remove with keep asked first under the same lock, of the
// record as it stands then; an error from keep refuses the removal and is
// returned as it is. The look and the removal have to be one step: a look
// taken before the lock could see a checkout a launch had claimed and not yet
// checked out — missing, forgotten, nothing to keep — and the removal after
// the wait would take the checkout that launch had just made.
func RemoveChecked(record Record, force bool, keep func(Record) error) error {
	return underLock(record, func(current Record) error {
		if keep != nil {
			if err := keep(current); err != nil {
				return err
			}
		}
		return remove(current, force)
	})
}

// LandedError is a finish that landed and could not remove the checkout after
// all, though the look before landing found nothing in the way.
type LandedError struct {
	Landing Landing
	Err     error
}

func (e *LandedError) Error() string { return e.Err.Error() }

func (e *LandedError) Unwrap() error { return e.Err }

// Finish lands a checkout once more and removes it, holding the repository's
// lock from keep's look to the removal, so what keep saw is what finish acts
// on: a launch claiming the name, or an rm, waits for it. keep refuses as in
// RemoveChecked, before anything lands.
func Finish(record Record, into string, keep func(Record) error) (Landing, error) {
	var landing Landing
	err := underLock(record, func(current Record) error {
		if keep != nil {
			if err := keep(current); err != nil {
				return err
			}
		}
		var err error
		if landing, err = Land(current, into); err != nil {
			return err
		}
		if err := remove(current, false); err != nil {
			return &LandedError{Landing: landing, Err: err}
		}
		return nil
	})
	return landing, err
}

// underLock runs fn holding the lock of the record's repository, with the
// record read again under it: a launch that made the checkout since the
// record was listed has written its owner into it.
func underLock(record Record, fn func(Record) error) error {
	current := func() error {
		now, err := read(recordPath(record))
		if errors.Is(err, fs.ErrNotExist) {
			return &StateError{Reason: fmt.Sprintf("the worktree %s was removed meanwhile", record.Ref())}
		}
		if err != nil {
			return err
		}
		return fn(now)
	}
	directory := filepath.Dir(record.Path)
	if _, err := os.Stat(filepath.Dir(directory)); errors.Is(err, fs.ErrNotExist) {
		return current()
	}
	return withRepositoryLock(directory, current)
}

func remove(record Record, force bool) error {
	present := isDir(record.Path)
	forgotten := repositoryGone(record)
	if !forgotten && !present {
		_, found, err := listed(record)
		if err != nil {
			return err
		}
		forgotten = !found
	}
	switch {
	case !forgotten:
		args := []string{"worktree", "remove"}
		if force {
			// Twice: once passes changes and submodules, and only the second
			// passes a checkout somebody locked with git worktree lock.
			args = append(args, "--force", "--force")
		}
		if err := gitDir(record.CommonDir, append(args, record.Path)...); err != nil {
			return fmt.Errorf("git worktree remove failed: %w", err)
		}
	case present:
		if !force {
			return fmt.Errorf("the repository of %s is gone, so git cannot tell what it holds; only a forced removal deletes it", record.Path)
		}
		if err := os.RemoveAll(record.Path); err != nil {
			return err
		}
	}
	if err := os.Remove(recordPath(record)); err != nil && !os.IsNotExist(err) {
		return err
	}
	// The repository's directory goes with its last checkout; one still in use
	// refuses to go, which is the answer wanted.
	_ = os.Remove(filepath.Dir(record.Path))
	return nil
}
