package claude

import "github.com/iiiokojiadbi/rewake/internal/inbox"

// interruptMarks is where the lane learns that a main interrupted the
// session's last turn (telemetry.Collector), to say so in the next notice
// (docs/remote-control.md).
type interruptMarks interface {
	Interrupter() (string, uint64)
	Told(mark uint64)
}

// interruption is who interrupted the session's last turn, "" when nobody or
// a person did, and told, which the notice's result passes through: a notice
// that carried the line and was delivered uses the mark up.
func (l *lane) interruption() (string, func(inbox.Result) inbox.Result) {
	if l.marks == nil {
		return "", func(result inbox.Result) inbox.Result { return result }
	}
	interrupter, mark := l.marks.Interrupter()
	return interrupter, func(result inbox.Result) inbox.Result {
		// Once written, the line has reached the model or will: it is not
		// said again.
		if interrupter != "" && result.State == inbox.Delivered {
			l.marks.Told(mark)
		}
		return result
	}
}
