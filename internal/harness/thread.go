package harness

import "github.com/iiiokojiadbi/rewake/internal/registry"

// ThreadTracker is optional: only harnesses whose conversations can change
// within one wrapper run provide thread identity.
type ThreadTracker interface {
	Thread(registry.Session) (string, error)
}

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
