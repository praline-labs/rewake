package pi

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mirror"
)

// piEvents feeds pi's events to a translator as the stream carries them,
// one batch per emit, and keeps what it made of them.
type piEvents struct {
	t     *testing.T
	tr    *translator
	clock time.Time
	out   []mirror.Event
}

func translating(t *testing.T) *piEvents {
	p := &piEvents{t: t, tr: newTranslator(), clock: time.Unix(1000, 0)}
	ids := 0
	p.tr.now = func() time.Time { return p.clock }
	p.tr.id = func() string { ids++; return fmt.Sprintf("id%d", ids) }
	return p
}

// emit takes one batch, each event as the stream's JSON.
func (p *piEvents) emit(lines ...string) {
	p.t.Helper()
	batch := make([]raw, len(lines))
	for i, line := range lines {
		if err := json.Unmarshal([]byte(line), &batch[i]); err != nil {
			p.t.Fatalf("%s: %v", line, err)
		}
	}
	p.out = append(p.out, p.tr.take(batch)...)
}

func (p *piEvents) tick(d time.Duration) { p.clock = p.clock.Add(d) }

// seen is each event's kind, id and what it says.
func seen(events []mirror.Event) []string {
	out := make([]string, len(events))
	for i, e := range events {
		said := e.Text
		switch e.Kind {
		case mirror.EventTurn:
			said = string(e.State)
		case mirror.EventTool:
			said = e.Name
		}
		out[i] = string(e.Kind) + " " + e.ID + " " + said
	}
	return out
}

func kinds(events []mirror.Event, kind mirror.EventKind) []mirror.Event {
	return slices.DeleteFunc(slices.Clone(events), func(e mirror.Event) bool { return e.Kind != kind })
}

func same(t *testing.T, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

const call1 = `{"type":"toolCall","id":"c1","name":"read","arguments":{}}`

func TestATurnStreamsItsPromptThinkingReplyAndCallsInTheFacesWords(t *testing.T) {
	p := translating(t)
	p.emit(`{"event":"input","text":"look","source":"interactive"}`, `{"event":"agent_start"}`, `{"event":"turn_start","turnIndex":1}`,
		`{"event":"message_update","type":"thinking_start","contentIndex":0}`,
		`{"event":"message_update","type":"thinking_end","contentIndex":0,"content":""}`,
		`{"event":"message_update","type":"text_delta","contentIndex":1,"delta":"Hi"}`,
		`{"event":"message_update","type":"text_end","contentIndex":1,"content":"Hi there"}`,
		`{"event":"message_update","type":"toolcall_start","contentIndex":2,"block":`+call1+`}`)
	p.tick(200 * time.Millisecond)
	p.emit(`{"event":"message_update","type":"toolcall_delta","contentIndex":2,"delta":"{\"path\":\"a\"}"}`)
	// Within the interval a delta says nothing more; the end says it whole.
	p.emit(`{"event":"message_update","type":"toolcall_delta","contentIndex":2,"delta":" "}`,
		`{"event":"message_update","type":"toolcall_end","contentIndex":2,"toolCall":{"id":"c1","name":"read","arguments":{"path":"a"}}}`,
		`{"event":"message_end","role":"assistant","stopReason":"toolUse"}`,
		`{"event":"tool_execution_start","toolCallId":"c1","toolName":"read","args":{"path":"a"}}`,
		`{"event":"tool_call","toolCallId":"c1"}`,
		`{"event":"tool_execution_end","toolCallId":"c1","result":{"content":[{"type":"text","text":"body"}]},"isError":false,"durationMs":3}`,
		`{"event":"tool_execution_start","toolCallId":"n1","toolName":"bash","args":{},"parentToolCallId":"c1"}`,
		`{"event":"agent_settled","aborted":false}`)
	same(t, seen(p.out), []string{
		"prompt id1 look", "turn  working", "delta id2.0.1.1 Hi", "settle id2.0.1.1 Hi there", "tool c1 Read",
		`input c1 {"file_path":"a"}`, `input c1 {"file_path":"a"}`, "tool c1 Read", `input c1 {"file_path":"a"}`,
		"result c1 body", "turn  idle",
	})
	if p.out[0].Origin != "person" || p.out[0].Turn != "" || p.out[3].Turn != "id2" || p.out[3].Of != 1 || p.out[9].Status != mirror.ItemStatusDone || p.out[10].Turn != "id2" {
		t.Fatalf("the events: %+v", p.out)
	}
}

// Seen live (October 31, 2026, brief 49): at minimal thinking every step of
// pi's opened an empty thinking block, a signature only, and the face drew a
// row for each where the Claude Code worker's drew none.
func TestAThinkingBlockShowsOnlyWhenItHasSomethingInIt(t *testing.T) {
	p := translating(t)
	block := func(kind string, index int, more string) string {
		return fmt.Sprintf(`{"event":"message_update","type":%q,"contentIndex":%d%s}`, kind, index, more)
	}
	p.emit(`{"event":"agent_start"}`, `{"event":"turn_start","turnIndex":0}`,
		block("thinking_end", 0, `,"content":""`), block("thinking_end", 1, `,"content":" \n"`),
		block("thinking_delta", 2, `,"delta":"hmm"`), block("thinking_end", 2, `,"content":"hmm"`),
		block("thinking_end", 3, `,"content":"a summary"`), `{"event":"agent_settled"}`)
	same(t, seen(p.out), []string{"turn  working", "delta id1.0.0.2 hmm", "settle id1.0.0.2 hmm", "settle id1.0.0.3 a summary", "turn  idle"})
}

func TestACallAHookBlockedEndsRefusedOnePisChecksRefusedOrAnAbortAnError(t *testing.T) {
	p := translating(t)
	failed := `"result":{"content":[{"type":"text","text":"not allowed"}]},"isError":true`
	p.emit(`{"event":"agent_start"}`, `{"event":"tool_call","toolCallId":"blocked"}`,
		`{"event":"tool_execution_end","toolCallId":"blocked",`+failed+`}`,
		`{"event":"tool_execution_end","toolCallId":"unknown",`+failed+`}`,
		`{"event":"tool_call","toolCallId":"cut"}`, `{"event":"tool_execution_end","toolCallId":"cut",`+failed+`,"aborted":true}`,
		`{"event":"tool_call","toolCallId":"ran"}`, `{"event":"tool_execution_end","toolCallId":"ran",`+failed+`,"durationMs":4}`)
	results := kinds(p.out, mirror.EventResult)
	var got []string
	for _, e := range results {
		got = append(got, e.ID+" "+string(e.Status))
	}
	same(t, got, []string{"blocked refused", "unknown error", "cut error", "ran error"})
	if permit := results[0].Permit; permit == nil || permit.By != mirror.PermitByHook || permit.Allowed || *permit.Reason != "not allowed" {
		t.Fatalf("the permit: %+v", permit)
	}
}

func TestTheFacesPromptIsPisInputFromTheFaceSteeredWhilePiWorks(t *testing.T) {
	p := translating(t)
	p.tr.sent("go")
	p.emit(`{"event":"input","text":"go","source":"extension"}`, `{"event":"agent_start"}`)
	p.tr.sent("more")
	p.emit(`{"event":"input","text":"more","source":"extension","streamingBehavior":"steer"}`, `{"event":"input","text":"other","source":"extension"}`)
	var got []string
	for _, e := range kinds(p.out, mirror.EventPrompt) {
		got = append(got, fmt.Sprintf("%s %s %v %s", e.Text, e.Origin, e.Turn == "", e.Status))
	}
	same(t, got, []string{"go face true ", "more face false queued", "other extension true "})
}

func TestEachRunPiStartsWithinATurnHasBlocksOfItsOwn(t *testing.T) {
	p := translating(t)
	// A retryable failure starts pi's loop again, its turns counted from 0.
	p.emit(`{"event":"agent_start"}`, `{"event":"turn_start","turnIndex":0}`, `{"event":"message_update","type":"text_end","contentIndex":0,"content":"first"}`,
		`{"event":"agent_start"}`, `{"event":"turn_start","turnIndex":0}`, `{"event":"message_update","type":"text_end","contentIndex":0,"content":"retried"}`,
		`{"event":"agent_settled"}`)
	settles := kinds(p.out, mirror.EventSettle)
	if len(settles) != 2 || settles[0].ID == settles[1].ID || settles[0].Turn != settles[1].Turn {
		t.Fatalf("the settles: %+v", settles)
	}
}

// Brief 56: pi tells no extension of its retries (auto_retry_* go to its own
// screen only); a run started again after a failed message is one. Its
// screen says each failure in red, then `Retry failed after N attempts` or
// `Aborted after N retry attempts`.
func TestAFailedMessagePiTriesAgainIsCountedAndTheRetriesEndSaidAsPiSaysIt(t *testing.T) {
	const failed = `{"event":"message_end","role":"assistant","stopReason":"error","errorMessage":"503"}`
	notices := func(p *piEvents) []string {
		var out []string
		for _, e := range kinds(p.out, mirror.EventNotice) {
			out = append(out, e.Text)
		}
		return out
	}
	exhausted := translating(t)
	exhausted.emit(`{"event":"agent_start"}`, failed, `{"event":"agent_start"}`, failed, `{"event":"agent_start"}`, failed, `{"event":"agent_start"}`, failed, `{"event":"agent_settled"}`)
	same(t, notices(exhausted), []string{"Error: 503", "Error: 503", "Error: 503", "Error: 503", "Error: Retry failed after 3 attempts: 503"})
	stopped := translating(t)
	stopped.emit(`{"event":"agent_start"}`, failed, `{"event":"agent_start"}`, `{"event":"message_end","role":"assistant","stopReason":"aborted"}`, `{"event":"agent_settled","aborted":true}`)
	same(t, notices(stopped), []string{"Error: 503", "Aborted after 1 retry attempt"})
	// A retry that came through, or a failure pi does not retry, says no more.
	through := translating(t)
	through.emit(`{"event":"agent_start"}`, failed, `{"event":"agent_start"}`, `{"event":"message_end","role":"assistant","stopReason":"stop"}`, `{"event":"agent_settled"}`,
		`{"event":"agent_start"}`, failed, `{"event":"agent_settled"}`)
	same(t, notices(through), []string{"Error: 503", "Error: 503"})
}

// The count pi's screen shows as `Retrying (2/3)` is the turn's fact: each
// failed message says the attempt on a working turn event, a call it asked
// for or not, and the first item come back after it says none.
func TestEachFailedMessageIsSaidAsTheTurnsRetryAttemptUntilSomethingComesBack(t *testing.T) {
	p := translating(t)
	p.emit(`{"event":"agent_start"}`, `{"event":"message_end","role":"assistant","stopReason":"error","errorMessage":"503"}`,
		`{"event":"agent_start"}`, `{"event":"turn_start","turnIndex":0}`,
		`{"event":"message_update","type":"toolcall_start","contentIndex":0,"block":{"type":"toolCall","id":"c","name":"bash","arguments":{}}}`,
		`{"event":"message_end","role":"assistant","stopReason":"error","errorMessage":"503","content":[{"type":"toolCall"}]}`,
		`{"event":"agent_start"}`, `{"event":"turn_start","turnIndex":0}`,
		`{"event":"message_update","type":"text_delta","contentIndex":0,"delta":"ok"}`,
		`{"event":"message_update","type":"text_delta","contentIndex":0,"delta":"!"}`,
		`{"event":"message_end","role":"assistant","stopReason":"stop"}`, `{"event":"agent_settled"}`)
	var turns, order []string
	for _, e := range p.out {
		order = append(order, string(e.Kind))
		if e.Kind == mirror.EventTurn {
			turns = append(turns, fmt.Sprintf("%s %d", e.State, e.Retry))
		}
	}
	same(t, turns, []string{"working 0", "working 1", "working 0", "working 2", "working 0", "idle 0"})
	// The attempt comes after the failure's notice, the reset before the item.
	same(t, order[:4], []string{"turn", "notice", "turn", "turn"})
	same(t, order[len(order)-4:len(order)-2], []string{"turn", "delta"})
}
