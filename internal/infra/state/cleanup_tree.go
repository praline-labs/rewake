package state

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

// The transitions of a 1.x tree (docs/v2/stage3-upgrade.md#the-trees-and-their-states):
// a tree's state is its name, a rename that never replaces is the only
// transition, and a transition counts once the flush of the base after it
// succeeded. Each is a durable step a fault plan counts, and its flush one
// more, which a plan may fail.

// renameNoReplace is renameat2's flag that fails on an existing target.
const renameNoReplace = 0x1

// ErrTreeExists means a tree's rename found its target taken: rooms/
// recreated meanwhile, or a name a clearer's id collided with.
var ErrTreeExists = errors.New("the name is taken")

// Candidate is where a tree set aside under id lives.
func (c *Cleanup) Candidate(id string) string { return filepath.Join(c.base, candidatePrefix+id) }

// Validated is where a validated tree of id lives.
func (c *Cleanup) Validated(id string) string { return filepath.Join(c.base, validatedPrefix+id) }

// Rooms is the 1.x tree, rooms/ in the base.
func (c *Cleanup) Rooms() string { return filepath.Join(c.base, legacyRooms) }

// SetAside renames rooms/ to the candidate of id.
func (c *Cleanup) SetAside(id string) error {
	return c.transition(c.Rooms(), c.Candidate(id), id)
}

// MarkValidated renames the candidate of id to its validated tree, keeping
// its id.
func (c *Cleanup) MarkValidated(id string) error {
	return c.transition(c.Candidate(id), c.Validated(id), id)
}

// Restore renames the candidate of id back to rooms/, never over a rooms/
// that appeared meanwhile: then ErrTreeExists, and the candidate stays.
func (c *Cleanup) Restore(id string) error {
	return c.transition(c.Candidate(id), c.Rooms(), id)
}

func (c *Cleanup) transition(from, to, id string) error {
	if !ValidTreeID(id) {
		return fmt.Errorf("%q is no tree id", id)
	}
	for _, path := range []string{from, to} {
		if err := c.check(OpRename, path); err != nil {
			return err
		}
	}
	if err := plainDir(from); err != nil {
		return err
	}
	if err := fault(OpRename, to); err != nil {
		return err
	}
	if err := renameExclusive(from, to); err != nil {
		if errors.Is(err, syscall.EEXIST) || errors.Is(err, syscall.ENOTEMPTY) {
			return fmt.Errorf("%w: renaming %s to %s", ErrTreeExists, from, to)
		}
		return fmt.Errorf("could not rename %s to %s: %w", from, to, err)
	}
	return c.flush()
}

// Settle flushes the base so that every transition already made in it is
// durable, by whichever clearer made it: a validated name another clearer
// left may be one whose flush failed, and seeing the name proves nothing
// about the disk. A clearing settles before it deletes a validated tree it
// did not just validate itself — the tree of id — and deletes nothing when it
// cannot.
func (c *Cleanup) Settle(id string) error {
	if !ValidTreeID(id) {
		return fmt.Errorf("%q is no tree id", id)
	}
	if err := c.check(OpSync, c.Validated(id)); err != nil {
		return err
	}
	return c.flush()
}

// flush syncs the base after a rename: until it succeeds the rename may not
// be durable, and nothing is deleted on it.
func (c *Cleanup) flush() error {
	if err := fault(OpSync, c.base); err != nil {
		return err
	}
	if err := syncDir(c.base); err != nil {
		return fmt.Errorf("could not flush %s: %w", c.base, err)
	}
	return nil
}

// renameExclusive renames with renameat2's RENAME_NOREPLACE: a target that
// exists fails the rename rather than being replaced, which a check followed
// by a plain rename cannot promise.
func renameExclusive(from, to string) error {
	fromPtr, err := syscall.BytePtrFromString(from)
	if err != nil {
		return err
	}
	toPtr, err := syscall.BytePtrFromString(to)
	if err != nil {
		return err
	}
	atCWD := ^uintptr(99) // AT_FDCWD, -100
	_, _, errno := syscall.Syscall6(sysRenameat2, atCWD, uintptr(unsafe.Pointer(fromPtr)), atCWD, uintptr(unsafe.Pointer(toPtr)), renameNoReplace, 0)
	if errno != 0 {
		return errno
	}
	return nil
}

// Delete removes the validated tree of id, room by room and then the tree,
// each removal a step a fault plan counts: a deletion stopped part way leaves
// part of a validated tree, which the next launch finishes. Nothing else is
// ever deleted in part. A symlink in it is removed, never followed.
func (c *Cleanup) Delete(id string) error {
	tree := c.Validated(id)
	if !ValidTreeID(id) {
		return fmt.Errorf("%q is no tree id", id)
	}
	if err := c.check(OpRemove, tree); err != nil {
		return err
	}
	if err := plainDir(tree); err != nil {
		return err
	}
	entries, err := ReadPlainDir(tree)
	if err != nil {
		return fmt.Errorf("could not read %s: %w", tree, err)
	}
	for _, entry := range entries {
		path := filepath.Join(tree, entry.Name())
		if err := fault(OpRemove, path); err != nil {
			return err
		}
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	if err := fault(OpRemove, tree); err != nil {
		return err
	}
	return os.RemoveAll(tree)
}

// summary is where the counts of the tree of id are recorded.
func (c *Cleanup) summary(id string) string {
	return filepath.Join(c.base, rootName, summaryPrefix+id+summarySuffix)
}

// WriteSummary records the counts of the tree of id under the root, by
// rename and sync, before the tree is deleted: a launch finishing a partial
// deletion reports them rather than what is left.
func (c *Cleanup) WriteSummary(id string, data []byte) error {
	if !ValidTreeID(id) {
		return fmt.Errorf("%q is no tree id", id)
	}
	path := c.summary(id)
	if err := c.check(OpWrite, path); err != nil {
		return err
	}
	return replace(path, data)
}

// ReadSummary reads the counts recorded for the tree of id; a missing one is
// os.ErrNotExist. Like every file the clearing opens, the record must be a
// regular file, refused before the open and checked on the descriptor: the
// open follows no link and does not wait, so a FIFO in its place cannot hold
// the clear lock until a writer comes.
func (c *Cleanup) ReadSummary(id string) ([]byte, error) {
	if !ValidTreeID(id) {
		return nil, fmt.Errorf("%q is no tree id", id)
	}
	path := c.summary(id)
	if info, err := os.Lstat(path); err != nil {
		return nil, noteReach(err, false)
	} else if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	if err := fault(OpRead, path); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, noteReach(err, false)
	}
	defer func() { _ = file.Close() }()
	if info, err := file.Stat(); err != nil {
		return nil, err
	} else if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	data, err := io.ReadAll(file)
	return data, noteReach(err, false)
}

// DiscardSummary removes the counts of a tree whose deletion is complete.
func (c *Cleanup) DiscardSummary(id string) error {
	if !ValidTreeID(id) {
		return fmt.Errorf("%q is no tree id", id)
	}
	path := c.summary(id)
	if err := c.check(OpRemove, path); err != nil {
		return err
	}
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// Trees is what a clearing finds in its base: whether rooms/ is there, and
// the ids of the trees set aside, each kind in order.
type Trees struct {
	Rooms      bool
	Candidates []string
	Validated  []string
}

// Empty says whether the base holds no tree of 1.x: the fast path.
func (t Trees) Empty() bool { return !t.Rooms && len(t.Candidates) == 0 && len(t.Validated) == 0 }

// Trees reads the base for the trees of 1.x. A name of theirs that is not a
// directory of its own — a link above all — is an error, never followed; a
// dotted name with no clearer's id is no tree and is left alone.
func (c *Cleanup) Trees() (Trees, error) {
	var trees Trees
	entries, err := os.ReadDir(c.base)
	if err != nil {
		return trees, fmt.Errorf("could not read %s: %w", c.base, err)
	}
	for _, entry := range entries {
		name := entry.Name()
		var list *[]string
		id := ""
		switch {
		case name == legacyRooms:
		case strings.HasPrefix(name, candidatePrefix) && ValidTreeID(strings.TrimPrefix(name, candidatePrefix)):
			list, id = &trees.Candidates, strings.TrimPrefix(name, candidatePrefix)
		case strings.HasPrefix(name, validatedPrefix) && ValidTreeID(strings.TrimPrefix(name, validatedPrefix)):
			list, id = &trees.Validated, strings.TrimPrefix(name, validatedPrefix)
		default:
			continue
		}
		if err := plainDir(filepath.Join(c.base, name)); err != nil {
			return trees, err
		}
		if list == nil {
			trees.Rooms = true
		} else {
			*list = append(*list, id)
		}
	}
	return trees, nil
}
