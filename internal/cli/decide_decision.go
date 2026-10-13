package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
)

// A decision as decide prints it, in the list and in the answer: its model
// from the records, and the words for its resend, notes and closure.

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
