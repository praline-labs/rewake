package grant

import (
	"os"
	"path/filepath"
	"strings"
)

// A Windows drive answers to a directory's short name as well as its long
// one: under WSL /mnt/c/PROGRA~1 is /mnt/c/Program Files, the same inode, and
// neither link resolution nor a comparison without case tells them apart. A
// protected directory named one way would let the other through, so every
// path on a drive is compared by its long names (docs/grants.md#the-hard-tier).

// onDrive says whether a path lies on a Windows drive, where short names
// resolve; a variable so that a test can stand a directory of its own in.
var onDrive = windowsDrive

// windowsDrive says whether a path lies under /mnt/<letter>, where WSL mounts
// the Windows drives.
func windowsDrive(path string) bool {
	path = filepath.Clean(path)
	return len(path) >= 6 && strings.HasPrefix(path, "/mnt/") && isLetter(path[5]) && (len(path) == 6 || path[6] == '/')
}

// longNames spells each short name in a path on a Windows drive by the long
// name of the same directory, as far as the path exists. An element it cannot
// match stays as it is, and check refuses the path for it.
func longNames(path string) string {
	if !onDrive(path) {
		return path
	}
	elements := strings.Split(filepath.Clean(path), string(filepath.Separator))
	current := string(filepath.Separator)
	for _, element := range elements[1:] {
		next := filepath.Join(current, element)
		if shortName(element) {
			if long, ok := longNameOf(current, next); ok {
				next = filepath.Join(current, long)
			}
		}
		current = next
	}
	return current
}

// longNameOf finds the entry of parent that is the same file as path under
// another name. A drive lists long names only.
func longNameOf(parent, path string) (string, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return "", false
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		return "", false
	}
	for _, entry := range entries {
		if strings.EqualFold(entry.Name(), filepath.Base(path)) || shortName(entry.Name()) {
			continue
		}
		if other, err := os.Stat(filepath.Join(parent, entry.Name())); err == nil && os.SameFile(info, other) {
			return entry.Name(), true
		}
	}
	return "", false
}

// shortName says whether a path element has the shape of a generated 8.3
// name: at most eight characters ending in a tilde and digits, and at most
// three after one dot.
func shortName(element string) bool {
	base, extension, _ := strings.Cut(element, ".")
	if len(base) > 8 || len(extension) > 3 || strings.Contains(extension, ".") {
		return false
	}
	tilde := strings.LastIndexByte(base, '~')
	if tilde < 0 || tilde == len(base)-1 {
		return false
	}
	for _, c := range base[tilde+1:] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// shortElement names the first short name left in a path on a drive: one
// longNames could not match, and so one the checks cannot see through.
func shortElement(path string) (string, bool) {
	if !onDrive(path) {
		return "", false
	}
	for _, element := range strings.Split(filepath.Clean(path), string(filepath.Separator)) {
		if shortName(element) {
			return element, true
		}
	}
	return "", false
}
