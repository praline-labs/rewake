package modhost

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/session"
)

// step is a sample whose native part is a main-loop step's usage and effort
// as Claude Code's turn.step answers them.
func step(sequence int, usage any, effort any) StateRequest {
	sample := withNative(testState(), map[string]any{
		"model": "test-model",
		"usage": map[string]any{"context": map[string]any{"tokens": 25000, "window": 100000, "percent": 25}},
		"step":  map[string]any{"usage": usage, "effort": effort},
	})
	sample.Sequence = sequence
	return sample
}

func usage(read, write, input int) map[string]any {
	return map[string]any{"cache_read_input_tokens": read, "cache_creation_input_tokens": write, "input_tokens": input, "output_tokens": 7, "model": "test-model"}
}

func cacheIs(t *testing.T, server *Server, want *session.Cache) {
	t.Helper()
	got := server.SessionState().Cache
	if (got == nil) != (want == nil) || got != nil && *got != *want {
		t.Fatalf("the cache: %+v, want %+v", got, want)
	}
}

// The last main-loop step's usage is the cache the Статус shows; a step
// without usage, or a placeholder's with no input, leaves the last response's
// standing. Its effort is the request's, a level or an integer budget.
func TestTheStepsUsageIsTheCacheAndAPlaceholderLeavesIt(t *testing.T) {
	server, socket := serving(t)
	postValue(t, socket, PathHello, HelloRequest{Protocol: Protocol, LoadID: "load", SessionID: "conversation-1", HarnessVersion: "test"})
	postValue(t, socket, PathModState, step(1, nil, nil))
	cacheIs(t, server, nil)
	postValue(t, socket, PathModState, step(2, usage(900, 50, 50), "high"))
	served := &session.Cache{HitPercent: 90, Read: 900, Write: 50, Input: 50}
	cacheIs(t, server, served)
	if got := server.SessionState().Effort; got == nil || *got != "high" {
		t.Fatalf("the effort: %v", got)
	}
	postValue(t, socket, PathModState, step(3, nil, 32000))
	cacheIs(t, server, served)
	if got := server.SessionState().Effort; got == nil || *got != "32000" {
		t.Fatalf("a budget: %v", got)
	}
	postValue(t, socket, PathModState, step(4, usage(0, 0, 0), nil))
	cacheIs(t, server, served)
	postValue(t, socket, PathModState, step(5, usage(0, 1000, 0), nil))
	cacheIs(t, server, &session.Cache{HitPercent: 0, Write: 1000})
}

// A part whose shape a release changed is unknown, and the rest still
// counts; a native part that is no object is no reading at all.
func TestAPartOfAnotherShapeIsUnknownAndTheRestCounts(t *testing.T) {
	server, socket := serving(t)
	postValue(t, socket, PathHello, HelloRequest{Protocol: Protocol, LoadID: "load", SessionID: "conversation-1", HarnessVersion: "test"})
	sample := withNative(testState(), map[string]any{
		"model": "test-model", "cwd": "/work",
		"usage": map[string]any{"context": "full", "rateLimits": map[string]any{}},
		"step":  map[string]any{"usage": "none", "effort": []any{}},
	})
	postValue(t, socket, PathModState, sample)
	got := server.SessionState()
	if got.Model == nil || *got.Model != "test-model" || got.Cwd == nil || *got.Cwd != "/work" {
		t.Fatalf("the parts that still read: %+v", got)
	}
	if got.ContextUsed != nil || got.RateLimits != nil || got.Cache != nil || got.Effort != nil {
		t.Fatalf("the parts of another shape: %+v", got)
	}
	sample.Sequence, sample.Native = 2, []byte(`["model"]`)
	postValue(t, socket, PathModState, sample)
	if got := server.SessionState(); !got.Fresh || got.Model != nil || got.Activity == nil {
		t.Fatalf("a native part that is no object: %+v", got)
	}
}

// The cache the Статус shows goes with the run to the next image: the mod
// sends its last step's usage, which by then may be a placeholder's, and the
// next image would show no cache where the response before it had one.
func TestTheCacheIsHandedToTheNextImage(t *testing.T) {
	short := func(s *Server) { s.cfg.Hold = 20 * time.Millisecond }
	before, socket := servingWith(t, short)
	postValue(t, socket, PathHello, HelloRequest{Protocol: Protocol, LoadID: "load", SessionID: "conversation-1", HarnessVersion: "test"})
	postValue(t, socket, PathModState, step(1, usage(900, 50, 50), nil))
	postValue(t, socket, PathModState, step(2, usage(0, 0, 0), nil))
	handed, file, err := before.HandOver()
	if err != nil {
		t.Fatal(err)
	}
	_ = before.listener.UnixListener.Close()
	after := New(Config{Socket: socket, Role: "main", Wait: 2 * time.Second, Hold: 20 * time.Millisecond, Record: filepath.Join(filepath.Dir(socket), "after.json")})
	if err := after.Adopt(handed, file); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(after.Close)
	postValue(t, socket, PathModState, step(3, usage(0, 0, 0), nil))
	cacheIs(t, after, &session.Cache{HitPercent: 90, Read: 900, Write: 50, Input: 50})
}
