package state

import "os"

// Fault is the one test hook of the state directory's file operations
// (docs/mail-bridge-checks.md#testing-without-a-live-harness). Every write,
// removal and read that goes through this package asks it first, with the
// operation and the path: a test learns the order of a call's durable steps
// from it, and makes any one of them fail or end the process. Nil in every
// build but a test's.
var Fault func(op, path string) error

// The operations Fault is asked about.
const (
	OpRead    = "read"
	OpWrite   = "write"
	OpPublish = "publish"
	OpRemove  = "remove"
	OpRename  = "rename"
	// OpStep is a step of a call's path that is no file operation: the
	// server asking for a ticket, starting a child, writing an answer.
	OpStep = "step"
)

func fault(op, path string) error {
	if Fault == nil {
		return nil
	}
	return Fault(op, path)
}

// ReadFile reads a file of the state directory.
func ReadFile(path string) ([]byte, error) {
	if err := fault(OpRead, path); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

// Remove removes a file of the state directory.
func Remove(path string) error {
	if err := fault(OpRemove, path); err != nil {
		return err
	}
	return os.Remove(path)
}

// Rename moves a file of the state directory to another name in it.
func Rename(from, to string) error {
	if err := fault(OpRename, to); err != nil {
		return err
	}
	return os.Rename(from, to)
}

// Step names a step of a call's path that touches no file, so a fault plan
// can end the process there as it ends it before a write. It cannot fail.
func Step(name string) { _ = fault(OpStep, name) }
