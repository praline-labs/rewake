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
	Causes       []string       `json:"causes,omitempty"`
	Parent       string         `json:"parent,omitempty"`
	Decided      *decisionModel `json:"decided"`
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
		head := fmt.Sprintf("%s · %s to %s (run %s), operation %s", s.Report, s.Kind, s.Recipient, s.RecipientRun, operationText(s.Operation))
		if len(s.Answers) > 0 {
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
	fmt.Fprintf(&b, "report %s of %s to %s %s — decided by %s, unproven; operation %s", m.Report, m.Session, m.Recipient, choiceText(d.Choice), deciderText(d.By), operationText(m.Operation))
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
		Operation: operationModel{Journal: s.Journal, Op: s.Op}, Answers: view.Answers, Causes: view.Causes, Parent: s.Parent(),
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

func decisionOf(view mail.DecisionView, durable bool, views map[string]mail.DecisionView) decisionModel {
	d := view.Decision
	model := decisionModel{
		ID: d.ID, Choice: d.Choice, At: d.At, Delivery: d.Delivery, Parent: d.Parent, Durable: durable,
		By: deciderModel{Kind: d.By.Kind, Name: d.By.Name, Run: d.By.Run, Verified: d.By.Verified}, Notes: []noteModel{},
	}
	if d.Choice == mail.ChoiceNotArrived {
		resend := resendOf(view, views)
		model.Resend = &resend
	}
	for _, note := range d.Notes {
		told := view.Told[note.Told]
		state := "owed"
		switch {
		case told.Err != nil:
			state = "unreadable"
		case told.Record != nil:
			state = "told"
		}
		model.Notes = append(model.Notes, noteModel{Told: note.Told, To: note.Note.To, State: state})
	}
	switch {
	case view.Closure != nil:
		model.Closure = "closed"
	case view.ClosureErr != nil:
		model.Closure = "unknown"
	default:
		model.Closure = "owed"
	}
	return model
}

// resendOf is how far a resend went, as its journal's forms keep it: a
// completed name alone never reads as published.
func resendOf(view mail.DecisionView, views map[string]mail.DecisionView) resendModel {
	r := view.Resend
	model := resendModel{Report: view.Decision.Resent()}
	switch {
	case r.Err != nil:
		model.State, model.Error = "unknown", r.Err.Error()
	case !r.Installed:
		model.State = "pending"
	case !r.Done:
		model.State = "in-progress"
	case r.Summary == nil:
		model.State = "completed"
	default:
		model.State, model.Child = r.Summary.Disposition, r.Summary.Child
		if child, ok := views[r.Summary.Report]; ok && child.Decision != nil && child.Decision.ID == r.Summary.Child {
			model.DecidedBy = deciderText(deciderModel{Kind: child.Decision.By.Kind, Name: child.Decision.By.Name, Verified: child.Decision.By.Verified})
		}
	}
	return model
}

func resendText(r resendModel, recipient string) string {
	switch r.State {
	case mail.DispositionPublished:
		return "published"
	case mail.DispositionMoot:
		return fmt.Sprintf("moot: %s's run ended; whether it landed is not known", recipient)
	case mail.DispositionDecided:
		if r.DecidedBy != "" {
			return fmt.Sprintf("unknown, decided by %s as %s", r.DecidedBy, r.Child)
		}
		return "unknown, decided as " + r.Child
	case "in-progress":
		return "in progress"
	case "completed":
		return "completed, outcome not kept"
	case "unknown":
		return "how far it went does not read: " + r.Error
	}
	return "resend pending"
}

func decidedText(d decisionModel, recipient string) string {
	text := fmt.Sprintf("decided %s, %s, by %s at %s, unproven", d.ID, choiceText(d.Choice), deciderText(d.By), d.At.Local().Format(time.DateTime))
	if !d.Durable {
		text += ", not durable yet — it settles nothing until a writer makes it durable"
	}
	if d.Resend != nil {
		text += fmt.Sprintf("; sent again as %s (%s)", d.Resend.Report, resendText(*d.Resend, recipient))
	}
	if owed := owedNotes(d.Notes); len(owed) > 0 {
		text += "; notes to " + strings.Join(owed, ", ") + " owed"
	}
	switch d.Closure {
	case "closed":
		text += "; closed by " + d.ID + ", delivery unproven"
	case "unknown":
		text += "; whether its tail closed what it answers does not read"
	default:
		text += "; still owed until its operation's tail runs"
	}
	return text
}

func owedNotes(notes []noteModel) []string {
	var owed []string
	for _, note := range notes {
		if note.State != "told" {
			owed = append(owed, note.To)
		}
	}
	return owed
}

func choiceText(choice string) string {
	if choice == mail.ChoiceArrived {
		return "arrived"
	}
	return "did not arrive"
}

func deciderText(by deciderModel) string {
	if by.Kind == mail.ByPerson {
		return "the person (unverified)"
	}
	if by.Verified {
		return by.Name + " (verified)"
	}
	return by.Name + " (unverified)"
}

func operationText(op operationModel) string {
	if op.Op == "" {
		return op.Journal
	}
	return op.Journal + " " + op.Op
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
