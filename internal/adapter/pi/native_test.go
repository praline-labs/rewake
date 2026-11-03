package pi

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/session"
)

// nativeOf is a state's native part as the extension sends it.
func nativeOf(native map[string]any) json.RawMessage {
	raw, err := json.Marshal(native)
	if err != nil {
		panic(err)
	}
	return raw
}

var cached = map[string]any{"id": "m", "name": "Model", "provider": "p", "reasoning": true, "contextWindow": 200000, "promptCache": map[string]any{"short": 300, "long": 3600}}

// answered is the last assistant message as the extension sends it.
func answered(input, read, write int, at int64) map[string]any {
	return map[string]any{"usage": map[string]any{"input": input, "output": 9, "cacheRead": read, "cacheWrite": write}, "provider": "p", "model": "m", "timestamp": at}
}

// stateOf posts a state of sequence on l whose last assistant message is
// last under the model selected, and answers what the run's snapshot shows.
func stateOf(t *testing.T, rig *rig, l *link, sequence uint64, selected, last any) session.Snapshot {
	t.Helper()
	askOK(t, l, frame{Op: "state", Thread: "conversation", State: &sample{Sequence: sequence, Idle: true, Native: nativeOf(map[string]any{
		"model": selected, "thinkingLevel": "high", "contextUsage": nil, "lastAssistant": last,
	})}})
	return rig.run.SessionState()
}

// A sample names the model, its own name and whether it thinks; the window
// is the model's where pi reports none.
func TestAPiSampleIsProjectedHere(t *testing.T) {
	rig := newRig(t, nil)
	l := rig.client(t, "conversation", nil)
	got := stateOf(t, rig, l, 2, cached, nil)
	if got.Model == nil || *got.Model != "m" || got.ModelName == nil || *got.ModelName != "Model" || got.Thinking == nil || !*got.Thinking || got.Effort == nil || *got.Effort != "high" {
		t.Fatalf("the settings: %+v", got)
	}
	if got.ContextWindow == nil || *got.ContextWindow != 200000 || got.ContextUsed != nil || got.Cache != nil {
		t.Fatalf("the window: %+v", got)
	}
	plain := map[string]any{"id": "plain", "provider": "p", "reasoning": false}
	if got := stateOf(t, rig, l, 3, plain, nil); got.Thinking != nil || got.ModelName != nil {
		t.Fatalf("a model without reasoning: %+v", got)
	}
	if got := stateOf(t, rig, l, 4, "a model of another shape", nil); got.Model != nil || got.Effort == nil {
		t.Fatalf("a model of another shape: %+v", got)
	}
}

// The cache is the last assistant message's usage that had input, a cut or
// failed one's too; its lifetime is the tier's from the request, the long
// tier by PI_CACHE_RETENTION, and a model other than the one that answered
// says nothing of it. A placeholder, or the same reading again after another
// model was chosen, leaves it; another conversation starts with none.
func TestTheCacheIsTheLastResponseWithInputAndLivesItsTier(t *testing.T) {
	rig := newRig(t, func(r *run) { r.retention = "long" })
	l := rig.client(t, "conversation", nil)
	at := time.Now().Add(-time.Minute).UnixMilli()
	got := stateOf(t, rig, l, 2, cached, answered(50, 900, 50, at))
	if c := got.Cache; c == nil || c.HitPercent != 90 || c.Read != 900 || c.Write != 50 || c.Input != 50 {
		t.Fatalf("the cache: %+v", c)
	}
	if life := got.CacheLife; life == nil || life.TTL != "1h" || life.ExpiresAt == nil || life.ExpiresAt.UnixMilli() != at+3_600_000 || !life.Warm {
		t.Fatalf("its life: %+v", life)
	}
	other := map[string]any{"id": "other", "provider": "p", "promptCache": map[string]any{"short": 60, "long": 60}}
	if got := stateOf(t, rig, l, 3, other, answered(50, 900, 50, at)); got.CacheLife == nil || got.CacheLife.TTL != "1h" {
		t.Fatalf("the same reading under a model chosen since: %+v", got.CacheLife)
	}
	if got := stateOf(t, rig, l, 4, cached, answered(0, 0, 0, at+1)); got.Cache == nil || got.Cache.Read != 900 {
		t.Fatalf("a placeholder: %+v", got.Cache)
	}
	got = stateOf(t, rig, l, 5, other, answered(50, 0, 950, at+2))
	if got.Cache == nil || got.Cache.Write != 950 || got.CacheLife != nil {
		t.Fatalf("another model's answer has no life of this one: %+v %+v", got.Cache, got.CacheLife)
	}
	if got := stateOf(t, rig, l, 6, map[string]any{"id": "m", "provider": "p", "promptCache": map[string]any{"short": 300}}, answered(50, 0, 950, at+3)); got.CacheLife != nil {
		t.Fatalf("a tier the model names no lifetime for: %+v", got.CacheLife)
	}
	if got := stateOf(t, rig, l, 7, cached, nil); got.Cache == nil {
		t.Fatal("a sample without a message cleared the cache")
	}
	fresh := rig.client(t, "new", nil)
	askOK(t, fresh, frame{Op: "state", Thread: "new", State: &sample{Sequence: 2, Idle: true, Native: nativeOf(map[string]any{"model": cached, "lastAssistant": nil})}})
	if got := rig.run.SessionState(); got.Thread != "new" || got.Cache != nil {
		t.Fatalf("another conversation: %+v", got.Cache)
	}
}

// The cache goes with the run to the next image: the extension sends its
// last message's, which by then may be a placeholder's.
func TestThePiCacheIsHandedToTheNextImage(t *testing.T) {
	rig := newRig(t, nil)
	l := rig.client(t, "conversation", nil)
	stateOf(t, rig, l, 2, cached, answered(50, 900, 50, time.Now().UnixMilli()))
	stateOf(t, rig, l, 3, cached, answered(0, 0, 0, time.Now().UnixMilli()))
	before := rig.run
	handed, err := before.HandOver()
	if err != nil {
		t.Fatal(err)
	}
	aside := before.socket + ".aside"
	if err := os.Rename(before.socket, aside); err != nil {
		t.Fatal(err)
	}
	before.Close()
	if err := os.Rename(aside, before.socket); err != nil {
		t.Fatal(err)
	}
	after := newRun(before.file, before.socket, "run-epoch")
	after.ReportTurns(adapter.TurnHandler{Capture: func() *mail.ReadBoundary { return &mail.ReadBoundary{} }, Publish: func(context.Context, adapter.TurnEnd) error { return nil }})
	if err := after.Adopt(t.Context(), nil, handed); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(after.Close)
	askOK(t, l, frame{Op: "state", Thread: "conversation", State: &sample{Sequence: 4, Idle: true, Native: nativeOf(map[string]any{"model": cached, "lastAssistant": answered(0, 0, 0, 1)})}})
	if got := after.SessionState().Cache; got == nil || got.Read != 900 || got.Write != 50 {
		t.Fatalf("the cache in the next image: %+v", got)
	}
}
