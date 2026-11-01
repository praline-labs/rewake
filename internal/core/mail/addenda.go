package mail

// maxReplacements bounds how many edits currentTask follows. A chain is one
// edit per step and a person makes a handful; the bound only keeps a record
// written by hand into a loop from turning into one here.
const maxReplacements = 64

// currentTask is the task an id stands for now in the mailbox of to: the id
// itself, or the letter rewake edit sent in its place, through as many edits
// as there were. An addendum names its task as it was when the addendum was
// sent, and an edit never rewrites the addenda already in a mailbox — a
// letter's content does not change once written — so the link is followed
// here, through the tombstone each edit leaves under the old id. A tombstone
// no longer kept ends the walk where it is. An error says a copy on the way
// could not be read, and the walk may have stopped short of the task that
// stands now.
func currentTask(dir, to, id string) (string, error) {
	for range maxReplacements {
		if !safeID(id) {
			return id, nil
		}
		message, kept, err := readCopy(dir, to, id)
		if err != nil {
			return id, err
		}
		if !kept || message.Withdrawn == nil || message.Withdrawn.ReplacedBy == "" {
			return id, nil
		}
		id = message.Withdrawn.ReplacedBy
	}
	return id, nil
}

// AddedTo is the task an addendum adds to now (currentTask): the one it
// names, or the letter an edit sent in its place.
func (b Box) AddedTo(addendum Message) (string, error) {
	return currentTask(b.dir, addendum.To, addendum.AddendumTo)
}

// AddendaOf lists the addenda that add to a task this run sent now: sent by
// the same run to the same recipient, not withdrawn, naming the task or a
// letter it replaced. The caller that acts on them holds the recipient's
// mailbox lock. An error says a letter could not be read, which may be an
// addendum.
func (b Box) AddendaOf(task Message) ([]Message, error) {
	all, err := everywhere(b.dir, task.To)
	if err != nil {
		return nil, err
	}
	var addenda []Message
	for _, message := range all {
		if message.AddendumTo == "" || message.Withdrawn != nil || message.From != task.From || message.FromEpoch != task.FromEpoch || message.ID == task.ID {
			continue
		}
		current, err := currentTask(b.dir, task.To, message.AddendumTo)
		if err != nil {
			return nil, err
		}
		if current == task.ID {
			addenda = append(addenda, message)
		}
	}
	return addenda, nil
}
