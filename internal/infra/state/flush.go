package state

// FlushFault is asked before every flush this package makes — a temporary
// file's contents before it is linked or renamed into place, a directory's
// entries after — with the path flushed. A test makes a flush fail with it,
// and learns what a link that is visible and not durable leads to: the
// publication answers an error and the name stays taken. It is apart from Fault, whose steps a fault plan counts: a
// flush is part of the step it ends, not a step of its own. Nil in every
// build but a test's.
var FlushFault func(path string) error

func flushFault(path string) error {
	if FlushFault == nil {
		return nil
	}
	return FlushFault(path)
}
