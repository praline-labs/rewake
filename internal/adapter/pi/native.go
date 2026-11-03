package pi

import (
	"bytes"
	"encoding/json"
	"math"
	"strconv"
	"time"

	"github.com/praline-labs/rewake/internal/core/session"
)

// What the extension read of the session comes as pi answered it (ctx's
// getContextUsage, model and thinkingLevel, the last assistant message's
// usage), and is named and converted here, so a change of what the Статус
// shows is the wrapper's alone. Each part is decoded on its own: a part whose
// shape a release changed is unknown, and the rest still counts.

type nativeState struct {
	ContextUsage  json.RawMessage `json:"contextUsage"`
	Model         json.RawMessage `json:"model"`
	ThinkingLevel json.RawMessage `json:"thinkingLevel"`
	LastAssistant json.RawMessage `json:"lastAssistant"`
}

type nativeModel struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Provider      string   `json:"provider"`
	Reasoning     bool     `json:"reasoning"`
	ContextWindow *float64 `json:"contextWindow"`
	PromptCache   *struct {
		Short *float64 `json:"short"`
		Long  *float64 `json:"long"`
	} `json:"promptCache"`
}

// assistantUsage is a message's usage as pi normalises it: input is the part
// neither read from nor written to the cache.
type assistantUsage struct {
	Usage struct {
		Input      float64 `json:"input"`
		CacheRead  float64 `json:"cacheRead"`
		CacheWrite float64 `json:"cacheWrite"`
	} `json:"usage"`
	Provider  string  `json:"provider"`
	Model     string  `json:"model"`
	Timestamp float64 `json:"timestamp"`
}

// cacheSeen is the last assistant message's cache usage that had input, as
// projected when first seen: its lifetime from that moment's model and the
// run's retention. It goes to the next image with the run (handedState).
type cacheSeen struct {
	// Thread is the conversation it is of: another starts with none.
	Thread string `json:"thread"`
	// Of is the reading it was last projected from: a sample repeating it is
	// not projected again with a model chosen since.
	Of    json.RawMessage `json:"of,omitempty"`
	Read  int64           `json:"read"`
	Write int64           `json:"write"`
	Input int64           `json:"input"`
	// TTL is the tier's lifetime in seconds and ExpiresAt its end in epoch
	// milliseconds, both 0 where the model names no lifetime for the tier.
	TTL       int64 `json:"ttl,omitempty"`
	ExpiresAt int64 `json:"expiresAt,omitempty"`
}

// projectLocked is a sample's native part as the session's sample takes it;
// it moves the run's cache observation on a reading not seen before.
func (r *run) projectLocked(thread string, raw json.RawMessage, now time.Time, observed *session.Sample) {
	var parts nativeState
	_ = json.Unmarshal(raw, &parts)
	var model *nativeModel
	if json.Unmarshal(parts.Model, &model) != nil || model != nil && model.ID == "" {
		model = nil
	}
	var level string
	_ = json.Unmarshal(parts.ThinkingLevel, &level)
	settings := &session.Settings{}
	if model != nil {
		settings.Model = &model.ID
		if model.Name != "" {
			settings.ModelName = &model.Name
		}
		// A model without reasoning has no thinking to be on or off.
		if model.Reasoning && level != "" {
			on := level != "off"
			settings.Thinking = &on
		}
	}
	if level != "" {
		settings.Effort = &level
	}
	observed.Settings = settings
	observed.Context = contextOf(parts.ContextUsage, model)
	seen := r.cacheLocked(thread, parts.LastAssistant, model)
	observed.Cache = session.NewCache(seen.Read, seen.Write, seen.Input)
	if observed.Cache != nil && seen.TTL > 0 && seen.ExpiresAt > 0 {
		at := time.UnixMilli(seen.ExpiresAt)
		observed.CacheLife = &session.CacheLife{TTL: tierName(seen.TTL), ExpiresAt: &at, Warm: now.Before(at)}
	}
}

// contextOf is the context's fill; the window is the model's where pi
// reports none.
func contextOf(raw json.RawMessage, model *nativeModel) *session.Context {
	var usage struct {
		Tokens        *float64 `json:"tokens"`
		ContextWindow *float64 `json:"contextWindow"`
		Percent       *float64 `json:"percent"`
	}
	_ = json.Unmarshal(raw, &usage)
	window := usage.ContextWindow
	if window == nil && model != nil {
		window = model.ContextWindow
	}
	context := &session.Context{Used: count(usage.Tokens), Window: count(window)}
	if p := usage.Percent; p != nil && *p >= 0 && !math.IsNaN(*p) && !math.IsInf(*p, 0) {
		percent := int(math.Round(math.Min(100, *p)))
		context.Percent = &percent
	}
	return context
}

func count(value *float64) *int64 {
	if value == nil || math.IsNaN(*value) || math.IsInf(*value, 0) {
		return nil
	}
	n := int64(math.Round(*value))
	return &n
}

// cacheLocked moves the run's cache observation by a sample's last assistant
// message. Whatever its stop reason: a response cut short or failed after its
// input usage arrived still sent that request. A placeholder with no input
// counts, or a reading already seen, leaves it.
func (r *run) cacheLocked(thread string, raw json.RawMessage, model *nativeModel) cacheSeen {
	seen := r.observed.cache
	if seen == nil || seen.Thread != thread {
		seen = &cacheSeen{Thread: thread}
		r.observed.cache = seen
	}
	var compact bytes.Buffer
	if json.Compact(&compact, raw) != nil || compact.String() == "null" || bytes.Equal(compact.Bytes(), seen.Of) {
		return *seen
	}
	seen.Of = compact.Bytes()
	var message assistantUsage
	if json.Unmarshal(raw, &message) != nil {
		return *seen
	}
	input, read, write := message.Usage.Input, message.Usage.CacheRead, message.Usage.CacheWrite
	if !(input >= 0 && read >= 0 && write >= 0) || input+read+write <= 0 {
		return *seen
	}
	seen.Read, seen.Write, seen.Input = int64(read), int64(write), int64(input)
	seen.TTL, seen.ExpiresAt = lifeOf(message, model, r.retention)
	return *seen
}

// lifeOf is how long the cache the message's request read or wrote lives,
// in seconds, and when it ends, in epoch milliseconds. pi's own requests name
// no retention, so the tier is long when PI_CACHE_RETENTION=long and short
// otherwise, as pi's cache warmer reads it; the model's promptCache gives the
// tier's best-effort lifetime, unset where the provider's is not known. The
// provider counts it from the request, the message's timestamp, not from the
// end of the answer. A model other than the one that answered says nothing of
// this cache.
func lifeOf(message assistantUsage, model *nativeModel, retention string) (ttl, expiresAt int64) {
	if model == nil || model.PromptCache == nil || model.Provider != message.Provider || model.ID != message.Model {
		return 0, 0
	}
	tier := model.PromptCache.Short
	if retention == "long" {
		tier = model.PromptCache.Long
	}
	if tier == nil || !(*tier > 0) || !(message.Timestamp > 0) {
		return 0, 0
	}
	ttl = int64(*tier)
	return ttl, int64(message.Timestamp) + ttl*1000
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
