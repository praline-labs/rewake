package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// The two requests `rewake compact` and `rewake interrupt` send on this column,
// answered the way the server of 0.155.1 answers them: the shapes from its
// schema, the order and the refusals from the live run of September 24, 2026
// recorded in docs/research-protocol.md.

var (
	compactShape = served{kind: "object", fields: map[string]served{
		"threadId": {kind: "string"},
	}, required: []string{"threadId"}}
	interruptShape = served{kind: "object", fields: map[string]served{
		"threadId": {kind: "string"},
		"turnId":   {kind: "string"},
	}, required: []string{"threadId", "turnId"}}
)

// eventSequence is several events to send after a reply, in order.
type eventSequence []any

// Usage the fixture reports: a work turn fills the context to workTokens, and
// a compaction leaves compactedTokens of it. The steered scenario compares the
// answer's counts against these.
const (
	workTokens      = 120000
	compactedTokens = 9000
)

// compact answers thread/compact/start. The server does not refuse a
// compaction while a turn runs: its compact() aborts the running turn and
// compacts in its place, so a held turn here ends interrupted, and whether
// that happens is rewake's choice to make before it asks. The compaction then
// runs as a turn of its own and the reply comes before any of its events.
func (s *shimSession) compact(params json.RawMessage) (any, any, error) {
	if wrong := unserved("thread/compact/start", params, compactShape); wrong != "" {
		return nil, nil, errors.New(wrong)
	}
	if asked := threadOf(params); asked != s.thread {
		return nil, nil, fmt.Errorf("no such thread %q", asked)
	}
	s.turn.mu.Lock()
	if s.turn.open != "" {
		s.abortOpen()
	}
	s.turn.compactions++
	id := fmt.Sprintf("compaction-%d", s.turn.compactions)
	s.turn.mu.Unlock()
	s.recordTurnEvent("compacted", id, "")
	item := compactionItem(id)
	return map[string]any{}, eventSequence{
		s.threadStatusChangedEvent(activeStatus()),
		s.turnStartedEvent(id),
		s.itemEvent("item/started", "startedAtMs", id, item),
		s.usageEvent(id, compactedTokens),
		s.itemEvent("item/completed", "completedAtMs", id, item),
		s.threadStatusChangedEvent(idleStatus()),
		map[string]any{
			"method": "turn/completed",
			"params": map[string]any{"threadId": s.thread, "turn": turnObject(id, "completed", item)},
		},
	}, nil
}

// interrupt answers turn/interrupt: refused with the server's own words when
// no turn runs or another one does, otherwise the held turn ends interrupted.
func (s *shimSession) interrupt(params json.RawMessage) (any, any, error) {
	if wrong := unserved("turn/interrupt", params, interruptShape); wrong != "" {
		return nil, nil, errors.New(wrong)
	}
	if asked := threadOf(params); asked != s.thread {
		return nil, nil, fmt.Errorf("no such thread %q", asked)
	}
	var asked struct {
		Turn string `json:"turnId"`
	}
	_ = json.Unmarshal(params, &asked)
	s.turn.mu.Lock()
	defer s.turn.mu.Unlock()
	switch s.turn.open {
	case "":
		return nil, nil, errors.New("no active turn to interrupt")
	case asked.Turn:
		s.abortOpen()
		return map[string]any{}, nil, nil
	default:
		return nil, nil, fmt.Errorf("expected active turn id %s but found %s", asked.Turn, s.turn.open)
	}
}

// abortOpen ends the held turn as interrupted. Called under s.turn.mu.
func (s *shimSession) abortOpen() {
	s.turn.aborted = true
	if s.turn.abort != nil {
		close(s.turn.abort)
		s.turn.abort = nil
	}
}

// takeAborted says whether the turn that just left its hold was aborted, and
// clears the mark for the next one.
func (s *shimSession) takeAborted() bool {
	s.turn.mu.Lock()
	defer s.turn.mu.Unlock()
	aborted := s.turn.aborted
	s.turn.aborted = false
	return aborted
}

func compactionItem(id string) map[string]any {
	return map[string]any{"id": "item-" + id, "type": "contextCompaction"}
}

// itemEvent is item/started or item/completed with the timestamp each one
// requires.
func (s *shimSession) itemEvent(method, stamp, turn string, item map[string]any) map[string]any {
	return map[string]any{
		"method": method,
		"params": map[string]any{"threadId": s.thread, "turnId": turn, "item": item, stamp: time.Now().UnixMilli()},
	}
}

// usageEvent is thread/tokenUsage/updated with the context the turn left. The
// window is left out, as the schema allows, so no share of it is ever computed.
func (s *shimSession) usageEvent(turn string, tokens int64) map[string]any {
	breakdown := map[string]any{"cachedInputTokens": 0, "inputTokens": tokens, "outputTokens": 0, "reasoningOutputTokens": 0, "totalTokens": tokens}
	return map[string]any{
		"method": "thread/tokenUsage/updated",
		"params": map[string]any{"threadId": s.thread, "turnId": turn, "tokenUsage": map[string]any{"last": breakdown, "total": breakdown}},
	}
}

// steeringShapes are the shape case's observations about the two requests.
var steeringShapes = []string{
	"the thread/compact/start reply matches ThreadCompactStartResponse",
	"the compaction's item/started event matches ItemStartedNotification",
	"the thread/tokenUsage/updated event matches ThreadTokenUsageUpdatedNotification",
	"the compaction's item/completed event matches ItemCompletedNotification",
	"the compaction's turn/completed event matches TurnCompletedNotification",
	"the turn/interrupt reply matches TurnInterruptResponse",
	"the fixture refuses every compaction and interrupt the schema refuses",
}

// steeringAgainstTheSchema checks what the fixture sends for the two requests,
// and that it accepts neither request in a shape the schema refuses.
func steeringAgainstTheSchema(c *Case, bundle *schemaBundle) {
	session := &shimSession{thread: shimThread}
	reply, after, err := session.compact(json.RawMessage(`{"threadId":"` + shimThread + `"}`))
	events, _ := after.(eventSequence)
	if err != nil || len(events) != 7 {
		for _, observation := range steeringShapes[:5] {
			c.Contradicted(observation, "the shim did not compact: %v, %d events", err, len(events))
		}
	} else {
		check(c, bundle, "ThreadCompactStartResponse", steeringShapes[0], reply)
		for i, sent := range []struct {
			typeName string
			at       int
		}{{"ItemStartedNotification", 2}, {"ThreadTokenUsageUpdatedNotification", 3}, {"ItemCompletedNotification", 4}, {"TurnCompletedNotification", 6}} {
			event, _ := events[sent.at].(map[string]any)
			check(c, bundle, sent.typeName, steeringShapes[i+1], eventParams(event))
		}
	}
	session.turn.open, session.turn.abort = "turn-1", make(chan struct{})
	reply, _, err = session.interrupt(json.RawMessage(`{"threadId":"` + shimThread + `","turnId":"turn-1"}`))
	if err != nil {
		c.Contradicted(steeringShapes[5], "the shim refused a valid interrupt: %v", err)
	} else {
		check(c, bundle, "TurnInterruptResponse", steeringShapes[5], reply)
	}
	for _, sample := range []struct{ typeName, params string }{
		{"ThreadCompactStartParams", `{}`},
		{"ThreadCompactStartParams", `{"threadId":5}`},
		{"TurnInterruptParams", `{"threadId":"` + shimThread + `"}`},
		{"TurnInterruptParams", `{"threadId":"` + shimThread + `","turnId":5}`},
	} {
		var params map[string]any
		if json.Unmarshal([]byte(sample.params), &params) != nil || len(bundle.check(sample.typeName, params)) == 0 {
			continue
		}
		fresh := &shimSession{thread: shimThread}
		fresh.turn.open, fresh.turn.abort = "turn-1", make(chan struct{})
		var err error
		if sample.typeName == "ThreadCompactStartParams" {
			_, _, err = fresh.compact(json.RawMessage(sample.params))
		} else {
			_, _, err = fresh.interrupt(json.RawMessage(sample.params))
		}
		if err == nil {
			c.Contradicted(steeringShapes[6], "the schema refuses %s %s and the fixture accepts it", sample.typeName, sample.params)
			return
		}
	}
	c.Observed(steeringShapes[6], "no sample the schema refuses is accepted here")
}
