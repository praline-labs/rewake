package cli

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/taskbrief"
	"github.com/praline-labs/rewake/internal/infra/proc"
)

// The views of briefs. Their --json is what rewake's shell reads, polling as
// main's run (docs/briefs.md): a field is added, never renamed or retyped.

// briefSummary is one brief as list shows it, and the head of show.
type briefSummary struct {
	ID       int        `json:"id"`
	Title    string     `json:"title"`
	Closed   bool       `json:"closed"`
	Created  time.Time  `json:"created"`
	ClosedAt *time.Time `json:"closedAt,omitempty"`
	// Edition counts main's edits; a tick names it. Revision is the log's
	// latest edit, tick or untick; an edit names it.
	Edition  int `json:"edition"`
	Revision int `json:"revision"`
	// Done and Total count the current edition's items.
	Done         int        `json:"done"`
	Total        int        `json:"total"`
	CommentCount int        `json:"commentCount"`
	Bound        []boundRun `json:"bound"`
}

// boundRun is a run the brief was sent to.
type boundRun struct {
	Name  string    `json:"name"`
	Epoch string    `json:"epoch"`
	Since time.Time `json:"since"`
	// Run is whether that run lives: running, ended or unknown. Its right
	// ends with it, and every right ends when the brief closes.
	Run string `json:"run"`
	// Kind is its right: work reads, ticks and comments, review reads and
	// comments.
	Kind taskbrief.BindingKind `json:"kind"`
}

type briefListing struct {
	Room   string         `json:"room"`
	Briefs []briefSummary `json:"briefs"`
	// Unreadable are the entries of the briefs directory that do not read as
	// a brief; one among them is neither listed nor taken for absent.
	Unreadable []taskbrief.Problem `json:"unreadable"`
}

// briefDetail is one brief as show gives it.
type briefDetail struct {
	briefSummary
	// Text is the current text with the ticks applied and no numbers: what
	// main edits and the shell renders.
	Text     string           `json:"text"`
	Items    []taskbrief.Item `json:"items"`
	Comments []commentView    `json:"comments"`
	// numbered is the text a worker reads, each item's number after its box.
	numbered string
}

type commentView struct {
	N       int       `json:"n"`
	From    string    `json:"from"`
	At      time.Time `json:"at"`
	Edition int       `json:"edition"`
	Text    string    `json:"text"`
	// Capacity is the kind of the writer's binding when it wrote, absent on
	// a comment written before kinds.
	Capacity taskbrief.BindingKind `json:"capacity,omitempty"`
}

// author is who wrote the comment and in what capacity, as every reader is
// shown it: "review-pi · review", or the name alone for an old comment.
func (c commentView) author() string {
	if c.Capacity == "" {
		return c.From
	}
	return c.From + " · " + string(c.Capacity)
}

// asMain is the reader main is, beside the kinds of a binding.
const asMain taskbrief.BindingKind = "main"

func briefSummaryOf(brief taskbrief.Brief) briefSummary {
	summary := briefSummary{
		ID: brief.ID, Title: brief.Title(), Closed: brief.IsClosed(), Created: brief.Created, ClosedAt: brief.Closed,
		Edition: brief.Edition, Revision: brief.Revision, Done: brief.Done(), Total: len(brief.Items()),
		CommentCount: len(brief.Comments()), Bound: []boundRun{},
	}
	for _, binding := range brief.Bound {
		run := map[proc.IdentityEvidence]string{proc.IdentityAlive: "running", proc.IdentityEnded: "ended"}[registry.ObserveRun(binding.Epoch)]
		if run == "" {
			run = "unknown"
		}
		summary.Bound = append(summary.Bound, boundRun{Name: binding.Name, Epoch: binding.Epoch, Since: binding.Since, Run: run, Kind: binding.Kind.Of()})
	}
	return summary
}

func briefDetailOf(brief taskbrief.Brief) briefDetail {
	view := briefDetail{briefSummary: briefSummaryOf(brief), Text: brief.Markdown(), Items: brief.Items(), Comments: []commentView{}, numbered: brief.Numbered()}
	if view.Items == nil {
		view.Items = []taskbrief.Item{}
	}
	for i, entry := range brief.Comments() {
		view.Comments = append(view.Comments, commentView{N: i + 1, From: entry.From, At: entry.At, Edition: entry.Edition, Text: entry.Text, Capacity: entry.Capacity})
	}
	return view
}

func briefList(ctx *Context, call Call, home taskbrief.Home, caller briefRun, args []string) error {
	if len(args) > 0 {
		return &UsageError{Command: call.Command, Message: "brief list takes no other argument."}
	}
	if err := caller.mainOnly("list"); err != nil {
		return err
	}
	briefs, problems, err := taskbrief.List(home, call.Switch("closed"))
	if err != nil {
		return failf("could not read the briefs: %v", err)
	}
	listing := briefListing{Room: filepath.Base(home.Room), Briefs: []briefSummary{}, Unreadable: problems}
	if listing.Unreadable == nil {
		listing.Unreadable = []taskbrief.Problem{}
	}
	for _, brief := range briefs {
		listing.Briefs = append(listing.Briefs, briefSummaryOf(brief))
	}
	return printValue(ctx, listing, listing.lines)
}

func (l briefListing) lines() []string {
	var lines []string
	if len(l.Briefs) == 0 {
		lines = append(lines, "Rewake: no briefs in room "+l.Room+"; main makes one with rewake brief new \"# Title ...\".")
	}
	for _, brief := range l.Briefs {
		line := fmt.Sprintf("%d · %s · %d/%d · %s", brief.ID, brief.Title, brief.Done, brief.Total, plural(brief.CommentCount, "comment"))
		if brief.Closed {
			line += " · closed"
		}
		if len(brief.Bound) > 0 {
			line += " · " + boundNames(brief)
		}
		lines = append(lines, line)
	}
	for _, problem := range l.Unreadable {
		lines = append(lines, "unreadable: "+problem.Path+": "+problem.Why)
	}
	return lines
}

func briefShow(ctx *Context, call Call, home taskbrief.Home, caller briefRun, args []string) error {
	id, err := briefID(call, "show", args)
	if err != nil {
		return err
	}
	if len(args) > 1 {
		return &UsageError{Command: call.Command, Message: "brief show takes the id alone."}
	}
	brief, err := taskbrief.Load(home, id)
	if err != nil {
		return briefError(call, err)
	}
	reader := asMain
	if !caller.isMain() {
		binding, bound := brief.BindingOf(caller.author)
		if !bound {
			return briefError(call, &taskbrief.NotBoundError{ID: id, Name: caller.author.Name})
		}
		if brief.IsClosed() {
			return briefError(call, &taskbrief.ClosedError{ID: id})
		}
		reader = binding.Kind.Of()
	}
	view := briefDetailOf(brief)
	if call.Switch("markdown") && !ctx.JSON {
		return emit(ctx, strings.Split(strings.TrimSuffix(view.Text, "\n"), "\n")...)
	}
	return printValue(ctx, view, func() []string { return view.lines(reader) })
}

// lines are the brief as a reader reads it — main, or a run by the kind of
// its binding: a header, how to act on it, the text with each item's number,
// and the comments with who wrote each.
func (d briefDetail) lines(reader taskbrief.BindingKind) []string {
	lines := []string{d.header()}
	if len(d.Bound) > 0 {
		lines = append(lines, "bound: "+boundNames(d.briefSummary))
	}
	lines = append(lines, d.how(reader), "")
	lines = append(lines, strings.Split(strings.TrimSuffix(d.numbered, "\n"), "\n")...)
	if len(d.Comments) == 0 {
		return lines
	}
	lines = append(lines, "", "comments:")
	for _, comment := range d.Comments {
		lines = append(lines, fmt.Sprintf("%s · %s · #%d", comment.author(), comment.At.Local().Format("15:04:05"), comment.N))
		// A comment is markdown: each line indented under its header, a
		// blank one left blank.
		for _, line := range strings.Split(comment.Text, "\n") {
			if line != "" {
				line = "  " + line
			}
			lines = append(lines, line)
		}
	}
	return lines
}

func (d briefDetail) header() string {
	header := fmt.Sprintf("brief %d · %s · edition %d · revision %d · %d of %d done · %s", d.ID, d.Title, d.Edition, d.Revision, d.Done, d.Total, plural(d.CommentCount, "comment"))
	if d.Closed {
		header += " · closed"
	}
	return header
}

func (d briefDetail) how(reader taskbrief.BindingKind) string {
	switch {
	case d.Closed:
		return "Rewake: this brief is closed; it takes no edit, tick or comment."
	case reader == asMain:
		return fmt.Sprintf("Rewake: edit the text rewake brief show %d --markdown prints, then rewake brief edit %d --rev %d --file <path>; give it with rewake send <name> \"...\" --brief %d, or --brief %d --review to a reviewer.", d.ID, d.ID, d.Revision, d.ID, d.ID)
	case reader == taskbrief.Review:
		return fmt.Sprintf("Rewake: this brief was given to you to review: comment with rewake brief comment %d -- \"<text>\", read it again with rewake brief show %d; the tools brief_comment and brief_read do the same. Ticking is the worker's: say in a comment which items you find done or not.", d.ID, d.ID)
	}
	return fmt.Sprintf("Rewake: tick item n with rewake brief tick %d <n> --edition %d (--undo to untick), comment with rewake brief comment %d -- \"<text>\", read it again with rewake brief show %d; the tools brief_tick, brief_comment and brief_read do the same.", d.ID, d.Edition, d.ID, d.ID)
}

func boundNames(summary briefSummary) string {
	names := make([]string, 0, len(summary.Bound))
	for _, run := range summary.Bound {
		name := run.Name
		var marks []string
		if run.Kind == taskbrief.Review {
			marks = append(marks, "review")
		}
		if summary.Closed || run.Run == "ended" {
			marks = append(marks, "ended")
		}
		if len(marks) > 0 {
			name += " (" + strings.Join(marks, ", ") + ")"
		}
		names = append(names, name)
	}
	return strings.Join(names, ", ")
}
