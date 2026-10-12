package legacy

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"

	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// wrapper is what a 1.x session record says of the wrapper that serves it.
type wrapper struct {
	pid       int
	start     uint64
	namespace string
}

// maxRecord bounds what is read of one file of the 1.x tree: a session record
// or a letter is a few kilobytes, and a file far past that is no record.
const maxRecord = 1 << 20

// readWrapper reads a 1.x session record's wrapper. A record that cannot be
// read or parsed, or that names a zero pid or start or no pid namespace, is
// an error: it proves no session gone.
//
// legacy(rewake <2026-10-12): a 1.x session record is read for its wrapper's pid, start time and pid namespace only, to see whether a 1.x session still runs; remove when no session started by an earlier build is registered
func readWrapper(path string) (wrapper, error) {
	data, err := readFile(path)
	if err != nil {
		return wrapper{}, err
	}
	var record struct {
		ServicePid   int    `json:"servicePid"`
		ServiceStart uint64 `json:"serviceStart"`
		PidNamespace string `json:"pidNamespace"`
	}
	if err := json.Unmarshal(data, &record); err != nil {
		return wrapper{}, fmt.Errorf("it is no session record: %w", err)
	}
	if record.ServicePid <= 0 || record.ServiceStart == 0 || record.PidNamespace == "" {
		return wrapper{}, errors.New("it names no wrapper: its pid, its start time or its pid namespace is missing")
	}
	return wrapper{pid: record.ServicePid, start: record.ServiceStart, namespace: record.PidNamespace}, nil
}

// readFile reads a file of the 1.x tree without following a link out of it.
func readFile(path string) ([]byte, error) {
	if err := state.AskRead(path); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, maxRecord+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxRecord {
		return nil, fmt.Errorf("it is larger than %d bytes", maxRecord)
	}
	return data, nil
}

// liveness is what /proc says of a wrapper.
type liveness int

const (
	gone liveness = iota
	running
	unknown
)

// alive judges a wrapper by /proc, in the pid namespace this launch sees (D1):
// no /proc/<pid> is proof it is gone, another start time is another process,
// a zombie is not running, and a read, a permission or a parse that fails is
// unknown — never taken for death, which is why proc.Alive is not used. A
// record of another namespace cannot be judged here at all.
func alive(reader proc.Reader, here string, w wrapper) (liveness, error) {
	if here == "" {
		return unknown, errors.New("this launch cannot read its own pid namespace, so whether its wrapper runs cannot be checked")
	}
	if w.namespace != here {
		return unknown, fmt.Errorf("it was written in pid namespace %s and this launch sees %s, so whether its wrapper runs cannot be checked", w.namespace, here)
	}
	switch reader.ObserveIdentity(w.pid, w.start) {
	case proc.IdentityAlive:
		return running, nil
	case proc.IdentityEnded:
		return gone, nil
	}
	return unknown, fmt.Errorf("/proc/%d/stat cannot be read", w.pid)
}
