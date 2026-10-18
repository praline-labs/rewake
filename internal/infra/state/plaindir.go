package state

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"syscall"
)

// ReadPlainDir lists a directory of its own in name order. The open takes
// nothing but a directory and follows no link, so a FIFO or a link put where
// a directory was checked is refused by the open itself — never waited on,
// never followed.
func ReadPlainDir(path string) ([]os.DirEntry, error) {
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_CLOEXEC|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
	if errors.Is(err, syscall.ENOTDIR) || errors.Is(err, syscall.ELOOP) {
		return nil, fmt.Errorf("%s is not a directory", path)
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	entries, err := file.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(entries, func(a, b os.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	return entries, nil
}
