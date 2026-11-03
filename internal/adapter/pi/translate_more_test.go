package pi

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mirror"
)

// taken is each taken event as the prompt's text and how it was taken.
func taken(p *piEvents) []string {
	texts := map[string]string{}
	var out []string
	for _, e := range p.out {
		switch e.Kind {
		case mirror.EventPrompt:
			texts[e.ID] = e.Text
		case mirror.EventTaken:
			how := "taken"
			if e.Status != "" {
				how = string(e.Status)
			}
			out = append(out, texts[e.ID]+" "+how)
		}
	}
	return out
}

func steer(text string) string {
	return fmt.Sprintf(`{"event":"input","text":%q,"source":"extension","streamingBehavior":"steer"}`, text)
}

func user(text string) string {
	return fmt.Sprintf(`{"event":"message_start","role":"user","content":[{"type":"text","text":%q}],"pending":true}`, text)
}

// pi takes a steered prompt after the tool batch, its user message beginning
// then: the face hears it taken, the same texts one at a time.
func TestASteeredPromptWaitsQueuedUntilPiTakesIt(t *testing.T) {
	p := translating(t)
	p.emit(`{"event":"agent_start"}`, steer("a"), steer("a"), user("a"))
	same(t, taken(p), []string{"a taken"})
	p.emit(user("a"))
	same(t, taken(p), []string{"a taken", "a taken"})
}

// Review of 8e94ba1: an input handler consumed one prompt and transformed the
// next, and the transformed message was said to be the consumed prompt, the
// oldest. A message of no queued text is none of them: both are given back
// once pi's queue is found empty.
func TestAMessageOfNoQueuedTextIsSaidToBeNoQueuedPrompt(t *testing.T) {
	p := translating(t)
	p.emit(`{"event":"agent_start"}`, steer("handled"), steer("/tpl x"), user("the template, expanded"))
	same(t, taken(p), nil)
	empty := `{"event":"message_start","role":"assistant","pending":false}`
	p.emit(empty, empty)
	same(t, taken(p), []string{"handled refused", "/tpl x refused"})
}

// Alt+up in pi's own screen takes the queue back into its editor while the
// run goes on: the next two of pi's events find the queue empty. Esc's end
// of the run gives back what is left before the turn's end.
func TestAPromptPiGaveBackByAltUpOrEscIsRefused(t *testing.T) {
	p := translating(t)
	p.emit(`{"event":"agent_start"}`, steer("back"), `{"event":"turn_start","turnIndex":1,"pending":false}`)
	same(t, taken(p), nil)
	p.emit(`{"event":"message_start","role":"assistant","pending":false}`)
	same(t, taken(p), []string{"back refused"})
	p.emit(steer("left"), `{"event":"agent_settled","aborted":true}`)
	same(t, taken(p), []string{"back refused", "left refused"})
	if last := p.out[len(p.out)-2:]; last[0].Kind != mirror.EventTaken || last[1].Kind != mirror.EventTurn {
		t.Fatalf("the end: %+v", last)
	}
}

func turnsOf(p *piEvents) []string {
	var out []string
	for _, e := range kinds(p.out, mirror.EventTurn) {
		out = append(out, e.Turn+" "+string(e.State))
	}
	return out
}

// pi says every compaction alike, its own at the threshold, the person's
// /compact and the one main asks for: one session_before_compact, then
// session_compact or session_compact_failed.
func TestACompactionInsideARunIsTheRunsWhichGoesOnWorkingAfterIt(t *testing.T) {
	p := translating(t)
	p.emit(`{"event":"agent_start"}`, `{"event":"session_before_compact"}`, `{"event":"session_compact"}`, `{"event":"agent_settled"}`)
	same(t, turnsOf(p), []string{"id1 working", "id1 compacting", "id1 working", "id1 idle"})
}

func TestACompactionOutsideARunLeavesPiIdleAndOneRefusedSaysNothing(t *testing.T) {
	for _, end := range []string{"session_compact", "session_compact_failed"} {
		p := translating(t)
		// pi's /compact while it works stops the run first.
		p.emit(`{"event":"agent_start"}`, `{"event":"agent_settled","aborted":true}`, `{"event":"session_before_compact"}`, `{"event":"`+end+`"}`)
		same(t, turnsOf(p), []string{"id1 working", "id1 idle", "id2 compacting", "id2 idle"})
	}
	p := translating(t)
	p.emit(`{"event":"session_compact_failed"}`)
	same(t, turnsOf(p), nil)
}

// pi's own screen ends an aborted or failed message so
// (dist/modes/interactive/interactive-mode.js, message_end, and
// components/assistant-message.js, read on 1.4.0): pi sends no update for
// it, and its blocks heal what streamed.
func TestAReplyStoppedMidStreamKeepsWhatStreamedAndSaysOperationAborted(t *testing.T) {
	p := translating(t)
	p.emit(`{"event":"agent_start"}`, `{"event":"turn_start","turnIndex":0}`,
		`{"event":"message_update","type":"thinking_delta","contentIndex":0,"delta":"hm"}`,
		`{"event":"message_update","type":"text_delta","contentIndex":1,"delta":"Half a"}`,
		`{"event":"message_end","role":"assistant","stopReason":"aborted","content":[{"type":"thinking","thinking":"hmm"},{"type":"text","text":"Half a sentence"}]}`,
		`{"event":"agent_settled","aborted":true}`)
	same(t, seen(p.out), []string{"turn  working", "delta id1.0.0.0 hm", "delta id1.0.0.1 Half a", "settle id1.0.0.0 hmm", "settle id1.0.0.1 Half a sentence", "notice id2 Operation aborted", "turn  idle"})
	if end := p.out[len(p.out)-1]; end.Status != mirror.ItemStatusError || p.out[5].Status != mirror.ItemStatusError {
		t.Fatalf("the end: %+v", p.out)
	}
}

func TestCallsAnAbortedOrFailedMessageAskedForEndWithItsReason(t *testing.T) {
	p := translating(t)
	start := func(id string) string {
		return `{"event":"message_update","type":"toolcall_start","contentIndex":0,"block":{"type":"toolCall","id":"` + id + `","name":"bash","arguments":{}}}`
	}
	p.emit(`{"event":"agent_start"}`, `{"event":"turn_start","turnIndex":0}`, start("ran"),
		`{"event":"message_end","role":"assistant","stopReason":"toolUse"}`,
		`{"event":"tool_execution_start","toolCallId":"ran","toolName":"bash","args":{"command":"sleep 30"}}`,
		`{"event":"tool_execution_end","toolCallId":"ran","result":{"content":[{"type":"text","text":"Command aborted"}]},"isError":true,"durationMs":5000}`,
		`{"event":"turn_start","turnIndex":1}`, start("asked"),
		`{"event":"message_end","role":"assistant","stopReason":"aborted","content":[{"type":"toolCall"}]}`,
		// A failed message with no call says pi's error, as its screen does.
		`{"event":"turn_start","turnIndex":2}`,
		`{"event":"message_end","role":"assistant","stopReason":"error","errorMessage":"This operation was aborted","content":[]}`)
	var got []string
	for _, e := range p.out {
		if e.Kind == mirror.EventResult || e.Kind == mirror.EventNotice {
			got = append(got, fmt.Sprintf("%s %s %s %s", e.Kind, e.ID, e.Status, e.Text))
		}
	}
	same(t, got, []string{"result ran error Command aborted", "result asked error Operation aborted", "notice id2 error Error: This operation was aborted"})
}

// Review of b1f3f12: an Esc during a running call ends with the call's error
// and no notice; the stop is the turn's end saying so.
func TestATurnsEndSaysItWasStoppedOnlyWhenPiSettledAborted(t *testing.T) {
	for _, aborted := range []bool{true, false} {
		p := translating(t)
		p.emit(`{"event":"agent_start"}`, `{"event":"tool_execution_start","toolCallId":"c1","toolName":"bash","args":{"command":"x"}}`,
			`{"event":"tool_execution_end","toolCallId":"c1","result":{"content":[{"type":"text","text":"boom"}]},"isError":true,"durationMs":1}`,
			fmt.Sprintf(`{"event":"agent_settled","aborted":%v}`, aborted))
		if end := p.out[len(p.out)-1]; end.State != mirror.TurnStateIdle || (end.Status == mirror.ItemStatusError) != aborted {
			t.Fatalf("aborted %v: %+v", aborted, end)
		}
	}
}

// A long event comes in parts of its text, put together here; a part out
// of place drops what was being put together, lost with its batch.
func TestALongEventIsPutTogetherFromItsParts(t *testing.T) {
	p := translating(t)
	whole, _ := json.Marshal(map[string]any{"event": "input", "text": "a long prompt", "source": "interactive"})
	half := len(whole) / 2
	part := func(at, of int, text []byte) string {
		line, _ := json.Marshal(map[string]any{"event": "part", "part": at, "of": of, "text": string(text)})
		return string(line)
	}
	p.emit(part(0, 2, whole[:half]))
	p.emit(part(1, 2, whole[half:]))
	p.emit(part(0, 2, whole[:half]), `{"event":"lost","events":1}`, part(1, 2, whole[half:]))
	same(t, seen(p.out), []string{"prompt id1 a long prompt", "resync  "})
}

// A delta the stream lost leaves a call's input to its end, which says it
// whole; an event lost resyncs the model, counting what was lost.
func TestWhatTheStreamLostLeavesACallsInputToItsEnd(t *testing.T) {
	p := translating(t)
	p.emit(`{"event":"agent_start"}`, `{"event":"message_update","type":"toolcall_start","contentIndex":0,"block":{"type":"toolCall","id":"w","name":"write","arguments":{}}}`,
		`{"event":"message_update","type":"toolcall_delta","contentIndex":0,"delta":"{\"path\":\"a\",\"content\":\"he"}`)
	p.emit(`{"event":"lost","deltas":2}`)
	p.tick(time.Second)
	p.emit(`{"event":"message_update","type":"toolcall_delta","contentIndex":0,"delta":"llo"}`)
	p.emit(`{"event":"message_update","type":"toolcall_end","contentIndex":0,"toolCall":{"id":"w","name":"write","arguments":{"path":"a","content":"hello"}}}`,
		`{"event":"lost","deltas":1,"events":2}`)
	same(t, seen(p.out)[1:], []string{"tool w Write", `input w {"file_path":"a","content":"he"}`, `input w {"file_path":"a","content":"hello"}`, "resync  "})
	if lost := p.out[len(p.out)-1].Lost; lost != 3 {
		t.Fatalf("the resync counts %d", lost)
	}
}

// pi-ai parses a call's streamed arguments as they come; so does the wrapper.
func TestACallsStreamedArgumentsAreReadAsFarAsTheyGo(t *testing.T) {
	cases := map[string]string{
		``:                         `{}`,
		`{`:                        `{}`,
		`{"pa`:                     `{}`,
		`{"path":`:                 `{}`,
		`{"path":"a/b`:             `{"path":"a/b"}`,
		`{"path":"a\`:              `{"path":"a"}`,
		`{"path":"a\u00`:           `{"path":"a"}`,
		`{"n":12`:                  `{"n":12}`,
		`{"n":1.`:                  `{}`,
		`{"a":"x","b":tru`:         `{"a":"x"}`,
		`{"edits":[{"oldText":"x"`: `{"edits":[{"oldText":"x"}]}`,
		`{"edits":[{"o":"x"},`:     `{"edits":[{"o":"x"}]}`,
		`{"a":{"b":[1,2`:           `{"a":{"b":[1,2]}}`,
		`[1,2`:                     `{}`,
	}
	for prefix, want := range cases {
		if got := string(partialJSON(prefix)); got != want {
			var a, b any
			if json.Unmarshal([]byte(got), &a) != nil || json.Unmarshal([]byte(want), &b) != nil || fmt.Sprint(a) != fmt.Sprint(b) {
				t.Errorf("%q: %s, want %s", prefix, got, want)
			}
		}
	}
}
