package mail

// TasksAnswered finds the tasks and questions a report answers, from its
// inReplyTo, in the mailbox of the session that wrote the report: the letters
// it read, wherever they lie now. A letter no longer there, swept after its
// day, is not among them. It reads the letters and writes nothing.
func TasksAnswered(dir string, report Message) ([]Message, error) {
	if len(report.InReplyTo) == 0 || report.From == "" {
		return nil, nil
	}
	all, err := everywhere(dir, report.From)
	if err != nil {
		return nil, err
	}
	wanted := map[string]bool{}
	for _, id := range report.InReplyTo {
		wanted[id] = true
	}
	var tasks []Message
	for _, message := range all {
		if wanted[message.ID] {
			tasks = append(tasks, message)
		}
	}
	return tasks, nil
}

// WithBriefNote returns the message with the note its notice's preview ends
// with, "brief 96 7/8" (internal/core/reportbrief). The note is of the copy
// handed to an adapter: it is not in the record, and no record carries it.
func (m Message) WithBriefNote(note string) Message {
	m.briefNote = note
	return m
}

// BriefNote is the note WithBriefNote set.
func (m Message) BriefNote() string { return m.briefNote }
