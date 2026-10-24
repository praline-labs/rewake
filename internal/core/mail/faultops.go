//go:build rewaketest

package mail

import (
	"os"
	"sync"
	"sync/atomic"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// The operation of this file lets the delivery server's tests, in another
// package, build the fault schedule that no exported operation, file or
// state.Fault reaches: a waiting copy whose removal fails. It exists only in
// a build with the rewaketest tag.

// failingRemovals holds, by path, the attempt counter of each waiting copy
// whose removal a test made fail.
var failingRemovals sync.Map

func init() {
	// Replaced once, before any test or server runs, so a test switching a
	// failure on while a server serves races with nothing.
	removeWaiting = func(path string) error {
		if attempts, ok := failingRemovals.Load(path); ok {
			attempts.(*atomic.Int64).Add(1)
			return os.ErrPermission
		}
		return state.Discard(path)
	}
}

// FailWaitingRemoval makes the removal of exactly the waiting copy at path fail
// with os.ErrPermission until restore, whether settling or a withdrawal removes
// it. attempts says how many removals it refused, so a test can tell the step
// was reached. Every other path is removed as in any build.
func FailWaitingRemoval(path string) (attempts func() int64, restore func()) {
	counter := &atomic.Int64{}
	failingRemovals.Store(path, counter)
	return counter.Load, func() { failingRemovals.CompareAndDelete(path, counter) }
}
