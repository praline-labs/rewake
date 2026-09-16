package cli

// Exit codes are a contract: an agent branches on the code instead of reading
// the text. They are printed in the guide for that reason.
const (
	// ExitOK means the command did what it says.
	ExitOK = 0
	// ExitFailed means the target refused or could not be reached.
	ExitFailed = 1
	// ExitUsage means the call itself was wrong.
	ExitUsage = 2
	// ExitPending means a message was accepted but is not delivered yet.
	ExitPending = 3
)

// UsageError is a wrong call: unknown command or flag, missing argument, no such
// session. It carries the command so the refusal can print its syntax.
type UsageError struct {
	Command *Command
	Message string
}

func (e *UsageError) Error() string { return e.Message }

// PendingError is an accepted message that has not been delivered yet. It is not
// a failure: the message is durable and lands on its own.
type PendingError struct {
	Message string
}

func (e *PendingError) Error() string { return e.Message }

// FailedError is a target that refused or could not be reached.
type FailedError struct {
	Message string
}

func (e *FailedError) Error() string { return e.Message }

// failf builds a FailedError.
func failf(format string, args ...any) error {
	return &FailedError{Message: sprintf(format, args...)}
}
