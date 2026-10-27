package pi

import (
	"context"
	"math"
	"strconv"
	"time"

	"github.com/praline-labs/rewake/internal/core/session"
)

// A sample's sequence orders event pushes and answers to polls on one
// connection. A late poll answer cannot replace a newer pushed observation.
type sample struct {
	Sequence  uint64  `json:"sequence"`
	Activity  string  `json:"activity"`
	Model     *string `json:"model"`
	ModelName *string `json:"modelName"`
	Effort    *string `json:"effort"`
	// Thinking is whether the model's thinking level is on; absent from an
	// extension that predates it and for a model without reasoning.
	Thinking   *bool    `json:"thinking,omitempty"`
	Tokens     *int64   `json:"tokens"`
	Window     *int64   `json:"window"`
	Percent    *float64 `json:"percent"`
	Compacting bool     `json:"compacting"`
	// Cache is the last assistant message's usage; absent from an extension
	// that predates it and before the first response.
	Cache *cacheUsage `json:"cache,omitempty"`
}

type cacheUsage struct {
	Read  int64 `json:"read"`
	Write int64 `json:"write"`
	Input int64 `json:"input"`
	// TTL is the lifetime in seconds of the tier the request was sent with,
	// and ExpiresAt its end in epoch milliseconds: the extension's reckoning
	// from the model's promptCache, absent where the model names none.
	TTL       *int64 `json:"ttl,omitempty"`
	ExpiresAt *int64 `json:"expiresAt,omitempty"`
}

// life is the cache's lifetime as the snapshot carries it; nil where the
// extension had none to tell. pi gives no expiry and no warm flag of its own:
// warm is whether the reckoned expiry is still ahead at now.
func (c *cacheUsage) life(now time.Time) *session.CacheLife {
	if c.TTL == nil || c.ExpiresAt == nil || *c.TTL <= 0 || *c.ExpiresAt <= 0 {
		return nil
	}
	at := time.UnixMilli(*c.ExpiresAt)
	return &session.CacheLife{TTL: tierName(*c.TTL), ExpiresAt: &at, Warm: now.Before(at)}
}

// tierName writes a lifetime as the status line of Claude Code names its
// tiers: 5m, 1h, else seconds.
func tierName(seconds int64) string {
	switch {
	case seconds%3600 == 0:
		return strconv.FormatInt(seconds/3600, 10) + "h"
	case seconds%60 == 0:
		return strconv.FormatInt(seconds/60, 10) + "m"
	}
	return strconv.FormatInt(seconds, 10) + "s"
}

// observations is what the run keeps of its samples beside the session's
// observer: the connection the last came on and its sequence, by which a
// late answer to a poll cannot replace a newer pushed sample.
type observations struct {
	view     *session.Observer
	link     *link
	sequence uint64
}

// sampleExpiry is how long a sample stays fresh: the run polls every second.
const sampleExpiry = 3 * time.Second

func (r *run) observe(l *link, thread string, value *sample) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.observeLocked(l, thread, value)
}

func (r *run) observeLocked(l *link, thread string, value *sample) bool {
	if value == nil || value.Sequence == 0 || l != r.current || thread != r.thread {
		return false
	}
	if r.observed.link == l && value.Sequence <= r.observed.sequence {
		return false
	}
	if value.Activity != "working" && value.Activity != "idle" {
		return false
	}
	compacting := value.Compacting
	observed := session.Sample{
		Thread: thread, Activity: value.Activity, Compacting: &compacting,
		Context:  &session.Context{Used: value.Tokens, Window: value.Window},
		Settings: &session.Settings{Model: value.Model, ModelName: value.ModelName, Effort: value.Effort, Thinking: value.Thinking},
	}
	if value.Percent != nil && *value.Percent >= 0 && !math.IsNaN(*value.Percent) && !math.IsInf(*value.Percent, 0) {
		percent := int(math.Round(math.Min(100, *value.Percent)))
		observed.Context.Percent = &percent
	}
	if value.Cache != nil {
		observed.Cache = session.NewCache(value.Cache.Read, value.Cache.Write, value.Cache.Input)
		observed.CacheLife = value.Cache.life(time.Now())
	}
	r.observed.view.Sample(observed)
	r.observed.link, r.observed.sequence = l, value.Sequence
	return true
}

// SessionState only copies cached observations. The host publishes four times
// a second; a stalled native runtime must not stall that publisher.
func (r *run) SessionState() session.Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	live := r.current != nil && !r.current.gone() && r.current == r.observed.link
	return r.observed.view.Snapshot(session.Reading{Live: live, Counted: true})
}

func (r *run) pollState(l *link, thread string) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-l.closed:
			return
		case <-r.ctx.Done():
			return
		case <-ticker.C:
		}
		if !r.on(l) {
			return
		}
		ctx, cancel := context.WithTimeout(r.ctx, time.Second)
		answer, err := l.ask(ctx, frame{Op: "state", Thread: thread})
		cancel()
		if err == nil && answer.OK {
			r.observe(l, thread, answer.State)
		}
	}
}
