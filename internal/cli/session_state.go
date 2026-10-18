package cli

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/praline-labs/rewake/internal/core/channel"
	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/core/session"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// Visibility is deliberately independent of collection and of the observed role.
// Name or role environment variables alone never establish a main caller.
func canSeeSessionState(dir string) bool {
	self, err := registry.LookupReadOnly(dir, os.Getenv(state.SessionEnv))
	epoch := os.Getenv(state.EpochEnv)
	return err == nil && epoch != "" && self.Epoch() == epoch && self.Role == role.Main.ID
}

func sessionSnapshot(dir, name, epoch string) *session.Snapshot {
	value := session.Load(dir, name, epoch)
	// A boot clock reading means nothing outside this machine's processes.
	value.CompactionEvents, value.CompactionOutcomes, value.PublishedBoot = nil, nil, 0
	current, err := registry.LookupReadOnly(dir, name)
	if err != nil || current.Epoch() != epoch {
		value.Stale()
	}
	return &value
}

type sessionView struct {
	Telemetry *session.Snapshot `json:"telemetry,omitempty"`
	// Stopped is the stop of the session's mailbox, when it has one.
	Stopped *stoppedView `json:"stopped,omitempty"`
	registry.Session
}

// stoppedView is what a session's mailbox is stopped on: the reports a
// decision can settle, and how many causes none settles.
type stoppedView struct {
	Decidable   []string `json:"decidable,omitempty"`
	Undecidable int      `json:"undecidable,omitempty"`
}

// stoppedOf reads what a mailbox is stopped on, as decide lists it, without
// a lock; nil when nothing is open.
func stoppedOf(dir, name string) *stoppedView {
	listing, err := mail.ListDecisions(dir, name)
	if err != nil {
		return nil
	}
	view := stoppedView{Undecidable: len(listing.Undecidable)}
	for _, subject := range listing.Subjects {
		if subject.Decided == nil {
			view.Decidable = append(view.Decidable, subject.Subject.Report)
		}
	}
	if len(view.Decidable) == 0 && view.Undecidable == 0 {
		return nil
	}
	return &view
}

type messageView struct {
	Telemetry *session.Snapshot `json:"telemetry,omitempty"`
	// ResentBy is the decision that sent this report again: the first copy
	// went unproven, so the same report may have been read already.
	ResentBy *awaitedDecision `json:"resentBy,omitempty"`
	mail.Message
}

func viewedMessages(dir string, messages []mail.Message) []messageView {
	visible := canSeeSessionState(dir)
	result := make([]messageView, 0, len(messages))
	for _, message := range messages {
		view := messageView{Message: message}
		// A boot clock reading means nothing to the reader.
		view.SenderState, view.CreatedBoot = nil, 0
		if message.AddendumTo != "" {
			// The task as it is now: an edit leaves the addendum naming the
			// letter it replaced. A copy on the way that cannot be read
			// leaves the task it was sent to, which is still true.
			if current, err := mail.CurrentTask(dir, message.To, message.AddendumTo); err == nil {
				view.AddendumTo = current
			}
		}
		if mail.IsReport(message) {
			if mark, ok := mail.ResentBy(dir, message.From, message.ID); ok {
				view.ResentBy = awaitedDecisionOf(&mark)
			}
		}
		if visible {
			view.Telemetry = sessionSnapshot(dir, message.From, message.FromEpoch)
			if message.Departure != nil && message.SenderState != nil && message.SenderState.Epoch == message.FromEpoch {
				frozen := *message.SenderState
				frozen.Stale()
				frozen.CompactionEvents, frozen.CompactionOutcomes = nil, nil
				frozen.PublishedBoot = 0
				view.Telemetry = &frozen
			}
		}
		result = append(result, view)
	}
	return result
}

func stateLine(name string, snapshot *session.Snapshot, settings bool) string {
	percent, window, count := "unknown", "unknown", "unknown"
	if snapshot.FilledPercent != nil {
		percent = strconv.Itoa(*snapshot.FilledPercent) + "%"
	}
	if snapshot.ContextWindow != nil {
		window = fmt.Sprintf("%.0fK", math.Round(float64(*snapshot.ContextWindow)/1000))
	}
	if snapshot.Compactions != nil {
		count = strconv.FormatUint(*snapshot.Compactions, 10)
	}
	line := fmt.Sprintf("%s: %s | context %s used / %s", name, activityText(snapshot), percent, window)
	if snapshot.ContextAt != nil && !snapshot.ContextFresh {
		line += " (stale)"
	}
	line += " | compactions " + count
	if snapshot.Coverage == "partial" {
		line += " (partial)"
	}

	if settings {
		model, effort := "unknown", "unknown"
		if snapshot.Model != nil {
			model = strconv.Quote(*snapshot.Model)
		}
		if snapshot.Effort != nil {
			effort = strconv.Quote(*snapshot.Effort)
		}
		line += " | model " + model + " | effort " + effort
		if snapshot.SettingsAt != nil && !snapshot.SettingsFresh {
			line += " (stale)"
		}
	}
	if snapshot.Channel != nil {
		line += " | " + channel.Label(snapshot.Channel)
	}
	if !snapshot.Fresh {
		line += " | state unavailable/stale"
	}
	return line
}

const sessionStateHelp = "A verified main also sees each launch's state: activity, configured model and effort, last reported context fill, usable window (in decimal K) and completed compactions, with unknown or stale values marked. Workers and plain shells get none of it, in JSON either."

func activityText(snapshot *session.Snapshot) string {
	label := "unknown"
	if snapshot.Activity != nil {
		switch *snapshot.Activity {
		case "idle":
			label = "idle"
		case "working":
			label = "working"
			switch {
			case snapshot.WaitingFor == nil:
				label += " (waiting state unknown)"
			case len(snapshot.WaitingFor) > 0:
				label = "waiting for " + strings.Join(snapshot.WaitingFor, " and ")
			}
		case "not_loaded":
			label = "not loaded"
		case "system_error":
			label = "system error"
		}
	}
	if snapshot.Compacting != nil && *snapshot.Compacting {
		label += "; compacting"
	}
	// Only the gap is named. A session that hears interruptions behaves as
	// expected and needs no word in a narrow cell, and a harness that always
	// hears them never sets the field. A column of its own would read empty or
	// "unknown" for every such row.
	if snapshot.Interruptions == session.InterruptionsUnobserved {
		label += "; interruptions unheard"
	}
	if snapshot.ActivityAt != nil && (!snapshot.ActivityFresh || !snapshot.Fresh) {
		label += " (stale)"
	}
	return label
}
