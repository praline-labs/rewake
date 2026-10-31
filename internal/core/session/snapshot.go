// Package session carries optional, epoch-scoped harness observations.
package session

import (
	"encoding/json"
	"time"
	"unicode/utf8"
)

// Snapshot describes the last reported primary state, not cumulative usage or
// the model executing an already captured inference step. Nil means unknown.
type Snapshot struct {
	Activity         *string           `json:"activity"`
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
	// Cache is what the harness reported of the prompt cache on the last
	// request; nil when it reports none.
	Cache *Cache `json:"cache,omitempty"`
	// CacheLife is how long the cache lives, from the harness's own status
	// line where it has one, or reckoned from the model's lifetime for its
	// tier where the harness names one; a second source beside Cache, which
	// comes from the responses' usage.
	CacheLife *CacheLife `json:"cacheLife,omitempty"`
	// ModelName is the harness's own display name of the model, else the
	// template's label (rewake list adds it); nil when neither is known.
	ModelName *string `json:"configuredModelName,omitempty"`
	Effort    *string `json:"configuredReasoningEffort"`
	// EffortSource is EffortFromLaunch when Effort is what the launch passed,
	// not what the harness was seen to run at; empty when observed.
	EffortSource string `json:"reasoningEffortSource,omitempty"`
	// Thinking says whether thinking is on, as the harness tells it: Claude
	// Code's session setting, pi's level for a model with reasoning; nil
	// where the harness does not tell.
	Thinking    *bool      `json:"thinking,omitempty"`
	SettingsAt  *time.Time `json:"settingsObservedAt"`
	Compactions *uint64    `json:"completedCompactions"`
	Compacting  *bool      `json:"compactionInProgress"`
	Coverage    string     `json:"compactionCoverage"`
	// Interruptions says whether a turn a person interrupts is heard:
	// observed, unobserved, or empty while nobody can tell yet.
	Interruptions string `json:"interruptions,omitempty"`
	// CompactionOutcomes are the ends of compactions a main asked for whose
	// command answered before the end: that main's wrapper sends each as a
	// letter.
	CompactionOutcomes []CompactionOutcome `json:"compactionOutcomes,omitempty"`
	// Dialog is what the harness said of a dialog the session waits at on its
	// screen that nobody else is asked about, nil when none: main is told of
	// it once, and it is cleared by the next tool result or step.
	Dialog *string `json:"dialog,omitempty"`
	// Waiting is what a dialog the session waits at on its screen asks while
	// a request carries it to main: WaitingPermission or WaitingQuestion,
	// empty when none.
	Waiting string `json:"waiting,omitempty"`
	// PublishedBoot is the boot clock's reading at PublishedAt (package
	// boottime), 0 from a build that did not write it. Freshness is judged by
	// it: the publisher and the reader are different processes, and the wall
	// clock can be stepped by seconds between the two.
	PublishedBoot int64 `json:"publishedBoot,omitempty"`
	// Cwd is the directory the session runs in now, where the harness tells
	// it: a worktree entry moves it. Nil where it does not tell, and the
	// session's record names where it started.
	Cwd *string `json:"cwd,omitempty"`
	// RateLimits are the account's rate-limit windows the harness reported
	// last; none where it reports none, as off a subscription.
	RateLimits []RateLimit `json:"rateLimits,omitempty"`
	// Verdict is Judge's verdict, set by a reader for those it shows the
	// snapshot to; Save never stores it, since its staleness is the
	// reader's.
	Verdict *Verdict `json:"verdict,omitempty"`
}

// Cache is the prompt-cache usage of the last request as the harness reports
// it. HitPercent is Read's share of every input token the request carried —
// the part served from the cache, the part written to it and the part neither
// read nor written — so a request that wrote a fresh cache shows a low figure,
// and one the cache served whole a high one. The raw counts ride along.
type Cache struct {
	HitPercent int   `json:"hitPercent"`
	Read       int64 `json:"readTokens"`
	Write      int64 `json:"writeTokens"`
	Input      int64 `json:"uncachedInputTokens"`
}

// RateLimit is one rate-limit window of the account as the harness reports
// it: its kind (five_hour, seven_day, or another the harness names), the
// whole percent used, and when it resets where the harness says.
type RateLimit struct {
	Kind     string     `json:"kind"`
	Percent  int        `json:"percent"`
	ResetsAt *time.Time `json:"resetsAt,omitempty"`
}

// CacheLife is the prompt cache's lifetime as the harness tells it: the tier
// (5m or 1h, another length where the model names one, empty when unknown),
// when it expires and whether the harness takes it for warm. The expiry is absolute, so a reader subtracts its own clock.
type CacheLife struct {
	TTL       string     `json:"ttl,omitempty"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	Warm      bool       `json:"warm"`
}

// Remaining is how long the cache has left at now, 0 when it has no expiry or
// has expired.
func (c *CacheLife) Remaining(now time.Time) time.Duration {
	if c == nil || c.ExpiresAt == nil {
		return 0
	}
	return max(0, c.ExpiresAt.Sub(now))
}

// NewCache builds a Cache from one request's counts; nil for a negative count
// or a request with no input, where no share exists.
func NewCache(read, write, input int64) *Cache {
	total := read + write + input
	if read < 0 || write < 0 || input < 0 || total <= 0 {
		return nil
	}
	return &Cache{HitPercent: int((read*100 + total/2) / total), Read: read, Write: write, Input: input}
}

// EffortFromLaunch marks an effort taken from the launch's flag: the person
// may have changed it since, and nothing observed says so yet.
const EffortFromLaunch = "launch"

// Values of Snapshot.Waiting.
const (
	WaitingPermission = "permission"
	WaitingQuestion   = "question"
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
// Detail are those of internal/core/control; the tokens are the counts around the
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
