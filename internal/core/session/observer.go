package session

import (
	"sync"
	"time"
)

// maxCompactionEvents bounds the cues a snapshot carries.
const maxCompactionEvents = 64

// Observer is what an adapter knows of one run of its session: the adapter
// hands it what the harness reported, as typed observations, and it alone
// merges them into the snapshot the wrapper publishes — which source wins,
// what is fresh, what has expired. An adapter reports facts; it builds no
// snapshot.
type Observer struct {
	mu     sync.Mutex
	config ObserverConfig

	// sample is the last primary state, nil before the first.
	sample *Snapshot
	// thread is the conversation as last told, by a sample or on its own.
	thread string

	// The standing facts: told by a second source between samples — Claude
	// Code's status line —, kept across samples and reloads, and shown over a
	// sample's own value of the same thing.
	effort    string
	efforts   map[string]string
	modelName string
	thinking  *bool
	life      *CacheLife

	// source is what compactions are counted from now, counting once one
	// was, and last its count or its last sequence.
	source      any
	counting    bool
	last        uint64
	compactions uint64
	events      []CompactionEvent
	outcomes    []CompactionOutcome
	partial     bool
}

// ObserverConfig is what the run was started with that observations stand
// beside.
type ObserverConfig struct {
	Epoch string
	// Effort is the effort the launch passed, shown, marked
	// EffortFromLaunch, until one is observed.
	Effort string
	// Window is the context window the launch limited the conversation to, 0
	// for none: it bounds the model's, and the filled percent is of it.
	Window int64
	// Expiry is how long a sample stays fresh without the next; 0 keeps it
	// fresh while the adapter reads it live.
	Expiry time.Duration
}

// Sample is one report of the session's primary state: what it carries
// replaces the last sample whole, and a part it leaves nil was not observed.
type Sample struct {
	// Thread is the conversation the sample is of.
	Thread string
	// Activity is working or idle, empty when the sample does not tell it.
	Activity   string
	Context    *Context
	Settings   *Settings
	Compacting *bool
	Cache      *Cache
	CacheLife  *CacheLife
	// Dialog is what the harness said of a dialog the session waits at that
	// nobody else is asked about, empty for none.
	Dialog string
	// Waiting is WaitingPermission or WaitingQuestion while a request
	// carries a dialog to main, empty for none.
	Waiting string
}

// Context is the context's fill as a sample tells it; a negative count, a
// window not above 0 or a percent outside 0–100 is unknown.
type Context struct {
	Used    *int64
	Window  *int64
	Percent *int
}

// Settings is the session's configuration as a sample tells it.
type Settings struct {
	Model     *string
	ModelName *string
	// Effort is the effort the sample was taken at; a source that repeats
	// an effort whether or not it changed reports it by Observer.Effort.
	Effort   *string
	Thinking *bool
}

// Reading is what the adapter knows at a read that its observations do not.
type Reading struct {
	// Live says the source the last sample came from is still the one
	// reporting; a sample from one that is gone is stale.
	Live bool
	// Counted says the run's compactions are counted: the snapshot carries
	// their cues and outcomes, and their count once a sample came.
	Counted bool
}

// NewObserver is an observer of one run, with nothing observed yet.
func NewObserver(config ObserverConfig) *Observer {
	return &Observer{config: config, efforts: map[string]string{}}
}

// Sample takes a report of the primary state, observed now.
func (o *Observer) Sample(sample Sample) {
	now := time.Now()
	snapshot := Unknown(o.config.Epoch)
	snapshot.Thread, snapshot.Selection = sample.Thread, "ready"
	snapshot.ObservedAt, snapshot.Fresh = &now, true
	snapshot.Interruptions = InterruptionsObserved
	if sample.Activity != "" {
		activity := sample.Activity
		snapshot.Activity, snapshot.ActivityAt, snapshot.ActivityFresh = &activity, &now, true
	}
	if c := sample.Context; c != nil {
		snapshot.ContextAt, snapshot.ContextFresh = &now, true
		snapshot.ContextUsed, snapshot.ContextWindow = known(c.Used, 0), known(c.Window, 1)
		// The model's window and percent are the model's; the window the
		// launch limited the conversation to is the limit the person means.
		if limit := o.config.Window; limit > 0 && (snapshot.ContextWindow == nil || limit < *snapshot.ContextWindow) {
			snapshot.ContextWindow = &limit
			if snapshot.ContextUsed != nil {
				percent := min(100, int((*snapshot.ContextUsed*100+limit/2)/limit))
				snapshot.FilledPercent = &percent
			}
		} else if snapshot.ContextUsed != nil && snapshot.ContextWindow != nil && c.Percent != nil && *c.Percent >= 0 && *c.Percent <= 100 {
			percent := *c.Percent
			snapshot.FilledPercent = &percent
		}
	}
	if s := sample.Settings; s != nil {
		snapshot.SettingsAt, snapshot.SettingsFresh = &now, true
		snapshot.Model, snapshot.Effort, snapshot.Thinking = s.Model, s.Effort, s.Thinking
		if s.ModelName != nil && *s.ModelName != "" {
			snapshot.ModelName = s.ModelName
		}
	}
	snapshot.Compacting, snapshot.Cache, snapshot.CacheLife = sample.Compacting, sample.Cache, sample.CacheLife
	if sample.Dialog != "" {
		dialog := sample.Dialog
		snapshot.Dialog = &dialog
	}
	if sample.Waiting == WaitingPermission || sample.Waiting == WaitingQuestion {
		snapshot.Waiting = sample.Waiting
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.sample, o.thread = &snapshot, sample.Thread
}

// known is a count read as known when it is at least floor.
func known(value *int64, floor int64) *int64 {
	if value == nil || *value < floor {
		return nil
	}
	count := *value
	return &count
}

// Thread takes the conversation the session moved to, told apart from a
// sample.
func (o *Observer) Thread(thread string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.thread = thread
}

// Effort takes an effort from a source that repeats it — a request asks for
// its effort on every heartbeat, a status line its setting on every run —, so
// it counts only when it differs from that source's last: the effort shown is
// the one whose source changed last.
func (o *Observer) Effort(source, effort string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if effort == "" || effort == o.efforts[source] {
		return
	}
	o.efforts[source], o.effort = effort, effort
}

// ModelName takes the harness's own name of the model; an empty one changes
// nothing.
func (o *Observer) ModelName(name string) {
	if name == "" {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.modelName = name
}

// Thinking takes whether thinking is on.
func (o *Observer) Thinking(on bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.thinking = &on
}

// CacheLife takes the prompt cache's lifetime.
func (o *Observer) CacheLife(life CacheLife) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.life = &life
}

// Compacted takes a source's own running count of the compactions that
// ended — Claude Code's mod counts them per load —, the last asked for by the
// main from in its request (both empty for any other). Its increase over the
// source's last count is added to the run's, with one cue; a source that
// follows another is counted from zero and leaves the count partial, since
// what ended between the two was not seen. A source is any comparable value
// naming it, a load's id or a connection.
func (o *Observer) Compacted(source any, total uint64, from, request string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.from(source)
	if total > o.last {
		o.counted(total-o.last, from, request)
		o.last = total
	}
}

// Compaction takes one compaction's end as a source numbers it: a sequence
// not above the source's last is one already counted, and 0 is an end the
// source does not number, counted as it comes.
func (o *Observer) Compaction(source any, sequence uint64, from, request string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.from(source)
	if sequence != 0 {
		if sequence <= o.last {
			return
		}
		o.last = sequence
	}
	o.counted(1, from, request)
}

// from makes source the one compactions are counted from. Called under mu.
func (o *Observer) from(source any) {
	if o.counting && o.source == source {
		return
	}
	if o.counting {
		o.partial = true
	}
	o.source, o.last, o.counting = source, 0, true
}

// counted adds n ended compactions with one cue. Called under mu.
func (o *Observer) counted(n uint64, from, request string) {
	o.compactions += n
	o.events = append(o.events, CompactionEvent{Sequence: o.compactions, ObservedAt: time.Now(), RequestedBy: from, Request: request})
	if len(o.events) > maxCompactionEvents {
		o.events = o.events[len(o.events)-maxCompactionEvents:]
	}
}

// Outcome keeps how a compaction a main asked for ended, once per request:
// the end can come both as the command's answer and as the harness's event.
func (o *Observer) Outcome(outcome CompactionOutcome) {
	if outcome.Request == "" || outcome.RequestedBy == "" {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, known := range o.outcomes {
		if known.Request == outcome.Request {
			return
		}
	}
	o.outcomes = KeepOutcome(o.outcomes, outcome)
}

// Partial says compactions may have gone uncounted though no source followed
// yet: the one counted from dropped, and what ends before the next is not
// seen.
func (o *Observer) Partial() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.partial = true
}

// Snapshot is what is observed, merged as of now: the last sample, stale when
// its source is not live or it is older than the expiry; the standing facts
// over the sample's; the effort that changed last, else the sample's, else
// the launch's.
func (o *Observer) Snapshot(reading Reading) Snapshot {
	o.mu.Lock()
	defer o.mu.Unlock()
	snapshot := Unknown(o.config.Epoch)
	if o.sample != nil {
		snapshot = *o.sample
		snapshot.Thread = o.thread
		if !reading.Live || o.config.Expiry > 0 && time.Since(*snapshot.ObservedAt) > o.config.Expiry {
			snapshot.Stale()
		}
		if reading.Counted {
			count := o.compactions
			snapshot.Compactions, snapshot.Coverage = &count, "observed"
			if o.partial {
				snapshot.Coverage = "partial"
			}
		}
	}
	// The cues and outcomes go out before a sample too: main's wrapper
	// sends each as a letter, and the count is what waits for a sample.
	if reading.Counted {
		snapshot.CompactionEvents = append([]CompactionEvent{}, o.events...)
		snapshot.CompactionOutcomes = append([]CompactionOutcome{}, o.outcomes...)
	}
	if o.life != nil {
		life := *o.life
		snapshot.CacheLife = &life
	}
	switch {
	case o.effort != "":
		effort := o.effort
		snapshot.Effort, snapshot.EffortSource = &effort, ""
	case snapshot.Effort != nil:
	case o.config.Effort != "":
		effort := o.config.Effort
		snapshot.Effort, snapshot.EffortSource = &effort, EffortFromLaunch
	}
	if o.modelName != "" {
		name := o.modelName
		snapshot.ModelName = &name
	}
	if o.thinking != nil {
		on := *o.thinking
		snapshot.Thinking = &on
	}
	return snapshot
}
