package cli

import (
	"fmt"
	"strings"

	"github.com/praline-labs/rewake/internal/core/taskbrief"
)

// letterBrief is the brief a letter gives, read when the letter is: the
// current text, so a worker that re-reads its task after a compaction reads
// what main has edited since. It is never copied into the letter.
type letterBrief struct {
	ID int `json:"id"`
	*briefDetail
	// Error says why the brief could not be read; the letter is shown whole
	// all the same (E5, E6).
	Error string `json:"error,omitempty"`
	// EarlierRun says the brief was given to an earlier run of the name, not
	// to the reader, which gets no right to it and is not shown it.
	EarlierRun bool `json:"earlierRun,omitempty"`
}

// withBriefs reads the brief of each letter that gives one, for the run
// reading them: a letter left unread by an earlier run of the name shows its
// brief only to a run it was given to, as brief show would.
func withBriefs(dir string, views []messageView, name, epoch string) []messageView {
	reader := taskbrief.Author{Name: name, Epoch: epoch}
	for index := range views {
		views[index].BriefView = letterBriefOf(dir, views[index].Brief, reader)
	}
	return views
}

func letterBriefOf(dir, named string, reader taskbrief.Author) *letterBrief {
	if named == "" {
		return nil
	}
	id, ok := taskbrief.ParseID(named)
	if !ok {
		return &letterBrief{Error: fmt.Sprintf("the letter names %q, which is no brief's id", named)}
	}
	brief, err := taskbrief.Load(dir, id)
	if err != nil {
		return &letterBrief{ID: id, Error: err.Error()}
	}
	if !brief.Has(reader) {
		return &letterBrief{ID: id, EarlierRun: true}
	}
	view := briefDetailOf(brief)
	return &letterBrief{ID: id, briefDetail: &view}
}

// briefLetterLines print a letter's brief under its text: a header, how to
// act on it, and the text with each item's number.
func briefLetterLines(brief *letterBrief) []string {
	if brief == nil {
		return nil
	}
	if brief.EarlierRun {
		return []string{"", fmt.Sprintf("Rewake: this letter gave brief %d to an earlier run of this session, not to this one; main can give it again with rewake send <name> \"...\" --brief %d.", brief.ID, brief.ID)}
	}
	if brief.briefDetail == nil {
		return []string{"", fmt.Sprintf("Rewake: this letter gives brief %d, which could not be read: %s. Read it with rewake brief show %d.", brief.ID, brief.Error, brief.ID)}
	}
	d := brief.briefDetail
	header := fmt.Sprintf("brief %d · %s · edition %d · %d of %d done · %s", d.ID, d.Title, d.Edition, d.Done, d.Total, plural(d.CommentCount, "comment"))
	if d.Closed {
		header += " · closed"
	}
	lines := []string{"", header, d.how(false), ""}
	return append(lines, strings.Split(strings.TrimSuffix(d.numbered, "\n"), "\n")...)
}
