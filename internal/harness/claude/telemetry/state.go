package telemetry

import (
	"time"

	"github.com/iiiokojiadbi/rewake/internal/sessionstate"
)

// Activity values, the same words the Codex side publishes.
const (
	activityIdle    = "idle"
	activityWorking = "working"
)

// state is what the events so far say. Every value starts unknown and stays
// unknown until an event says otherwise: a session nobody has heard from is
// not idle, and one whose hooks never ran has not had zero compactions.
type state struct {
	heard      bool
	observedAt time.Time
	// hooks is set by the first hook event. Only then are compactions counted
	// from the start of the session: hooks can be switched off (a managed
	// policy, --bare), and a count nobody could have made must not read zero.
	hooks bool

	thread   string
	threadAt int64

	activity   *string
	waiting    []string
	activityAt int64
	activityOn time.Time

	statusAt   int64
	model      *string
	effort     *string
	settingsOn time.Time

	context   *Context
	contextOn time.Time
	// measured is the plugin's session.measure, used only while the status
	// line has said nothing about the context: a policy may keep the tap out.
	measured   *Context
	measuredOn time.Time
	// limit is the auto-compact window the plugin last found configured,
	// and autocompact the flag the session was launched with; together they
	// decide the window the snapshot shows (window.go). Without the plugin
	// the limit is unknown and the model window stands.
	limit       *Limit
	limitAt     int64
	autocompact Autocompact
	// compactedAt is when the last compaction ended. The fill counted before
	// it no longer holds, and the harness reports none until the next
	// response, so until then the fill is unknown, not the old value.
	compactedAt int64

	// plugin is set by the first event of the function-hooks plugin, and
	// turns by the first hook that starts or ends a turn. A turn heard with
	// no plugin event means interruptions go unheard in this session.
	plugin bool
	turns  bool

	compacting bool
	count      uint64
	events     []sessionstate.CompactionEvent
	// asked is the compaction a main asked for with `rewake compact`, heard
	// as the host starts it and before any hook of it: the next PostCompact is
	// taken for that compaction, which holds unless another compaction's
	// PostCompact arrives later than a second after its end
	// (docs/remote-control.md).
	asked *askedCompaction
}

// askedCompaction is who asked for a compaction, and by which request.
type askedCompaction struct {
	by, request string
}

func (s *state) apply(event Event, now time.Time) {
	switch event.Kind {
	case StatusLine, SessionStart, UserPromptSubmit, Stop, StopFailure, PreCompact, PostCompact, Notification, SessionEnd:
	case PluginReady, TurnStart, TurnComplete, SessionMeasure, CompactAsked, CompactRefused:
		s.applyPlugin(event, now)
		return
	default:
		// A kind this collector does not know says nothing it can use.
		return
	}
	s.heard = true
	s.observedAt = now
	if event.Session != "" && event.At >= s.threadAt {
		// The conversation: /clear gives the session a new one, and the next
		// event of any kind already carries it.
		s.thread, s.threadAt = event.Session, event.At
	}
	if event.Kind == StatusLine {
		s.applyStatus(event, now)
		return
	}
	s.hooks = true
	if event.Model != "" {
		s.model = text(event.Model)
		s.settingsOn = now
	}
	if event.Effort != "" {
		s.effort = text(event.Effort)
		s.settingsOn = now
	}
	switch event.Kind {
	case SessionStart:
		// A compaction starts the session again with source "compact" in the
		// middle of a turn; only a fresh start means nothing is running.
		if event.Source != "compact" {
			s.setActivity(event.At, activityIdle, []string{}, now)
		}
	case UserPromptSubmit:
		s.turns = true
		s.setActivity(event.At, activityWorking, nil, now)
		s.compacting = false
	case Stop, StopFailure:
		s.turns = true
		s.setActivity(event.At, activityIdle, []string{}, now)
		// A compaction that failed runs PreCompact and nothing after it; the
		// end of the turn is the latest point it can still be running.
		s.compacting = false
	case Notification:
		switch event.Notice {
		case "permission_prompt":
			if s.activity != nil && *s.activity == activityWorking && event.At >= s.activityAt {
				s.waiting = []string{"approval"}
				s.activityOn = now
			}
		case "idle_prompt":
			s.setActivity(event.At, activityIdle, []string{}, now)
		}
	case PreCompact:
		s.compacting = true
	case PostCompact:
		s.compacting = false
		if event.At >= s.compactedAt {
			s.compactedAt = event.At
			s.context = unmeasured(s.context)
			s.measured = unmeasured(s.measured)
		}
		s.count++
		compaction := sessionstate.CompactionEvent{Sequence: s.count, ObservedAt: now}
		if s.asked != nil {
			compaction.RequestedBy, compaction.Request = s.asked.by, s.asked.request
			s.asked = nil
		}
		s.events = append(s.events, compaction)
		if len(s.events) > maxCompactionEvents {
			s.events = append([]sessionstate.CompactionEvent{}, s.events[len(s.events)-maxCompactionEvents:]...)
		}
	}
}

// applyPlugin takes what the plugin says. Its turn boundaries set activity
// the way UserPromptSubmit and Stop do, and unlike them it hears the end of a
// turn a person interrupted — which is what used to leave a session working
// until its next turn.
func (s *state) applyPlugin(event Event, now time.Time) {
	s.heard = true
	s.observedAt = now
	s.plugin = true
	if event.Limit != nil && event.At >= s.limitAt {
		limit := *event.Limit
		s.limit, s.limitAt = &limit, event.At
	}
	switch event.Kind {
	case TurnStart:
		s.setActivity(event.At, activityWorking, nil, now)
		s.compacting = false
	case TurnComplete:
		s.setActivity(event.At, activityIdle, []string{}, now)
		s.compacting = false
	case SessionMeasure:
		if event.Context != nil && event.At >= s.compactedAt {
			copied := *event.Context
			s.measured = &copied
			s.measuredOn = now
		}
	case CompactAsked:
		s.asked = &askedCompaction{by: event.By, request: event.Request}
	case CompactRefused:
		if s.asked != nil && s.asked.request == event.Request {
			s.asked = nil
		}
		// The host may refuse after PreCompact — "Not enough messages" comes
		// then — and no PostCompact follows: this is the compaction's end.
		s.compacting = false
	}
}

// applyStatus takes model, effort and context from the status line, which
// states all three together each time it runs.
func (s *state) applyStatus(event Event, now time.Time) {
	if event.At < s.statusAt {
		return
	}
	s.statusAt = event.At
	if event.Model != "" {
		s.model = text(event.Model)
		// Here an absent effort is an answer: the status line leaves the key
		// out for a model that takes none, so the previous model's effort
		// must not stay behind.
		s.effort = nil
		if event.Effort != "" {
			s.effort = text(event.Effort)
		}
		s.settingsOn = now
	}
	if event.Context != nil && event.At >= s.compactedAt {
		if s.waitingForApproval() && usedChanged(s.context, event.Context) {
			// The count moves only after a response, and a response after a
			// permission prompt means the prompt was answered.
			s.waiting = nil
		}
		copied := *event.Context
		s.context = &copied
		s.contextOn = now
	}
}

// unmeasured keeps what a compaction does not change — the window — and
// drops the fill.
func unmeasured(context *Context) *Context {
	if context == nil {
		return nil
	}
	return &Context{Window: context.Window}
}

func (s *state) setActivity(at int64, activity string, waiting []string, now time.Time) {
	// Hooks may run in the background, so an older event can arrive late.
	if at < s.activityAt {
		return
	}
	s.activityAt = at
	s.activity = text(activity)
	s.waiting = waiting
	s.activityOn = now
}

func (s *state) waitingForApproval() bool {
	return len(s.waiting) == 1 && s.waiting[0] == "approval"
}

func usedChanged(before, after *Context) bool {
	if before == nil || before.Used == nil || after.Used == nil {
		return after.Used != nil
	}
	return *before.Used != *after.Used
}

func (s *state) snapshot() sessionstate.Snapshot {
	snapshot := sessionstate.Unknown("")
	if !s.heard {
		return snapshot
	}
	snapshot.Selection = "ready"
	snapshot.Fresh = true
	snapshot.ObservedAt = moment(s.observedAt)
	snapshot.Thread = s.thread
	if s.activity != nil {
		snapshot.Activity = text(*s.activity)
		if s.waiting != nil {
			snapshot.WaitingFor = append([]string{}, s.waiting...)
		}
		snapshot.ActivityAt = moment(s.activityOn)
		snapshot.ActivityFresh = true
	}
	if s.model != nil || s.effort != nil {
		snapshot.Model = copyText(s.model)
		snapshot.Effort = copyText(s.effort)
		snapshot.SettingsAt = moment(s.settingsOn)
		snapshot.SettingsFresh = true
	}
	context, contextOn := s.context, s.contextOn
	if context == nil {
		context, contextOn = s.measured, s.measuredOn
	}
	context = capped(context, configured(s.limit, s.autocompact))
	if context != nil {
		snapshot.ContextUsed = copyInt64(context.Used)
		snapshot.ContextWindow = copyInt64(context.Window)
		if context.Percent != nil {
			percent := *context.Percent
			snapshot.FilledPercent = &percent
		}
		snapshot.ContextAt = moment(contextOn)
		snapshot.ContextFresh = true
	}
	switch {
	case s.plugin:
		snapshot.Interruptions = sessionstate.InterruptionsObserved
	case s.turns:
		snapshot.Interruptions = sessionstate.InterruptionsUnobserved
	}
	if s.hooks {
		count, compacting := s.count, s.compacting
		snapshot.Compactions = &count
		snapshot.Compacting = &compacting
		snapshot.Coverage = "observed"
		snapshot.CompactionEvents = append([]sessionstate.CompactionEvent{}, s.events...)
	}
	return snapshot
}

func text(value string) *string { return &value }

func copyText(value *string) *string {
	if value == nil {
		return nil
	}
	return text(*value)
}

func copyInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func moment(at time.Time) *time.Time { return &at }
