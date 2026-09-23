package harness

import "github.com/iiiokojiadbi/rewake/internal/registry"

// ThreadTracker is optional: only harnesses whose conversations can change
// within one wrapper run provide thread identity.
type ThreadTracker interface {
	Thread(registry.Session) (string, error)
}

// ThreadSource is a session-owned observer that also follows the conversation:
// the wrapper asks it which one a message is delivered into. It is the same
// question a ThreadTracker answers, asked of the process that hears the
// harness rather than of the registry, and an empty answer means unknown —
// never a refusal, because a conversation nobody has named yet must not stop
// the delivery.
type ThreadSource interface {
	Thread() (string, error)
}

// SessionThread returns a known conversation identity without requiring every
// harness to implement thread tracking.
func SessionThread(session registry.Session) (string, error) {
	for _, candidate := range All() {
		if candidate.ID() == session.Harness {
			if tracker, ok := candidate.(ThreadTracker); ok {
				return tracker.Thread(session)
			}
			break
		}
	}
	return "", nil
}
