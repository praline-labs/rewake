package adapter

// RunProcess names the harness process of one run: what a service that
// checks its peer needs of the session (HarnessWatcher), and nothing more of
// it.
type RunProcess struct {
	PID   int
	Start uint64
}

// ThreadSource is a run that follows its conversation itself: the host asks it
// which one a message is delivered into. An empty answer means unknown — never
// a refusal, because a conversation nobody has named yet must not stop the
// delivery.
type ThreadSource interface {
	Thread() (string, error)
}

// Resumer is a harness whose conversation a later launch continues by its id:
// the arguments after the launch command that resume it, for a terminal
// manager that restarts the session after its own restart (docs/launch.md#herdr).
type Resumer interface {
	ResumeArgs(conversation string) []string
}
