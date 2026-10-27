package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
)

// awaitedView is one task or question this run sent and has no report on yet.
// Gone is "ended" or "replaced" when the run it was written for no longer
// lives: then no report is coming, whatever the state says.
type awaitedView struct {
	ID        string     `json:"id"`
	Kind      mail.Kind  `json:"kind"`
	CreatedAt time.Time  `json:"createdAt"`
	State     mail.Stage `json:"state"`
	Detail    string     `json:"detail,omitempty"`
	Gone      string     `json:"gone,omitempty"`
	Text      string     `json:"text"`
	// AddendumTo names the task this one adds to.
	AddendumTo string `json:"addendumTo,omitempty"`
	// nested is an addendum listed under its task.
	nested bool
	coming bool
}

type awaitedRecipient struct {
	Name     string        `json:"name"`
	Messages []awaitedView `json:"messages"`
}

type awaitedModel struct {
	Session    string             `json:"session"`
	Recipients []awaitedRecipient `json:"recipients"`
}

// showAwaited lists what this run sent and still waits on a report for, by
// recipient — what a main session hands out, for one that lost track of it to a
// compaction. It only reads: no lock, no record, nothing sent.
func showAwaited(ctx *Context, dir string, session registry.Session, epoch string) error {
	standings, err := mail.Open(dir, session.Name, epoch).Standings()
	if err != nil {
		// A partial list would show a task as answered, or lost and worth
		// sending again.
		return failf("could not read where what %s sent stands (%v); nothing is shown rather than part of it; run the same command again", session.Name, err)
	}
	model := awaitedModel{Session: session.Name, Recipients: []awaitedRecipient{}}
	index := map[string]int{}
	for _, standing := range standings {
		view := awaitedView{
			ID: standing.ID, Kind: mail.KindOf(standing.Message), CreatedAt: standing.CreatedAt,
			State: standing.Stage, Detail: standing.Detail, Text: standing.Text,
			Gone: standing.GoneReason(), AddendumTo: standing.Under, nested: standing.Nested, coming: standing.Coming(),
		}
		at, ok := index[standing.To]
		if !ok {
			at = len(model.Recipients)
			index[standing.To] = at
			model.Recipients = append(model.Recipients, awaitedRecipient{Name: standing.To})
		}
		model.Recipients[at].Messages = append(model.Recipients[at].Messages, view)
	}
	return printValue(ctx, model, func() []string { return awaitedLines(model.Recipients) })
}

func awaitedLines(recipients []awaitedRecipient) []string {
	var coming, lost int
	for _, recipient := range recipients {
		for _, message := range recipient.Messages {
			if message.coming {
				coming++
			} else {
				lost++
			}
		}
	}
	var header string
	switch {
	case coming == 0 && lost == 0:
		return []string{"Rewake: nobody owes you a report."}
	case lost == 0:
		header = fmt.Sprintf("Rewake: waiting on %s:", plural(coming, "report"))
	case coming == 0:
		header = fmt.Sprintf("Rewake: waiting on no reports; %d will not come:", lost)
	default:
		header = fmt.Sprintf("Rewake: waiting on %s; %d more will not come:", plural(coming, "report"), lost)
	}
	lines := []string{header}
	for _, recipient := range recipients {
		lines = append(lines, "", "to "+recipient.Name)
		for _, message := range recipient.Messages {
			// An addendum lists after its task, which sorts first: indented,
			// it reads as part of the same work.
			indent, kind := "", string(message.Kind)
			switch {
			case message.nested:
				indent, kind = "  + ", "addendum to "+shortRef(message.AddendumTo)
			case message.AddendumTo != "":
				// Its task is not listed — reported on, or no longer
				// kept — so it stands alone, still saying what it adds to.
				kind += ", addendum to " + shortRef(message.AddendumTo)
			}
			lines = append(lines,
				fmt.Sprintf("%s%s · %s · %s · %s", indent, message.ID, kind, message.CreatedAt.Local().Format("15:04:05"), awaitedState(recipient.Name, message)),
				strings.Repeat(" ", len(indent))+mail.Preview(message.Text),
			)
		}
	}
	return lines
}

// awaitedState says in a few words where a message stands. The whole detail
// is in --json; one line of it is enough to know what to do.
func awaitedState(recipient string, message awaitedView) string {
	switch message.Gone {
	case mail.GoneEnded:
		return "no report coming: " + recipient + " ended; check its saved work and give the task to a new worker"
	case mail.GoneReplaced:
		return "no report coming: " + recipient + " was replaced by a new run; check its saved work and give the task to a new worker"
	}
	detail := func(prefix string) string {
		if line := mail.Preview(message.Detail); line != "" {
			return prefix + ": " + line
		}
		return prefix
	}
	switch message.State {
	case mail.StageUndelivered:
		return detail("not delivered yet")
	case mail.StageHeld:
		return detail("held")
	case mail.StageUnread:
		return "delivered, unread"
	case mail.StagePending:
		return detail("pending")
	case mail.StageStopped:
		// The stop's own text says who stopped the turn: the person at the
		// keyboard, or a main by name with rewake interrupt.
		return detail("stopped")
	case mail.StageFailed:
		return detail("not delivered, no report coming")
	}
	return "read, being worked on"
}

func plural(count int, noun string) string {
	if count == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", count, noun)
}
