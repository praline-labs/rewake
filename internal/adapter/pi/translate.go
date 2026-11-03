package pi

import (
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/praline-labs/rewake/internal/core/control"
	"github.com/praline-labs/rewake/internal/core/mirror"
)

// The face's mirror of a pi session (docs/face.md, docs/research-pi.md): the
// extension forwards pi's own events as they come (stream.ts), and the
// translator turns them into the model's neutral events here, so a change of
// what the face is told is the wrapper's alone. The prompts, the reply and
// thinking as they stream, each tool call with its input as it streams, its
// output and its end, and the turn.

// raw is one of pi's events as the stream carries it: the event's name and
// its fields as pi gave them, less what the stream left out; or one of the
// stream's own: a part of a long event, or a marker of what it lost.
type raw struct {
	Event string `json:"event"`
	// Part and Of place a part of a long event, whose serialized text it carries.
	Part int    `json:"part,omitempty"`
	Of   int    `json:"of,omitempty"`
	Text string `json:"text,omitempty"`
	// Deltas and Events count what a lost marker stands for.
	Deltas int `json:"deltas,omitempty"`
	Events int `json:"events,omitempty"`
	// An input's source and how pi was told to take it while it works.
	Source            string `json:"source,omitempty"`
	StreamingBehavior string `json:"streamingBehavior,omitempty"`
	TurnIndex         int    `json:"turnIndex,omitempty"`
	// Pending is pi's hasPendingMessages at the event.
	Pending *bool `json:"pending,omitempty"`
	// A message's start or end; Content is a user message's, or an aborted
	// or failed assistant message's blocks; a block's end carries its text.
	Role         string          `json:"role,omitempty"`
	StopReason   string          `json:"stopReason,omitempty"`
	ErrorMessage string          `json:"errorMessage,omitempty"`
	Content      json.RawMessage `json:"content,omitempty"`
	// A message_update: pi-ai's event less its partial message; a tool
	// call's start carries the call's native block.
	Type         string   `json:"type,omitempty"`
	ContentIndex int      `json:"contentIndex,omitempty"`
	Delta        string   `json:"delta,omitempty"`
	Block        *rawCall `json:"block,omitempty"`
	ToolCall     *rawCall `json:"toolCall,omitempty"`
	// A tool's execution.
	ToolCallID       string          `json:"toolCallId,omitempty"`
	ToolName         string          `json:"toolName,omitempty"`
	Args             json.RawMessage `json:"args,omitempty"`
	ParentToolCallID string          `json:"parentToolCallId,omitempty"`
	Result           json.RawMessage `json:"result,omitempty"`
	IsError          bool            `json:"isError,omitempty"`
	DurationMs       *float64        `json:"durationMs,omitempty"`
	Aborted          bool            `json:"aborted,omitempty"`
}

type rawCall struct {
	Type      string          `json:"type,omitempty"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// rawBlock is a block of an aborted or failed message's content.
type rawBlock struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Thinking string `json:"thinking"`
}

// translator holds what the events said so far. Its exported fields go to
// the next image of the wrapper (handover.go), so the face's items go on.
type translator struct {
	mu  sync.Mutex
	now func() time.Time
	id  func() string
	out []mirror.Event
	// Run names the agent run going on, the face's turn; Pass counts pi's
	// loops in it, a retry or a continuation starting its turns again from
	// 0, and Step is pi's turn in that loop.
	Run  string `json:"run,omitempty"`
	Pass int    `json:"pass,omitempty"`
	Step int    `json:"step,omitempty"`
	// Reached are the calls pi's tool_call hook saw: pi's own checks passed,
	// so one that ends without running was refused by a hook.
	Reached map[string]bool `json:"reached,omitempty"`
	// Faced are the face's prompts handed to pi, not yet heard back as its input.
	Faced []string `json:"faced,omitempty"`
	// Thought are the thinking blocks that streamed text: only they reach
	// the face before their end. Open are the blocks streaming, by their
	// index in the message; Calls the calls whose input streamed and that
	// have not run, and Index the streaming message's calls by index.
	Thought map[string]bool  `json:"thought,omitempty"`
	Open    map[string]int   `json:"open,omitempty"`
	Calls   map[string]*call `json:"calls,omitempty"`
	Index   map[int]string   `json:"index,omitempty"`
	// Seq numbers the calls in the order they streamed.
	Seq int `json:"seq,omitempty"`
	// Failed is the error the latest message ended with; Retries the runs
	// pi started again since a message did not fail, which it tells no
	// extension; Retrying the turn said to be trying again since nothing
	// came back.
	Failed   *string `json:"failed,omitempty"`
	Retries  int     `json:"retries,omitempty"`
	Retrying bool    `json:"retrying,omitempty"`
	// Compaction is the turn a compaction going on is said as: the run's
	// when pi compacts inside one, else one of its own.
	Compaction string `json:"compaction,omitempty"`
	// Queue follows the prompts pi holds until it takes them.
	Queue []queued `json:"queue,omitempty"`
	// Parts is a long event being put together from its parts.
	Parts *parts `json:"parts,omitempty"`
}

// call is a call whose input streams: its native start's arguments, the
// JSON streamed since, when its input was last said, and whether the stream
// lost a piece of it, which leaves its input to its end.
type call struct {
	Name     string          `json:"name"`
	Seq      int             `json:"seq"`
	Start    json.RawMessage `json:"start,omitempty"`
	Streamed string          `json:"streamed,omitempty"`
	Said     time.Time       `json:"said"`
	Changed  bool            `json:"changed,omitempty"`
	Broken   bool            `json:"broken,omitempty"`
	Ended    bool            `json:"ended,omitempty"`
}

// inputEvery is how often a call's input is said while it streams: each
// says the input parsed so far, and a long write streams thousands of deltas.
const inputEvery = 100 * time.Millisecond

// facedKept bounds the face's prompts sent and not yet heard back.
const facedKept = 16

var origins = map[string]string{"interactive": "person", "extension": "extension", "rpc": "rpc"}

func newTranslator() *translator {
	return &translator{now: time.Now, id: control.NewID}
}

// sent is a prompt of the face handed to pi, heard back as its input.
func (t *translator) sent(text string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Faced = append(t.Faced, text)
	if len(t.Faced) > facedKept {
		t.Faced = t.Faced[1:]
	}
}

// take translates a batch of the stream, in order.
func (t *translator) take(events []raw) []mirror.Event {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.out = nil
	for _, e := range events {
		// A lost marker counts the rest of an event it dropped.
		if e.Event == "lost" {
			t.Parts = nil
		} else if e.Event != "part" && t.Parts != nil {
			t.broken()
		}
		switch e.Event {
		case "part":
			if whole, ok := t.part(e); ok {
				t.apply(whole)
			}
		case "lost":
			t.lost(e)
		default:
			t.apply(e)
		}
	}
	t.flush()
	return t.out
}

// lost is what the stream dropped past its bounds: a delta lost leaves the
// calls streaming their input to their end, as a block's end heals its
// text; an event lost the model resyncs.
func (t *translator) lost(e raw) {
	for _, c := range t.Calls {
		c.Broken = true
	}
	if e.Events > 0 {
		t.out = append(t.out, mirror.Event{Kind: mirror.EventResync, Lost: e.Events + e.Deltas})
	}
}

func (t *translator) push(e mirror.Event) {
	// An attempt's first item come back ends its trying again.
	if t.Retrying && (e.Kind == mirror.EventDelta || e.Kind == mirror.EventSettle || e.Kind == mirror.EventTool) {
		t.Retrying = false
		t.out = append(t.out, mirror.Event{Kind: mirror.EventTurn, Turn: t.Run, State: mirror.TurnStateWorking})
	}
	t.out = append(t.out, e)
}

func (t *translator) apply(e raw) {
	nested := e.ParentToolCallID != ""
	switch e.Event {
	case "input":
		t.input(e.Text, e.Source, e.StreamingBehavior != "")
	case "agent_start":
		t.started()
	case "turn_start":
		t.Step = e.TurnIndex
		t.check(pending(e))
	case "message_start":
		if e.Role == "user" {
			t.took(textOf(e.Content))
		}
		t.check(pending(e))
	case "message_update":
		t.streamed(e)
	case "message_end":
		t.ended(e)
	case "tool_execution_start":
		if !nested {
			t.toolStarted(e.ToolCallID, e.ToolName, e.Args)
		}
	case "tool_call":
		if !nested {
			mark(&t.Reached, e.ToolCallID, true)
		}
	case "tool_execution_end":
		if !nested {
			t.toolEnded(e)
		}
	case "session_before_compact":
		t.compacting()
	case "session_compact", "session_compact_failed":
		t.compacted()
	case "agent_settled":
		t.settled(e.Aborted)
	}
}

// pending is pi's queue holding something, as the event read it; an event
// that could not read it is taken to hold something.
func pending(e raw) bool { return e.Pending == nil || *e.Pending }

func (t *translator) input(text, source string, steering bool) {
	origin, ok := origins[source]
	if !ok {
		origin = source
	}
	if source == "extension" && len(t.Faced) > 0 && t.Faced[0] == text {
		t.Faced = t.Faced[1:]
		origin = "face"
	}
	// A prompt steered into the run going on is that run's, and waits in
	// pi's queue until pi takes it.
	prompt := mirror.Event{Kind: mirror.EventPrompt, ID: t.id(), Origin: origin, Text: text}
	if steering {
		prompt.Turn, prompt.Status = t.Run, mirror.ItemStatusQueued
	}
	t.push(prompt)
	if steering {
		t.add(prompt.ID, text)
	}
}

func (t *translator) started() {
	t.Step = 0
	if t.Run != "" {
		if t.Failed != nil {
			t.Retries++
		}
		t.Failed = nil
		t.Pass++
		return
	}
	t.Run, t.Pass = t.id(), 0
	t.push(mirror.Event{Kind: mirror.EventTurn, Turn: t.Run, State: mirror.TurnStateWorking})
}

// settled ends the run; an aborted one (the person's Esc) says so in the
// turn's end, a fact the face draws, not a notice it would have to guess.
func (t *translator) settled(aborted bool) {
	run := t.Run
	t.giveBack()
	// Retries that all failed end as pi's screen ends them.
	if run != "" && !aborted && t.Retries > 0 && t.Failed != nil {
		t.notice(fmt.Sprintf("Error: Retry failed after %d attempts: %s", t.Retries, *t.Failed))
	}
	t.Failed, t.Retries, t.Retrying, t.Run = nil, 0, false, ""
	t.Reached, t.Thought, t.Open, t.Calls, t.Index = nil, nil, nil, nil, nil
	if run != "" {
		end := mirror.Event{Kind: mirror.EventTurn, Turn: run, State: mirror.TurnStateIdle}
		if aborted {
			end.Status = mirror.ItemStatusError
		}
		t.push(end)
	}
}

// compacting is pi beginning a compaction: its own at the threshold, the
// person's /compact or the one main asked for.
func (t *translator) compacting() {
	if t.Compaction != "" {
		return
	}
	t.Compaction = t.Run
	if t.Compaction == "" {
		t.Compaction = t.id()
	}
	t.push(mirror.Event{Kind: mirror.EventTurn, Turn: t.Compaction, State: mirror.TurnStateCompacting})
}

// compacted is its end, done, failed or aborted: the run pi compacted inside
// goes on, and one outside a run leaves pi idle. pi says the end of one it
// refused before beginning, with nothing to compact, which changed nothing.
func (t *translator) compacted() {
	turn := t.Compaction
	if turn == "" {
		return
	}
	t.Compaction = ""
	if t.Run != "" {
		t.push(mirror.Event{Kind: mirror.EventTurn, Turn: t.Run, State: mirror.TurnStateWorking})
		return
	}
	t.push(mirror.Event{Kind: mirror.EventTurn, Turn: turn, State: mirror.TurnStateIdle})
}

func (t *translator) blockID(index int) string {
	run := t.Run
	if run == "" {
		run = "run"
	}
	return run + "." + strconv.Itoa(t.Pass) + "." + strconv.Itoa(t.Step) + "." + strconv.Itoa(index)
}

func (t *translator) settle(id string, block mirror.ItemKind, text string) {
	t.push(mirror.Event{Kind: mirror.EventSettle, ID: id, Turn: t.Run, Block: block, Part: 0, Of: 1, Text: text})
}

// notice says a failure in red under what it stopped.
func (t *translator) notice(text string) {
	t.push(mirror.Event{Kind: mirror.EventNotice, ID: t.id(), Turn: t.Run, Status: mirror.ItemStatusError, Text: text})
}

func mark[K comparable, V any](m *map[K]V, key K, value V) {
	if *m == nil {
		*m = map[K]V{}
	}
	(*m)[key] = value
}
