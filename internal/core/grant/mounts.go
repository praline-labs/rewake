package grant

import (
	"os"
	"strconv"
	"strings"
)

// mountPoints reads the mount points from a mountinfo file: the fifth field
// of each line, with the octal escapes the kernel writes for a space, a tab, a
// newline and a backslash undone. An unreadable file gives none, and the
// fixed /mnt and /media still stand.
func mountPoints(path string) []string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var mounts []string
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		mounts = append(mounts, unescapeMount(fields[4]))
	}
	return mounts
}

func unescapeMount(field string) string {
	var out strings.Builder
	for i := 0; i < len(field); i++ {
		if field[i] == '\\' && i+3 < len(field) {
			if code, err := strconv.ParseUint(field[i+1:i+4], 8, 8); err == nil {
				out.WriteByte(byte(code))
				i += 3
				continue
			}
		}
		out.WriteByte(field[i])
	}
	return out.String()
}
