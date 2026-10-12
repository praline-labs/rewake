package proc

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// maxEnvironBytes bounds the initial environment a read takes: the kernel
// caps a process's arguments and environment together well below this, so a
// longer one is not a process's environment.
const maxEnvironBytes = 8 << 20

// InitialEnv says whether the environment a process was started with names
// key. It is the environment as the process was executed, which nothing the
// process does later changes: unsetting a variable in a shell leaves it here.
// The kernel lets only the process's own user read it.
func (r Reader) InitialEnv(pid int, key string) (bool, error) {
	file, err := os.Open(filepath.Join(r.Root, strconv.Itoa(pid), "environ"))
	if err != nil {
		return false, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxEnvironBytes+1))
	if err != nil {
		return false, err
	}
	if len(raw) > maxEnvironBytes {
		return false, fmt.Errorf("the environment of pid %d is longer than %d bytes", pid, maxEnvironBytes)
	}
	prefix := []byte(key + "=")
	for entry := range bytes.SplitSeq(raw, []byte{0}) {
		if bytes.HasPrefix(entry, prefix) {
			return true, nil
		}
	}
	return false, nil
}

// Owner returns the real and effective user of a process, from its status.
func (r Reader) Owner(pid int) (real, effective int, err error) {
	raw, err := os.ReadFile(filepath.Join(r.Root, strconv.Itoa(pid), "status"))
	if err != nil {
		return 0, 0, err
	}
	for line := range strings.SplitSeq(string(raw), "\n") {
		rest, found := strings.CutPrefix(line, "Uid:")
		if !found {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) < 2 {
			break
		}
		real, realErr := strconv.Atoi(fields[0])
		effective, effectiveErr := strconv.Atoi(fields[1])
		if realErr != nil || effectiveErr != nil {
			break
		}
		return real, effective, nil
	}
	return 0, 0, errors.New("the status of pid " + strconv.Itoa(pid) + " names no user")
}
