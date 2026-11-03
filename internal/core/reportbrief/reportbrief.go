// Package reportbrief reads the state of the brief a report answers, at the
// moment the report is read or announced (docs/briefs.md#what-main-reads): a
// worker given a task with --brief finishes, and main sees in the same place
// what the brief still holds open and what was said in it since. Nothing is
// written; the tie is the letters' own, a report's inReplyTo naming the task
// the worker read and the task's brief field naming the brief.
package reportbrief

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/taskbrief"
)

// Comment is one comment made in the brief after the task was sent.
type Comment struct {
	From     string                `json:"from"`
	Capacity taskbrief.BindingKind `json:"capacity,omitempty"`
	At       time.Time             `json:"at"`
	Text     string                `json:"text"`
}

// Block is the state of one brief a report answers.
type Block struct {
	ID     int  `json:"id"`
	Done   int  `json:"done"`
	Total  int  `json:"total"`
	Closed bool `json:"closed,omitempty"`
	// Unticked are the items still open, by number and text.
	Unticked []taskbrief.Item `json:"unticked"`
	// Comments are those made after the task was sent, by anyone.
	Comments []Comment `json:"comments"`
	// Error says why the brief could not be read; the report stands without
	// its state.
	Error string `json:"error,omitempty"`
}

// Of reads the briefs the report answers, one block each, in the order of
// their ids. A report that answers no task given with a brief has none, and
// a task that cannot be found leaves the report as it is.
func Of(dir string, home taskbrief.Home, report mail.Message) []Block {
	if len(report.InReplyTo) == 0 || !mail.IsReport(report) {
		return nil
	}
	tasks, err := mail.TasksAnswered(dir, report)
	if err != nil {
		return nil
	}
	since := map[int]time.Time{}
	var ids []int
	for _, task := range tasks {
		id, ok := taskbrief.ParseID(task.Brief)
		if !ok {
			continue
		}
		if first, seen := since[id]; !seen {
			ids = append(ids, id)
			since[id] = task.CreatedAt
		} else if task.CreatedAt.Before(first) {
			since[id] = task.CreatedAt
		}
	}
	slices.Sort(ids)
	blocks := make([]Block, 0, len(ids))
	for _, id := range ids {
		blocks = append(blocks, read(home, id, since[id]))
	}
	return blocks
}

func read(home taskbrief.Home, id int, since time.Time) Block {
	block := Block{ID: id, Unticked: []taskbrief.Item{}, Comments: []Comment{}}
	brief, err := taskbrief.Load(home, id)
	if err != nil {
		block.Error = err.Error()
		return block
	}
	block.Done, block.Total, block.Closed = brief.Done(), len(brief.Items()), brief.IsClosed()
	for _, item := range brief.Items() {
		if !item.Done {
			block.Unticked = append(block.Unticked, item)
		}
	}
	for _, entry := range brief.Comments() {
		if entry.At.After(since) {
			block.Comments = append(block.Comments, Comment{From: entry.From, Capacity: entry.Capacity, At: entry.At, Text: entry.Text})
		}
	}
	return block
}

// Note is what a notice's preview adds, "brief 96 7/8"; empty when the brief
// could not be read.
func (b Block) Note() string {
	if b.Error != "" {
		return ""
	}
	return fmt.Sprintf("brief %d %d/%d", b.ID, b.Done, b.Total)
}

// Notes joins the notes of the blocks, for a report that answers several
// briefs.
func Notes(blocks []Block) string {
	var notes []string
	for _, block := range blocks {
		if note := block.Note(); note != "" {
			notes = append(notes, note)
		}
	}
	return strings.Join(notes, ", ")
}

// Lines print a block under its report: the done count, what is still open,
// and each comment in full with its author, capacity and time.
func (b Block) Lines() []string {
	if b.Error != "" {
		return []string{fmt.Sprintf("── brief %d could not be read: %s. Read it with rewake brief show %d.", b.ID, b.Error, b.ID)}
	}
	head := fmt.Sprintf("── brief %d · %d/%d", b.ID, b.Done, b.Total)
	if b.Closed {
		head += " · closed"
	}
	lines := []string{head}
	if len(b.Unticked) == 0 {
		lines = append(lines, "unticked: none")
	} else {
		lines = append(lines, "unticked:")
		for _, item := range b.Unticked {
			lines = append(lines, fmt.Sprintf("  #%d %s", item.N, item.Text))
		}
	}
	if len(b.Comments) == 0 {
		return append(lines, "comments since the task: none")
	}
	lines = append(lines, "comments since the task:")
	for _, comment := range b.Comments {
		who := comment.From
		if comment.Capacity != "" {
			who += " · " + string(comment.Capacity)
		}
		text := strings.Split(strings.TrimRight(comment.Text, "\n"), "\n")
		lines = append(lines, fmt.Sprintf("  %s · %s · %s", who, comment.At.Local().Format("15:04"), text[0]))
		for _, rest := range text[1:] {
			lines = append(lines, strings.TrimRight("    "+rest, " "))
		}
	}
	return lines
}
