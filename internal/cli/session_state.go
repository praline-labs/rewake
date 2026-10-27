package cli

import (
	"fmt"
	"math"
	"os"
	"strconv"

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
	return judged(value)
}

// judged is the snapshot as a reader is shown it, with the verdict on its
// activity judged now that what is stale is marked.
func judged(snapshot session.Snapshot) *session.Snapshot {
	verdict := snapshot.Judge()
	snapshot.Verdict = &verdict
	return &snapshot
}

type sessionView struct {
	Telemetry *session.Snapshot `json:"telemetry,omitempty"`
	// Type is the worker type of the space the session is the worker of;
	// empty for main, a worker without a type and a session no space hosts.
	Type string `json:"type,omitempty"`
	registry.Session
}

type messageView struct {
	Telemetry *session.Snapshot `json:"telemetry,omitempty"`
	// EarlierRun says the letter was written for an earlier run of the
	// reader's name, which ended without reading it.
	EarlierRun bool `json:"earlierRun,omitempty"`
	// BriefView is the brief the letter gives, as it reads now; under --json
	// it stands where the letter's record has the brief's id.
	BriefView *letterBrief `json:"brief,omitempty"`
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
		// A letter from a person's shell has no run behind it, so no state.
		if visible && message.FromEpoch != "" {
			view.Telemetry = sessionSnapshot(dir, message.From, message.FromEpoch)
			if message.Departure != nil && message.SenderState != nil && message.SenderState.Epoch == message.FromEpoch {
				frozen := *message.SenderState
				frozen.Stale()
				frozen.CompactionEvents, frozen.CompactionOutcomes = nil, nil
				frozen.PublishedBoot = 0
				view.Telemetry = judged(frozen)
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
	line := fmt.Sprintf("%s: %s | context %s used / %s", name, snapshot.Judge().Label, percent, window)
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
			if snapshot.EffortSource != "" {
				effort += " (" + snapshot.EffortSource + ")"
			}
		}
		line += " | model " + model + " | effort " + effort
		if snapshot.SettingsAt != nil && !snapshot.SettingsFresh {
			line += " (stale)"
		}
	}
	if !snapshot.Fresh {
		line += " | state unavailable/stale"
	}
	return line
}

const sessionStateHelp = "A verified main also sees each launch's state: activity, model and effort as the session last reported them — an effort marked (launch) is the launch's own until the session reports one, and unknown when the launch passed none —, last reported context fill, usable window (in decimal K) and completed compactions, with unknown or stale values marked. Workers and plain shells get none of it, in JSON either."
