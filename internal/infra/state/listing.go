package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
)

// betweenOpens runs once base is open and before name is: a test swaps base
// there.
var betweenOpens = func(string) {}

// ReadDirBelow lists the directory name directly inside base, opening base
// and then name relative to it, neither through a link. A strict read takes
// a missing directory for an absent one, so what it lists must be missing
// for real: a dangling link at either level answers ENOENT through the path
// and would read as nothing there, and a link to a directory elsewhere as
// whatever lies there. Either is refused as no directory, while a level that
// is missing for real is os.ErrNotExist. Opening name relative to base keeps
// base from being swapped for a link between the two opens.
func ReadDirBelow(base, name string) ([]os.DirEntry, error) {
	const flags = syscall.O_RDONLY | syscall.O_CLOEXEC | syscall.O_DIRECTORY | syscall.O_NOFOLLOW
	path := filepath.Join(base, name)
	parent, err := syscall.Open(base, flags, 0)
	if err != nil {
		return nil, dirOpenError(base, err)
	}
	betweenOpens(base)
	fd, err := syscall.Openat(parent, name, flags, 0)
	_ = syscall.Close(parent)
	if err != nil {
		return nil, dirOpenError(path, err)
	}
	file := os.NewFile(uintptr(fd), path)
	defer func() { _ = file.Close() }()
	entries, err := file.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(entries, func(a, b os.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	return entries, nil
}

func dirOpenError(path string, err error) error {
	if errors.Is(err, syscall.ENOTDIR) || errors.Is(err, syscall.ELOOP) {
		return &os.PathError{Op: "open", Path: path, Err: fmt.Errorf("not a directory of its own: %w", err)}
	}
	return &os.PathError{Op: "open", Path: path, Err: err}
}
