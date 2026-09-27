package cli

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/iiiokojiadbi/rewake/internal/grant"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/role"
	"github.com/iiiokojiadbi/rewake/internal/sessionstate"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// Visibility is deliberately independent of collection and of the observed role.
// Name or role environment variables alone never establish a main caller.
func canSeeSessionState(dir string) bool {
	self, err := registry.LookupReadOnly(dir, os.Getenv(state.SessionEnv))
	epoch := os.Getenv(state.EpochEnv)
	return err == nil && epoch != "" && self.Epoch() == epoch && self.Role == role.Main.ID
}

func sessionSnapshot(dir, name, epoch string) *sessionstate.Snapshot {
	value := sessionstate.Load(dir, name, epoch)
	// A boot clock reading means nothing outside this machine's processes.
	value.CompactionEvents, value.CompactionOutcomes, value.PublishedBoot = nil, nil, 0
	current, err := registry.LookupReadOnly(dir, name)
	if err != nil || current.Epoch() != epoch {
		value.Stale()
	}
	return &value
}

type sessionView struct {
	Telemetry *sessionstate.Snapshot `json:"telemetry,omitempty"`
	// Grants are the directories rewake added to this run's writable roots
	// for a task, live and ended (docs/grants.md).
	Grants []grant.Entry `json:"grants,omitempty"`
	registry.Session
}

type messageView struct {
	Telemetry *sessionstate.Snapshot `json:"telemetry,omitempty"`
	inbox.Message
}

func viewedMessages(dir string, messages []inbox.Message) []messageView {
	visible := canSeeSessionState(dir)
	result := make([]messageView, 0, len(messages))
	for _, message := range messages {
		view := messageView{Message: message}
		// A boot clock reading means nothing to the reader.
		view.SenderState, view.CreatedBoot = nil, 0
		if message.AddendumTo != "" {
			// The task as it is now: an edit leaves the addendum naming the
			// letter it replaced.
			view.AddendumTo = inbox.CurrentTask(dir, message.To, message.AddendumTo)
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

func stateLine(name string, snapshot *sessionstate.Snapshot, settings bool) string {
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
	if !snapshot.Fresh {
		line += " | state unavailable/stale"
	}
	return line
}

const sessionStateHelp = "Verified main callers also see epoch-scoped primary state: current primary activity, last reported context fill, usable window (rounded decimal K), and observed completed compactions. List includes configured model/effort. Main also receives compaction-complete and known-departure notices; idle alone never triggers a resend. Unknown/stale/partial values are explicit; workers and plain shells receive no telemetry, including in JSON."

func activityText(snapshot *sessionstate.Snapshot) string {
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
	// expected and needs no word in a narrow cell, and a Codex session never
	// sets the field: its gateway always hears them. A column of its own would
	// read empty or "unknown" for every such row.
	if snapshot.Interruptions == sessionstate.InterruptionsUnobserved {
		label += "; interruptions unheard"
	}
	if snapshot.ActivityAt != nil && (!snapshot.ActivityFresh || !snapshot.Fresh) {
		label += " (stale)"
	}
	return label
}
