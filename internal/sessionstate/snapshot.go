// Package sessionstate carries optional, epoch-scoped harness observations.
package sessionstate

import "time"

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
}

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
type CompactionEvent struct {
	Sequence   uint64    `json:"sequence"`
	ObservedAt time.Time `json:"observedAt"`
}
