package boottime

import (
	"errors"
	"os"
	"strings"
)

// bootIDPath is where the kernel keeps the id of the current boot.
const bootIDPath = "/proc/sys/kernel/random/boot_id"

// ID reads the kernel's boot id, drawn at random at every boot. A process is
// named by its pid and its start in ticks since boot, and after a restart of
// the machine the same pair can name another process; with the boot id beside
// it the name never recurs (docs/protocol-cutover.md).
func ID() (string, error) {
	raw, err := os.ReadFile(bootIDPath)
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(string(raw))
	if !ValidID(id) {
		return "", errors.New("the kernel's boot id is not in the form it documents")
	}
	return id, nil
}

// ValidID says whether id has the form of a boot id: 36 characters of
// lower-case hex digits and dashes, which is also safe in a path.
func ValidID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i, r := range id {
		switch {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if r != '-' {
				return false
			}
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f':
		default:
			return false
		}
	}
	return true
}
