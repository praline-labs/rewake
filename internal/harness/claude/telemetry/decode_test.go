package telemetry

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// secret stands for conversation text. Every place it is put, it must not
// come out of.
const secret = "PRIVATE-CONVERSATION-TEXT"

// The conversation fields of a hook payload are skipped by the decoder: they
// are not in the event, not in what is sent, and not anywhere a formatted
// value of it could show them.
func TestHookDecoderKeepsNoConversationText(t *testing.T) {
	payload := fmt.Sprintf(`{
		"session_id": "s1", "hook_event_name": "PostCompact", "trigger": "manual",
		"transcript_path": "/home/u/.claude/projects/x/s1.jsonl",
		"prompt": %[1]q, "last_assistant_message": %[1]q,
		"compact_summary": %[1]q, "custom_instructions": %[1]q,
		"message": %[1]q, "title": %[1]q,
		"nested": {"text": %[1]q}, "list": [%[1]q]
	}`, secret)
	event, ok := DecodeHook([]byte(payload))
	if !ok {
		t.Fatal("a PostCompact payload was not decoded")
	}
	if event.Kind != PostCompact || event.Trigger != "manual" || event.Session != "s1" {
		t.Errorf("event = %+v, want the named fields", event)
	}
	assertNoSecret(t, event)
}

func TestStatusDecoderKeepsNoConversationText(t *testing.T) {
	payload := fmt.Sprintf(`{"session_id": "s1", "transcript_path": %[1]q,
		"model": {"id": "m", "display_name": %[1]q},
		"workspace": {"current_dir": %[1]q}, "output_style": {"name": %[1]q}}`, secret)
	event, ok := DecodeStatus([]byte(payload))
	if !ok || event.Model != "m" {
		t.Fatalf("event = %+v ok=%v, want model m", event, ok)
	}
	assertNoSecret(t, event)
}

func assertNoSecret(t *testing.T, event Event) {
	t.Helper()
	raw, err := event.encode()
	if err != nil {
		t.Fatal(err)
	}
	for _, form := range []string{string(raw), fmt.Sprintf("%+v", event), fmt.Sprintf("%#v", event)} {
		if strings.Contains(form, secret) {
			t.Errorf("conversation text survived decoding: %s", form)
		}
	}
	if event.Context != nil {
		if strings.Contains(fmt.Sprintf("%+v", *event.Context), secret) {
			t.Error("conversation text survived in the context")
		}
	}
}

// A field whose type changed costs that field, not the event: the harness
// may move a value into an object in a later version.
func TestDecodersTolerateAChangedField(t *testing.T) {
	event, ok := DecodeHook([]byte(`{"hook_event_name":"SessionStart","source":"startup","model":{"id":"m"}}`))
	if !ok || event.Kind != SessionStart || event.Source != "startup" || event.Model != "" {
		t.Errorf("event = %+v ok=%v, want the event without the changed model", event, ok)
	}
	status, ok := DecodeStatus([]byte(`{"model":"m","context_window":{"context_window_size":200000,"current_usage":null}}`))
	if !ok || status.Model != "" || status.Context == nil || *status.Context.Window != 200000 {
		t.Errorf("status = %+v ok=%v, want the window without the changed model", status, ok)
	}
	if _, ok := DecodeHook([]byte(`not json`)); ok {
		t.Error("garbage decoded as a hook")
	}
	if _, ok := DecodeHook([]byte(`{"session_id":"s"}`)); ok {
		t.Error("a payload without an event name decoded as a hook")
	}
}

// A hook fired inside a subagent carries its agent id and says nothing about
// the session.
func TestSubagentHooksAreIgnored(t *testing.T) {
	if _, ok := DecodeHook([]byte(`{"hook_event_name":"Stop","agent_id":"a1"}`)); ok {
		t.Error("a subagent's hook was decoded")
	}
}

// Before the first response the harness sends zeros beside a null usage, and
// those are "not measured", not an empty context.
func TestStatusBeforeTheFirstResponseIsUnknown(t *testing.T) {
	event, _ := DecodeStatus([]byte(`{"model":{"id":"m"},"context_window":{"total_input_tokens":0,
		"context_window_size":200000,"current_usage":null,"used_percentage":null}}`))
	if event.Context == nil || event.Context.Used != nil || event.Context.Percent != nil || *event.Context.Window != 200000 {
		t.Errorf("context = %+v, want the window alone", event.Context)
	}
	if event.Effort != "" {
		t.Errorf("effort = %q, want none for a model without one", event.Effort)
	}
	// After a compaction the counts are zero until the next response, and
	// a usage object beside them must not turn that zero into an empty
	// context.
	compacted, _ := DecodeStatus([]byte(`{"model":{"id":"m"},"context_window":{"total_input_tokens":0,
		"context_window_size":200000,"current_usage":{"input_tokens":0},"used_percentage":0}}`))
	if compacted.Context == nil || compacted.Context.Used != nil || compacted.Context.Percent != nil {
		t.Errorf("after a compaction context = %+v, want the window alone", compacted.Context)
	}
	measured, _ := DecodeStatus([]byte(`{"model":{"id":"m"},"effort":{"level":"high"},"context_window":{"total_input_tokens":34727,
		"context_window_size":200000,"current_usage":{"input_tokens":10},"used_percentage":17}}`))
	if measured.Effort != "high" || *measured.Context.Used != 34727 || *measured.Context.Percent != 17 {
		t.Errorf("measured = %+v %+v, want effort, used and percent", measured, measured.Context)
	}
}

// What goes on the wire decodes back to the same event, and an oversized one
// is refused at both ends.
func TestEventsRoundTripAndAreBounded(t *testing.T) {
	used, window, percent := int64(5), int64(10), 50
	event := Event{
		At: 7, Kind: StatusLine, Session: "s", Model: "m", Effort: "low",
		Context: &Context{Used: &used, Window: &window, Percent: &percent},
	}
	raw, err := event.encode()
	if err != nil {
		t.Fatal(err)
	}
	back, ok := decodeEvent(raw)
	if !ok {
		t.Fatal("an encoded event did not decode")
	}
	again, _ := json.Marshal(back)
	if string(again) != string(raw) {
		t.Errorf("round trip changed the event: %s vs %s", again, raw)
	}
	if _, err := (Event{Kind: "x", Session: strings.Repeat("a", maxDatagram)}).encode(); err == nil {
		t.Error("an oversized event was encoded")
	}
	if _, ok := decodeEvent([]byte(`{"at":1}`)); ok {
		t.Error("an event without a kind decoded")
	}
}
