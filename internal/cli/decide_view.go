package cli

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
)

// What decide prints: the list form, and the answer of a decision made or
// found made. Both are read from the mailbox after the fact, never from the
// wrapper's reply, so the words say only what the records establish
// (Q10 of docs/v2/stage3-decision-build.md): a phase not reached is pending,
// never "not done", and delivery is never claimed.

type operationModel struct {
	Journal string `json:"journal"`
	Op      string `json:"op,omitempty"`
}

type deciderModel struct {
	Kind     string `json:"kind"`
	Name     string `json:"name,omitempty"`
	Run      string `json:"run,omitempty"`
	Verified bool   `json:"verified"`
}

// resendModel is the report a decision on "did not arrive" sends again and
// how far it went: pending, in-progress, published, moot, decided (Child is
// the decision on it), completed (its outcome not kept) or unknown.
type resendModel struct {
	Report string `json:"report"`
	State  string `json:"state"`
	Child  string `json:"child,omitempty"`
	// DecidedBy is who decided the child, when its record reads.
	DecidedBy string `json:"decidedBy,omitempty"`
	Error     string `json:"error,omitempty"`
}

// noteModel is a note a decision owes: owed, told or unreadable.
type noteModel struct {
	Told  string `json:"told"`
	To    string `json:"to"`
	State string `json:"state"`
}

type decisionModel struct {
	ID       string       `json:"id"`
	Choice   string       `json:"choice"`
	By       deciderModel `json:"by"`
	At       time.Time    `json:"at"`
	Delivery string       `json:"delivery"`
	Parent   string       `json:"parent,omitempty"`
	// Durable is false for a record not made durable yet, which settles
	// nothing until a writer makes it so.
	Durable bool         `json:"durable"`
	Resend  *resendModel `json:"resend,omitempty"`
	Notes   []noteModel  `json:"notes"`
	// Closure is owed until the tail of the report's operation closes what
	// it answers, closed after, unknown when its record does not read.
	Closure string `json:"closure"`
}

type undecidableModel struct {
	Path string `json:"path"`
	Why  string `json:"why"`
}

type subjectModel struct {
	Report       string         `json:"report"`
	Kind         mail.Kind      `json:"kind"`
	Recipient    string         `json:"recipient"`
	RecipientRun string         `json:"recipientRun"`
	Operation    operationModel `json:"operation"`
	Answers      []string       `json:"answers"`
	// AnswersUnknown says the records that named what it answers are gone or
	// do not read: answers is then empty without the report answering none.
	AnswersUnknown bool           `json:"answersUnknown,omitempty"`
	Causes         []string       `json:"causes,omitempty"`
	Parent         string         `json:"parent,omitempty"`
	Decided        *decisionModel `json:"decided"`
}

type decideListModel struct {
	Session     string             `json:"session"`
	Subjects    []subjectModel     `json:"subjects"`
	Undecidable []undecidableModel `json:"undecidable"`
}

type decideModel struct {
	Session      string             `json:"session"`
	Report       string             `json:"report"`
	Recipient    string             `json:"recipient"`
	RecipientRun string             `json:"recipientRun"`
	Operation    operationModel     `json:"operation"`
	Decision     decisionModel      `json:"decision"`
	Recorded     bool               `json:"recorded"`
	Before       bool               `json:"before"`
	Resent       string             `json:"resent,omitempty"`
	Stopped      bool               `json:"stopped"`
	Remaining    []string           `json:"remaining"`
	Undecidable  []undecidableModel `json:"undecidable"`
	Barrier      string             `json:"barrier,omitempty"`
	Failed       string             `json:"failed,omitempty"`
}

// listDecisions prints what can be decided in name's mailbox.
func listDecisions(ctx *Context, dir, name string) error {
	listing, _ := mail.ListDecisions(dir, name)
	views := viewsByReport(dir, name)
	model := decideListModel{Session: name, Subjects: []subjectModel{}, Undecidable: undecidableOf(listing)}
	for _, subject := range listing.Subjects {
		model.Subjects = append(model.Subjects, subjectOf(subject, views))
	}
	return printValue(ctx, model, func() []string { return listLines(model) })
}

func listLines(model decideListModel) []string {
	if len(model.Subjects) == 0 && len(model.Undecidable) == 0 {
		return []string{fmt.Sprintf("Rewake: %s has no report to decide.", model.Session)}
	}
	var lines []string
	for _, s := range model.Subjects {
		head := fmt.Sprintf("%s · %s to %s (run %s), %s", s.Report, s.Kind, s.Recipient, s.RecipientRun, operationText(s.Operation))
		switch {
		case s.AnswersUnknown:
			head += ", what it answers no longer kept"
		case len(s.Answers) > 0:
			head += ", answering " + strings.Join(s.Answers, ", ")
		}
		if s.Parent != "" {
			head += ", sent again by decision " + s.Parent
		}
		if s.Decided != nil {
			lines = append(lines, head+": "+decidedText(*s.Decided, s.Recipient)+".")
			continue
		}
		lines = append(lines, head+": unknown — "+strings.Join(s.Causes, "; ")+".",
			fmt.Sprintf("  decide: rewake decide %s %s --arrived | --not-arrived", model.Session, s.Report))
	}
	for _, u := range model.Undecidable {
		lines = append(lines, fmt.Sprintf("undecidable: %s — %s; no decision settles it.", u.Path, u.Why))
	}
	return lines
}

// decided is what printDecided needs of the call beyond the mailbox.
type decided struct {
	report  string
	before  bool
	barrier error
	failed  error
}

// printDecided answers a decision made, found made, or made with an effect
// that failed after it, from the records as they are now.
func printDecided(ctx *Context, dir, name string, answer mail.DecideAnswer, failed error) error {
	result := decided{report: answer.Decision.Subject.Report, before: answer.Before, barrier: answer.Barrier, failed: failed}
	views := viewsByReport(dir, name)
	view, ok := views[result.report]
	if !ok || view.Decision == nil {
		why := "it is not listed"
		if ok {
			why = view.Err.Error()
		}
		return failf("Rewake: the decision on report %s of %s was answered recorded, and its record does not read now (%s); nothing more is known. See: rewake decide %s", result.report, name, why, name)
	}
	listing, _ := mail.ListDecisions(dir, name)
	model := decideModel{
		Session: name, Report: result.report, Recorded: true, Before: result.before,
		Decision: decisionOf(view, true, views), Undecidable: undecidableOf(listing), Remaining: []string{},
	}
	s := view.Decision.Subject
	model.Recipient, model.RecipientRun = s.Recipient, s.RecipientRun
	model.Operation = operationModel{Journal: s.Journal, Op: s.Op}
	if model.Decision.Resend != nil {
		model.Resent = model.Decision.Resend.Report
	}
	for _, subject := range listing.Subjects {
		if subject.Decided == nil && subject.Subject.Report != result.report {
			model.Remaining = append(model.Remaining, subject.Subject.Report)
		}
	}
	model.Stopped = len(model.Remaining) > 0 || len(model.Undecidable) > 0
	if result.barrier != nil {
		model.Barrier = result.barrier.Error()
	}
	if result.failed != nil {
		model.Failed = result.failed.Error()
	}
	lines := func() []string { return []string{decideLine(model)} }
	if result.failed != nil {
		if ctx.JSON {
			_ = printValue(ctx, model, func() []string { return nil })
			return unfinished(ctx, "")
		}
		return unfinished(ctx, decideLine(model))
	}
	return printValue(ctx, model, lines)
}

func decideLine(m decideModel) string {
	d := m.Decision
	var b strings.Builder
	if m.Before {
		fmt.Fprintf(&b, "Rewake: recorded before as %s at %s: ", d.ID, d.At.Local().Format(time.DateTime))
	} else {
		b.WriteString("Rewake: recorded: ")
	}
	fmt.Fprintf(&b, "report %s of %s to %s %s — decided by %s, unproven; %s", m.Report, m.Session, m.Recipient, choiceText(d.Choice), deciderText(d.By), operationText(m.Operation))
	if r := d.Resend; r != nil {
		fmt.Fprintf(&b, "; sent again as %s (%s)", r.Report, resendText(*r, m.Recipient))
	}
	if owed := owedNotes(d.Notes); len(owed) > 0 {
		fmt.Fprintf(&b, "; notes to %s owed, a later barrier tells them", strings.Join(owed, ", "))
	}
	if m.Failed != "" {
		fmt.Fprintf(&b, "; an effect it fixes failed: %s; the next barrier does it", m.Failed)
	}
	switch {
	case len(m.Remaining) > 0:
		fmt.Fprintf(&b, "; %s stays stopped on %s, decide each the same way", m.Session, strings.Join(m.Remaining, ", "))
	case len(m.Undecidable) > 0:
		fmt.Fprintf(&b, "; %s stays stopped on what no decision settles: %s — %s", m.Session, m.Undecidable[0].Path, m.Undecidable[0].Why)
	case m.Barrier != "":
		fmt.Fprintf(&b, "; %s waits: %s", m.Session, m.Barrier)
	case m.Failed == "":
		fmt.Fprintf(&b, "; %s goes on", m.Session)
	}
	if m.Stopped && d.Resend != nil {
		b.WriteString("; the resend and its tail run once the stop is settled")
	}
	b.WriteString(".")
	return b.String()
}

// viewsByReport are the decisions of a mailbox by report; a listing that
// does not read gives none, and the listing names it undecidable.
func viewsByReport(dir, name string) map[string]mail.DecisionView {
	views, _ := mail.Decisions(dir, name)
	byReport := make(map[string]mail.DecisionView, len(views))
	for _, view := range views {
		byReport[view.Report] = view
	}
	return byReport
}

func subjectOf(view mail.SubjectView, views map[string]mail.DecisionView) subjectModel {
	s := view.Subject
	model := subjectModel{
		Report: s.Report, Kind: s.Kind, Recipient: s.Recipient, RecipientRun: s.RecipientRun,
		Operation: operationModel{Journal: s.Journal, Op: s.Op}, Answers: view.Answers, AnswersUnknown: view.AnswersUnknown, Causes: view.Causes, Parent: s.Parent(),
	}
	if model.Answers == nil {
		model.Answers = []string{}
	}
	if view.Decided != nil {
		decision := decisionOf(*view.Decided, view.Durable, views)
		model.Decided = &decision
	}
	return model
}

// operationText names a report's operation once: a turn journal is named by
// its operation, so the two ids are mostly one; when they differ, each says
// which it is.
func operationText(op operationModel) string {
	if op.Op == "" || op.Op == op.Journal {
		return "operation " + op.Journal
	}
	return "operation " + op.Op + " in journal " + op.Journal
}

func undecidableOf(listing mail.DecisionListing) []undecidableModel {
	out := []undecidableModel{}
	for _, u := range listing.Undecidable {
		out = append(out, undecidableModel{Path: u.Path, Why: u.Why})
	}
	return out
}

// decisionRefusal answers a decision not made, or made and not finished, by
// its outcome: a wrong call, a final refusal, one the same words may still
// get past, or a record whose effects the next barrier finishes.
func decisionRefusal(ctx *Context, call Call, dir, name, report string, err error) error {
	var refusal *mail.DecisionError
	if !errors.As(err, &refusal) {
		return unfinished(ctx, fmt.Sprintf("Rewake: report %s of %s was not decided: %v.", report, name, err))
	}
	text := fmt.Sprintf("Rewake: report %s of %s was not decided (%s): %v.", report, name, refusal.Outcome, refusal)
	switch refusal.Outcome {
	case mail.DecisionNoSubject, mail.DecisionUnauthorized:
		return &UsageError{Command: call.Command, Message: text + " See what can be decided: rewake decide " + name}
	case mail.DecisionOpposite, mail.DecisionProven:
		return failf("%s", text)
	case mail.DecisionEffectFailed:
		return printDecided(ctx, dir, name, mail.DecideAnswer{Decision: mail.Decision{Subject: mail.Subject{Report: report}}}, refusal)
	case mail.DecisionUncertain:
		text = fmt.Sprintf("Rewake: whether report %s of %s was decided is unknown (%v); the record would be %s. The same words again find it or decide.", report, name, refusal.Err, refusal.Path)
	}
	return unfinished(ctx, text)
}
