package inbox

// maxReplacements bounds how many edits CurrentTask follows. A chain is one
// edit per step and a person makes a handful; the bound only keeps a record
// written by hand into a loop from turning into one here.
const maxReplacements = 64

// CurrentTask is the task an id stands for now: the id itself, or the letter
// rewake edit sent in its place, through as many edits as there were. An
// addendum names its task as it was when the addendum was sent, and an edit
// never rewrites the addenda already in a mailbox — a letter's content does not
// change once written — so the link is followed here, through the tombstone
// each edit leaves under the old id. A tombstone no longer kept ends the walk
// where it is.
func CurrentTask(dir, to, id string) string {
	for range maxReplacements {
		if !safeID(id) {
			return id
		}
		message, kept := readCopy(dir, to, id)
		if !kept || message.Withdrawn == nil || message.Withdrawn.ReplacedBy == "" {
			return id
		}
		id = message.Withdrawn.ReplacedBy
	}
	return id
}

// AddendaOf lists the addenda that add to a task now: sent by the same run to
// the same recipient, not withdrawn, naming the task or a letter it replaced.
// The caller that acts on them holds the recipient's mailbox lock.
func AddendaOf(dir string, task Message) []Message {
	var addenda []Message
	for _, message := range everywhere(dir, task.To) {
		if message.AddendumTo == "" || message.Withdrawn != nil || message.From != task.From || message.FromEpoch != task.FromEpoch || message.ID == task.ID {
			continue
		}
		if CurrentTask(dir, task.To, message.AddendumTo) == task.ID {
			addenda = append(addenda, message)
		}
	}
	return addenda
}
