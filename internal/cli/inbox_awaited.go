package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/role"
)

// awaitedView is one task or question this run sent and has no report on yet.
// Gone is "ended" or "replaced" when the run it was written for no longer
// lives: then no report is coming, whatever the state says.
type awaitedView struct {
	ID        string      `json:"id"`
	Kind      inbox.Kind  `json:"kind"`
	CreatedAt time.Time   `json:"createdAt"`
	State     inbox.Stage `json:"state"`
	Detail    string      `json:"detail,omitempty"`
	Gone      string      `json:"gone,omitempty"`
	Text      string      `json:"text"`
	// AddendumTo names the task this one adds to.
	AddendumTo string `json:"addendumTo,omitempty"`
	// nested is an addendum listed under its task.
	nested bool
}

// afterTheirTask lists each addendum right after the task it adds to, which
// other tasks sent in between would otherwise separate it from.
func afterTheirTask(messages []awaitedView) []awaitedView {
	roots := map[string]bool{}
	for _, message := range messages {
		roots[message.ID] = message.AddendumTo == ""
	}
	ordered := make([]awaitedView, 0, len(messages))
	for _, message := range messages {
		if roots[message.AddendumTo] {
			continue
		}
		ordered = append(ordered, message)
		for _, addendum := range messages {
			if addendum.AddendumTo == message.ID && roots[message.ID] {
				addendum.nested = true
				ordered = append(ordered, addendum)
			}
		}
	}
	return ordered
}

// coming says whether a report on it can still arrive.
func (v awaitedView) coming() bool { return v.Gone == "" && v.State != inbox.StageFailed }

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
	type recipientRun struct {
		run    string
		silent bool
		found  bool
	}
	runs := map[string]recipientRun{}
	current := func(name string) recipientRun {
		if known, ok := runs[name]; ok {
			return known
		}
		// Read-only on purpose: Lookup prunes a dead record, and this view
		// writes nothing.
		live, err := registry.LookupReadOnly(dir, name)
		found := recipientRun{found: err == nil}
		if found.found {
			found.run, found.silent = live.Epoch(), role.Of(live.Role).Silent
		}
		runs[name] = found
		return found
	}
	awaited := inbox.Awaited(dir, session.Name, epoch, func(recipient, run string) inbox.RecipientRun {
		switch live := current(recipient); {
		case live.found && live.run == run:
			return inbox.RunLive
		case live.found:
			return inbox.RunReplaced
		}
		return inbox.RunEnded
	})
	model := awaitedModel{Session: session.Name, Recipients: []awaitedRecipient{}}
	index := map[string]int{}
	for _, message := range awaited {
		live := current(message.To)
		// A silent session's reads owe nothing, so a task to one is never
		// answered by a report and is not waited on.
		if !message.Gone() && live.silent {
			continue
		}
		view := awaitedView{
			ID: message.ID, Kind: inbox.KindOf(message.Message), CreatedAt: message.CreatedAt,
			State: message.Stage, Detail: message.Detail, Text: message.Text,
		}
		if message.AddendumTo != "" {
			// Under the task as it is now, an edit's replacement included.
			view.AddendumTo = inbox.CurrentTask(dir, message.To, message.AddendumTo)
		}
		switch message.Run {
		case inbox.RunEnded:
			view.Gone = "ended"
		case inbox.RunReplaced:
			view.Gone = "replaced"
		}
		at, ok := index[message.To]
		if !ok {
			at = len(model.Recipients)
			index[message.To] = at
			model.Recipients = append(model.Recipients, awaitedRecipient{Name: message.To})
		}
		model.Recipients[at].Messages = append(model.Recipients[at].Messages, view)
	}
	for index := range model.Recipients {
		model.Recipients[index].Messages = afterTheirTask(model.Recipients[index].Messages)
	}
	return printValue(ctx, model, func() []string { return awaitedLines(model.Recipients) })
}

func awaitedLines(recipients []awaitedRecipient) []string {
	var coming, lost int
	for _, recipient := range recipients {
		for _, message := range recipient.Messages {
			if message.coming() {
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
				strings.Repeat(" ", len(indent))+harness.Preview(message.Text),
			)
		}
	}
	return lines
}

// awaitedState says in a few words where a message stands. The whole detail
// is in --json; one line of it is enough to know what to do.
func awaitedState(recipient string, message awaitedView) string {
	switch message.Gone {
	case "ended":
		return "no report coming: " + recipient + " ended"
	case "replaced":
		return "no report coming: " + recipient + " was replaced by a new run"
	}
	detail := func(prefix string) string {
		if line := harness.Preview(message.Detail); line != "" {
			return prefix + ": " + line
		}
		return prefix
	}
	switch message.State {
	case inbox.StageUndelivered:
		return detail("not delivered yet")
	case inbox.StageHeld:
		return detail("held")
	case inbox.StageUnread:
		return "delivered, unread"
	case inbox.StagePending:
		return detail("pending")
	case inbox.StageStopped:
		// The stop's own text says who stopped the turn: the person at the
		// keyboard, or a main by name with rewake interrupt.
		return detail("stopped")
	case inbox.StageFailed:
		return detail("not delivered, no report coming")
	default:
		return "read, being worked on"
	}
}

func plural(count int, noun string) string {
	if count == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", count, noun)
}
