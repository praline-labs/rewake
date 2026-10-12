package state

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
)

// View mode (docs/v2/stage3-state.md#what-is-a-writer): a view form answers
// from what is there, and every write primitive of this package refuses while
// the process is in it. The refusal alone would not hold the rule — a view
// that swallows an error would go on as if it had written — so the mode keeps
// what was attempted, and leaving it reports that whatever the caller did
// with the error.

// opLock names taking a lock in the evidence of a view's attempt: a lock file
// is created when missing, and holding one orders writers, so a view takes
// none.
const opLock = "lock"

// ErrViewWrite is the refusal of a write in view mode.
var ErrViewWrite = errors.New("a view does not write the state")

var view struct {
	sync.Mutex
	on       bool
	attempts []string
}

// EnterView puts the process in view mode until leave, which restores the
// mode it found and returns the writes attempted meanwhile as one error, or
// nil. Not reentrant: a view inside a view is a caller's mistake.
func EnterView() (leave func() error) {
	view.Lock()
	defer view.Unlock()
	view.on, view.attempts = true, nil
	return func() error {
		view.Lock()
		defer view.Unlock()
		attempts := view.attempts
		view.on, view.attempts = false, nil
		if len(attempts) == 0 {
			return nil
		}
		return fmt.Errorf("%w: it tried %s", ErrViewWrite, strings.Join(attempts, ", "))
	}
}

// viewWrite refuses a write in view mode and keeps it as evidence; outside
// the mode it lets every write through.
func viewWrite(op, path string) error {
	view.Lock()
	defer view.Unlock()
	if !view.on {
		return nil
	}
	view.attempts = append(view.attempts, op+" "+path)
	return &os.PathError{Op: op, Path: path, Err: ErrViewWrite}
}
