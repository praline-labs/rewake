package inbox

import "strings"

// MinReference is the shortest id reference taken: shorter ones would match
// most of a mailbox by accident.
const MinReference = 4

// SentMatching finds what this run sent by a reference to its id: the whole
// id, or a prefix of it or of its random tail — the part after the dash, which
// is what tells two messages of the same moment apart, as git's short hashes
// do. Only what a sender wrote itself is a candidate: tasks, questions, notes
// and the tombstones of those, never a report or a notice rewake wrote on this
// run's behalf, and only this run's: mail from a plain shell or an earlier run
// is out of reach. An exact id wins over prefixes of it. An error says a
// letter could not be read, which may be the one referred to.
func SentMatching(dir, name, epoch, reference string) ([]Message, error) {
	if epoch == "" || len(reference) < MinReference {
		return nil, nil
	}
	recipients, err := mailboxes(dir)
	if err != nil {
		return nil, err
	}
	var matches []Message
	for _, recipient := range recipients {
		all, err := everywhere(dir, recipient)
		if err != nil {
			return nil, err
		}
		for _, message := range all {
			if message.From != name || message.FromEpoch != epoch || !written(message) {
				continue
			}
			if message.ID == reference {
				return []Message{message}, nil
			}
			_, tail, _ := strings.Cut(message.ID, "-")
			if strings.HasPrefix(message.ID, reference) || strings.HasPrefix(tail, reference) {
				matches = append(matches, message)
			}
		}
	}
	return matches, nil
}

// written says a sender wrote this message itself, rather than rewake on its
// behalf.
func written(message Message) bool {
	if message.Compaction != nil || message.Departure != nil || message.Availability != nil || message.Undelivered != nil || message.Recall != nil {
		return false
	}
	switch KindOf(message) {
	case Task, Question, Note:
		return true
	}
	return false
}
