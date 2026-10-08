package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
)

var msgSeq atomic.Int64

func messageID() string { return fmt.Sprintf("msg_standin_%d", msgSeq.Add(1)) }

// contentBlock is the answer as one content block; input is a real object here.
func contentBlock(a answer) map[string]any {
	if a.tool != "" {
		return map[string]any{"type": "tool_use", "id": a.toolID, "name": a.tool, "input": json.RawMessage(a.input)}
	}
	return map[string]any{"type": "text", "text": a.text}
}

func messageBody(model string, a answer) map[string]any {
	return map[string]any{
		"id": messageID(), "type": "message", "role": "assistant", "model": model,
		"content":       []any{contentBlock(a)},
		"stop_reason":   a.stop(),
		"stop_sequence": nil,
		"usage":         map[string]int{"input_tokens": 10, "output_tokens": 5},
	}
}

// writeStream sends the answer in the streaming shape of the message API: the
// block opens empty and its payload arrives as one delta, a text_delta for text
// and an input_json_delta carrying partial_json for a tool call.
func writeStream(w http.ResponseWriter, model string, a answer) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	fl, _ := w.(http.Flusher)
	emit := func(event string, data map[string]any) {
		data["type"] = event
		b, _ := json.Marshal(data)
		_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
		if fl != nil {
			fl.Flush()
		}
	}

	start := map[string]any{
		"id": messageID(), "type": "message", "role": "assistant", "model": model,
		"content": []any{}, "stop_reason": nil, "stop_sequence": nil,
		"usage": map[string]int{"input_tokens": 10, "output_tokens": 1},
	}
	emit("message_start", map[string]any{"message": start})

	opened := map[string]any{"type": "text", "text": ""}
	delta := map[string]any{"type": "text_delta", "text": a.text}
	if a.tool != "" {
		opened = map[string]any{"type": "tool_use", "id": a.toolID, "name": a.tool, "input": map[string]any{}}
		delta = map[string]any{"type": "input_json_delta", "partial_json": a.input}
	}
	emit("content_block_start", map[string]any{"index": 0, "content_block": opened})
	emit("content_block_delta", map[string]any{"index": 0, "delta": delta})
	emit("content_block_stop", map[string]any{"index": 0})
	emit("message_delta", map[string]any{
		"delta": map[string]any{"stop_reason": a.stop(), "stop_sequence": nil},
		"usage": map[string]int{"output_tokens": 5},
	})
	emit("message_stop", map[string]any{})
}
