// Package sessionstate carries optional, epoch-scoped harness observations.
package sessionstate

import (
	"encoding/json"
	"time"
	"unicode/utf8"

	"github.com/praline-labs/rewake/internal/channel"
)

// Snapshot describes the last reported primary state, not cumulative usage or
// the model executing an already captured inference step. Nil means unknown.
type Snapshot struct {
	Activity         *string           `json:"activity"`
	WaitingFor       []string          `json:"waitingFor"`
	ActivityAt       *time.Time        `json:"activityObservedAt"`
	ActivityFresh    bool              `json:"activityFresh"`
	CompactionEvents []CompactionEvent `json:"compactionEvents,omitempty"`
	Epoch            string            `json:"epoch"`
	Thread           string            `json:"primaryThread"`
	Selection        string            `json:"selection"`
	PublishedAt      *time.Time        `json:"publishedAt"`
	ObservedAt       *time.Time        `json:"observedAt"`
	Fresh            bool              `json:"fresh"`
	ContextFresh     bool              `json:"contextFresh"`
	SettingsFresh    bool              `json:"settingsFresh"`
	ContextUsed      *int64            `json:"contextUsedTokens"`
	ContextWindow    *int64            `json:"contextWindowTokens"`
	FilledPercent    *int              `json:"contextFilledPercent"`
	ContextAt        *time.Time        `json:"contextObservedAt"`
	Model            *string           `json:"configuredModel"`
	Effort           *string           `json:"configuredReasoningEffort"`
	SettingsAt       *time.Time        `json:"settingsObservedAt"`
	Compactions      *uint64           `json:"completedCompactions"`
	Compacting       *bool             `json:"compactionInProgress"`
	Coverage         string            `json:"compactionCoverage"`
	// Interruptions says whether a turn a person interrupts is heard:
	// observed, unobserved, or empty while nobody can tell yet.
	Interruptions string `json:"interruptions,omitempty"`
	// CompactionOutcomes are the ends of compactions a main asked for whose
	// command answered before the end: that main's wrapper sends each as a
	// letter.
	CompactionOutcomes []CompactionOutcome `json:"compactionOutcomes,omitempty"`
	// PublishedBoot is the boot clock's reading at PublishedAt (package
	// boottime), 0 from a build that did not write it. Freshness is judged by
	// it: the publisher and the reader are different processes, and the wall
	// clock can be stepped by seconds between the two.
	PublishedBoot int64 `json:"publishedBoot,omitempty"`
	// DeliveryHold says why deliveries into the session wait although a
	// conversation is selected; nil when nothing holds them.
	DeliveryHold *DeliveryHold `json:"deliveryHold,omitempty"`
	// Channel is how the run's mail travels (docs/mail-bridge-channel.md),
	// kept by its wrapper only; nil from a run without the mail tool's
	// harness or a build before it.
	Channel *channel.Record `json:"channel,omitempty"`
}

// DeliveryHold is why deliveries wait, and what the person can do about it
// (docs/archive-1.x/delivery-conversation.md#when-the-selected-conversation-is-not-the-launchs).
type DeliveryHold struct {
	Reason string `json:"reason"`
	// Expected is the conversation the launch asked to resume, empty when it
	// named none (--last or the picker) and the terminal resumed none.
	Expected string `json:"expectedConversation,omitempty"`
	Selected string `json:"selectedConversation,omitempty"`
	Detail   string `json:"detail"`
}

// Reasons of a DeliveryHold.
const (
	// HoldUnintended: the terminal selected a conversation other than the
	// one its launch asked to resume, and the person has not accepted it.
	HoldUnintended = "unintended-conversation"
	// HoldMailClosed: the conversation the launch asked for is selected, and
	// the record that opens the session's mail could not be written, so its
	// worker could not read what would be delivered.
	HoldMailClosed = "mail-closed"
)

// Values of Snapshot.Interruptions.
const (
	InterruptionsObserved   = "observed"
	InterruptionsUnobserved = "unobserved"
)

// Unknown also covers unsupported adapters; zero compactions would claim coverage.
func Unknown(epoch string) Snapshot {
	return Snapshot{Epoch: epoch, Selection: "unknown", Coverage: "unknown"}
}

// Stale keeps last observations identifiable without pretending they were measured now.
func (s *Snapshot) Stale() {
	s.Fresh, s.ContextFresh, s.SettingsFresh, s.ActivityFresh = false, false, false, false
}

// CompactionEvent is a bounded notification cue, not transcript content.
// Sequence is the wrapper's deduplicated completed counter at observation.
// RequestedBy and Request name the main whose `rewake compact` asked for this
// compaction and its request; empty for any other compaction. That main's own
// command reports it, so its wrapper sends no notice of it.
type CompactionEvent struct {
	Sequence    uint64    `json:"sequence"`
	ObservedAt  time.Time `json:"observedAt"`
	RequestedBy string    `json:"requestedBy,omitempty"`
	Request     string    `json:"request,omitempty"`
}

// CompactionOutcome is how a compaction a main asked for with `rewake compact`
// ended, kept for that main's wrapper to send as a letter: the command
// answered started or requested and has ended by then. Outcome, Reason and
// Detail are those of internal/control; the tokens are the counts around the
// compaction, never its summary.
type CompactionOutcome struct {
	Request      string    `json:"request"`
	RequestedBy  string    `json:"requestedBy"`
	Outcome      string    `json:"outcome"`
	Reason       string    `json:"reason,omitempty"`
	Detail       string    `json:"detail,omitempty"`
	TokensBefore *int64    `json:"tokensBefore,omitempty"`
	TokensAfter  *int64    `json:"tokensAfter,omitempty"`
	EndedAt      time.Time `json:"endedAt"`
}

// MaxCompactionOutcomes bounds the outcomes a snapshot keeps. main's wrapper
// reads them once a second, and one main compacts one session at a time.
const MaxCompactionOutcomes = 16

// The outcomes' share of a snapshot. A snapshot over maxSnapshotBytes is not
// saved at all, which would stop the whole telemetry, so the outcomes have a
// byte budget beside everything else at its longest — 64 compaction events
// with the longest names take about 12 KiB — and each detail, the host's or
// the server's error text, is cut, as is a reason, which is a fixed word from
// a served side that behaves.
const (
	maxOutcomeDetail = 300
	maxOutcomeReason = 64
	outcomesBudget   = 3 << 10
)

// KeepOutcome appends an outcome within the bounds, dropping the oldest; the
// newest is always kept.
func KeepOutcome(outcomes []CompactionOutcome, outcome CompactionOutcome) []CompactionOutcome {
	outcome.Detail = cut(outcome.Detail, maxOutcomeDetail)
	outcome.Reason = cut(outcome.Reason, maxOutcomeReason)
	outcomes = append(append([]CompactionOutcome{}, outcomes...), outcome)
	if len(outcomes) > MaxCompactionOutcomes {
		outcomes = outcomes[len(outcomes)-MaxCompactionOutcomes:]
	}
	for len(outcomes) > 1 {
		encoded, err := json.Marshal(outcomes)
		if err == nil && len(encoded) <= outcomesBudget {
			break
		}
		outcomes = outcomes[1:]
	}
	return outcomes
}

// cut shortens a text to at most limit bytes, on a rune boundary.
func cut(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	end := limit
	for end > 0 && !utf8.RuneStart(text[end]) {
		end--
	}
	return text[:end] + "…"
}
