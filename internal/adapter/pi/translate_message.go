package pi

import (
	"cmp"
	"encoding/json"
	"maps"
	"slices"
	"strconv"

	"github.com/praline-labs/rewake/internal/core/mirror"
)

// streamed takes a piece of a reply as pi-ai streamed it.
func (t *translator) streamed(e raw) {
	switch e.Type {
	case "text_delta", "thinking_delta":
		block := mirror.ItemKindText
		if e.Type == "thinking_delta" {
			block = mirror.ItemKindThinking
		}
		if e.Delta == "" {
			return
		}
		id := t.blockID(e.ContentIndex)
		if block == mirror.ItemKindThinking {
			mark(&t.Thought, id, true)
		}
		mark(&t.Open, id, e.ContentIndex)
		t.push(mirror.Event{Kind: mirror.EventDelta, ID: id, Turn: t.Run, Block: block, Text: e.Delta})
	case "text_end":
		id := t.blockID(e.ContentIndex)
		delete(t.Open, id)
		t.settle(id, mirror.ItemKindText, stringOf(e.Content))
	case "thinking_end":
		// A block with nothing in it, a signature only as most providers
		// send, shows nothing, as the Claude Code worker's face shows none:
		// a thinking row is drawn only for something to show.
		id := t.blockID(e.ContentIndex)
		delete(t.Open, id)
		content := stringOf(e.Content)
		streamed := t.Thought[id]
		delete(t.Thought, id)
		if streamed || !blank(content) {
			t.settle(id, mirror.ItemKindThinking, content)
		}
	case "toolcall_start":
		b := e.Block
		if b == nil || b.Type != "toolCall" || b.ID == "" || b.Name == "" {
			return
		}
		t.Seq++
		mark(&t.Calls, b.ID, &call{Name: b.Name, Start: b.Arguments, Seq: t.Seq})
		mark(&t.Index, e.ContentIndex, b.ID)
		t.push(mirror.Event{Kind: mirror.EventTool, ID: b.ID, Turn: t.Run, Name: facedTool(b.Name, nil).name})
	case "toolcall_delta":
		if c := t.Calls[t.Index[e.ContentIndex]]; c != nil && !c.Ended && !c.Broken {
			c.Streamed += e.Delta
			c.Changed = true
			// Past its bound a call's input waits for its end, which says it
			// whole: each say parses all that streamed.
			if len(c.Streamed) > maxStreamed {
				c.Broken, c.Streamed = true, ""
			}
		}
	case "toolcall_end":
		if b := e.ToolCall; b != nil && b.ID != "" && b.Name != "" {
			if c := t.Calls[b.ID]; c != nil {
				c.Ended, c.Changed, c.Streamed = true, false, ""
			}
			t.toolInput(b.ID, b.Name, b.Arguments)
		}
	}
}

// maxStreamed bounds a call's input parsed as it streams.
const maxStreamed = 256 << 10

// flush says the input of each call that streamed in the batch, at most
// once per inputEvery, parsed from what streamed so far.
func (t *translator) flush() {
	now := t.now()
	for _, id := range t.waiting() {
		c := t.Calls[id]
		if !c.Changed || c.Broken || c.Ended || now.Sub(c.Said) < inputEvery {
			continue
		}
		c.Changed, c.Said = false, now
		args := c.Start
		if c.Streamed != "" {
			args = partialJSON(c.Streamed)
		}
		t.toolInput(id, c.Name, args)
	}
}

func (t *translator) toolInput(id, name string, args json.RawMessage) {
	input, err := json.Marshal(facedTool(name, args).input)
	if err != nil {
		return
	}
	t.push(mirror.Event{Kind: mirror.EventInput, ID: id, Text: string(input)})
}

// ended takes an assistant message's end. One pi stopped or that failed ends
// as pi's screen ends it: what streamed stays, each call it asked for ends
// with the reason, and a message with no call says it in red.
func (t *translator) ended(e raw) {
	if e.Role != "assistant" {
		return
	}
	t.Index = nil
	retries := t.Retries
	t.Failed = nil
	if e.StopReason == "error" {
		failed := e.ErrorMessage
		if failed == "" {
			failed = "Unknown error"
		}
		t.Failed = &failed
	} else {
		t.Retries = 0
	}
	if e.StopReason != "aborted" && e.StopReason != "error" {
		return
	}
	var content []rawBlock
	_ = json.Unmarshal(e.Content, &content)
	for _, id := range t.open() {
		index := t.Open[id]
		if index < 0 || index >= len(content) {
			continue
		}
		switch block := content[index]; block.Type {
		case "text":
			t.settle(id, mirror.ItemKindText, block.Text)
		case "thinking":
			t.settle(id, mirror.ItemKindThinking, block.Thinking)
		}
	}
	t.Open, t.Thought = nil, nil
	reason := "Operation aborted"
	switch {
	case e.StopReason == "error":
		reason = e.ErrorMessage
		if reason == "" {
			reason = "Error"
		}
	case retries == 1:
		reason = "Aborted after 1 retry attempt"
	case retries > 1:
		reason = "Aborted after " + strconv.Itoa(retries) + " retry attempts"
	}
	for _, id := range t.waiting() {
		t.push(mirror.Event{Kind: mirror.EventResult, ID: id, Status: mirror.ItemStatusError, Text: reason})
	}
	t.Calls = nil
	asked := false
	for _, block := range content {
		asked = asked || block.Type == "toolCall"
	}
	if !asked {
		if e.StopReason == "aborted" {
			t.notice(reason)
		} else {
			t.notice("Error: " + *t.Failed)
		}
	}
	// Each failed request is the turn's retry attempt, as pi's screen counts
	// them: pi settles at once a failure it does not retry.
	if e.StopReason == "error" && t.Run != "" {
		t.Retrying = true
		t.push(mirror.Event{Kind: mirror.EventTurn, Turn: t.Run, State: mirror.TurnStateWorking, Retry: retries + 1})
	}
}

func (t *translator) toolStarted(id, name string, args json.RawMessage) {
	delete(t.Calls, id)
	t.push(mirror.Event{Kind: mirror.EventTool, ID: id, Turn: t.Run, Name: facedTool(name, nil).name})
	t.toolInput(id, name, args)
}

func (t *translator) toolEnded(e raw) {
	text := resultText(e.Result)
	if !e.IsError {
		t.push(mirror.Event{Kind: mirror.EventResult, ID: e.ToolCallID, Status: mirror.ItemStatusDone, Text: text})
		return
	}
	// A call pi's checks let through that never ran was blocked by a hook.
	if ran := e.DurationMs != nil; !ran && !e.Aborted && t.Reached[e.ToolCallID] {
		t.push(mirror.Event{Kind: mirror.EventResult, ID: e.ToolCallID, Status: mirror.ItemStatusRefused, Text: text, Permit: &mirror.Permit{By: mirror.PermitByHook, Reason: &text}})
		return
	}
	t.push(mirror.Event{Kind: mirror.EventResult, ID: e.ToolCallID, Status: mirror.ItemStatusError, Text: text})
}

// waiting are the calls waiting to run, in the order they streamed.
func (t *translator) waiting() []string {
	return slices.SortedFunc(maps.Keys(t.Calls), func(a, b string) int { return cmp.Compare(t.Calls[a].Seq, t.Calls[b].Seq) })
}

// open are the blocks streaming, in the message's order.
func (t *translator) open() []string {
	return slices.SortedFunc(maps.Keys(t.Open), func(a, b string) int { return cmp.Compare(t.Open[a], t.Open[b]) })
}
