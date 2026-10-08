package workflow

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"regexp"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/praline-labs/rewake/internal/bridge"
)

// The fixture program's side of the mail tool (docs/v2/stage3-fixture.md#the-tool-transport):
// it registers the tools the probe offers, and in its first turn makes the
// calls a scenario asks for as a harness would — the native call reported to
// the adapter, the request to the wrapper's endpoint on a connection of its
// own, and the result it would hand the model reported back. It runs nothing
// itself: the endpoint runs the words in a child of the wrapper's image.

const (
	// shimNoTool leaves the tool transport out of the hello.
	shimNoTool = "RW_SHIM_NO_TOOL"
	// shimReusedTurns withholds the declaration that the program's turn ids
	// are never reused.
	shimReusedTurns = "RW_SHIM_REUSED_TURNS"
	// shimToolCalls is a JSON list of the calls the first turn makes through
	// the tool, each {"tool": ..., "arguments": {...}}. An argument
	// "{receipt}" is replaced by the receipt an earlier answer named, and
	// "awaitRead" on an inbox call holds the turn until its read commits.
	shimToolCalls = "RW_SHIM_TOOL_CALLS"
)

// toolEventKind marks a tool call's line in the turn log: the turn, then the
// call's record as JSON.
const toolEventKind = "tool"

// fixtureTools is what the probe offered and the program registered.
type fixtureTools struct {
	mu       sync.Mutex
	offered  []bridge.ToolDescriptor
	endpoint string
	made     bool
}

// fixtureToolCall is one call a scenario asks for.
type fixtureToolCall struct {
	Tool      string         `json:"tool"`
	Arguments map[string]any `json:"arguments,omitempty"`
	// AwaitRead is a scenario's choice of schedule: the turn goes on only
	// once the read this call showed has committed, or toolAckBound passed.
	AwaitRead bool `json:"awaitRead,omitempty"`
}

// fixtureToolRecord is what the turn log keeps of a call: what was asked,
// and what came back to hand the model, or why nothing did.
type fixtureToolRecord struct {
	Call      string         `json:"call"`
	Tool      string         `json:"tool"`
	Arguments map[string]any `json:"arguments,omitempty"`
	Texts     []string       `json:"texts,omitempty"`
	IsError   bool           `json:"isError,omitempty"`
	Error     string         `json:"error,omitempty"`
}

// registerTools answers the tool transport's probe: the names registered,
// in the order offered, and the digest of the whole set.
func (s *fixtureSession) registerTools(frame, reply fixtureFrame) fixtureFrame {
	s.tools.mu.Lock()
	defer s.tools.mu.Unlock()
	s.tools.offered, s.tools.endpoint = frame.Tools, frame.Endpoint
	reply.Names = make([]string, 0, len(frame.Tools))
	for _, tool := range frame.Tools {
		reply.Names = append(reply.Names, tool.Name)
	}
	reply.Digest, reply.State = bridge.DescriptorsDigest(frame.Tools), "proven"
	return reply
}

var (
	toolReceipt = regexp.MustCompile(`"receipt":\s*"([0-9a-f]+)"`)
	toolLetter  = regexp.MustCompile(`Rewake: (\d+-[0-9a-f]+), part \d+ of \d+`)
)

// toolAckBound bounds the wait a scenario asks for with AwaitRead. The read
// counts only once its acknowledgment is in, and an end captured before it
// leaves the letters unread (T7, T8): that order is the rule, held in
// test/toolrig, and a scenario that judges the read's commit asks for the other
// order explicitly and checks, from the event this records, that it got it.
const toolAckBound = 10 * time.Second

// toolAckEventKind marks whether the schedule a scenario asked for held: the
// read committed before the turn went on.
const toolAckEventKind = "tool-acknowledged"

// toolCalls are the calls shimToolCalls asks for, taken once: the first turn
// makes them.
func (s *fixtureSession) toolCalls() []fixtureToolCall {
	var calls []fixtureToolCall
	if raw := os.Getenv(shimToolCalls); raw == "" || json.Unmarshal([]byte(raw), &calls) != nil {
		return nil
	}
	s.tools.mu.Lock()
	defer s.tools.mu.Unlock()
	if s.tools.made {
		return nil
	}
	s.tools.made = true
	return calls
}

// readsThroughTool says whether the turn reads its mail through the tool
// rather than the shell.
func readsThroughTool(calls []fixtureToolCall) bool {
	for _, call := range calls {
		if call.Tool == "inbox" {
			return true
		}
	}
	return false
}

// callTools makes the calls in the turn and answers what an inbox call read.
// An inbox call that asks for it waits for its letters to leave the unread
// overview before the next call, so the turn's end comes after its
// acknowledgment.
func (s *fixtureSession) callTools(turn string, calls []fixtureToolCall) string {
	read, receipt := "", ""
	for i, call := range calls {
		arguments := map[string]any{}
		for name, value := range call.Arguments {
			if value == "{receipt}" {
				value = receipt
			}
			arguments[name] = value
		}
		record := s.callTool(turn, turn+"-call-"+strconv.Itoa(i+1), call.Tool, arguments)
		encoded, _ := json.Marshal(record)
		s.recordTurnEvent(toolEventKind, turn, string(encoded))
		text := record.answer()
		if found := toolReceipt.FindStringSubmatch(text); found != nil {
			receipt = found[1]
		}
		if call.Tool == "inbox" {
			read = "read through the tool: " + text
			if call.AwaitRead {
				s.recordTurnEvent(toolAckEventKind, turn, strconv.FormatBool(s.awaitAcknowledged(text)))
			}
		}
	}
	return read
}

// awaitAcknowledged waits, within toolAckBound, until no letter a tool's
// read showed is unread any more; false when none was named or one stayed.
func (s *fixtureSession) awaitAcknowledged(answer string) bool {
	var shown []string
	for _, found := range toolLetter.FindAllStringSubmatch(answer, -1) {
		shown = append(shown, found[1])
	}
	// No letter named in the answer is nothing to wait for: the schedule
	// asked for was not established.
	if len(shown) == 0 {
		return false
	}
	for until := time.Now().Add(toolAckBound); time.Now().Before(until); time.Sleep(50 * time.Millisecond) {
		unread, err := s.peekOverview()
		if err == nil && !slices.ContainsFunc(shown, func(id string) bool { return slices.Contains(unread, id) }) {
			return true
		}
	}
	return false
}

// callTool makes one call: reported, requested, its result reported.
func (s *fixtureSession) callTool(turn, id, tool string, arguments map[string]any) fixtureToolRecord {
	record := fixtureToolRecord{Call: id, Tool: tool, Arguments: arguments}
	encoded, _ := json.Marshal(arguments)
	if answer, err := s.ask(fixtureFrame{Op: "tool-call", Call: id, Turn: turn, Tool: tool, Arguments: encoded}, fixtureAskBound); err != nil || !answer.OK {
		record.Error = "the call's report: " + refusal(answer, err)
		return record
	}
	texts, isError, err := s.requestCall(turn, id, tool, encoded)
	if err != nil {
		record.Error = err.Error()
		return record
	}
	record.Texts, record.IsError = texts, isError
	if answer, err := s.ask(fixtureFrame{Op: "tool-result", Call: id, Texts: texts, IsError: isError}, fixtureAskBound); err != nil || !answer.OK {
		record.Error = "the result's report: " + refusal(answer, err)
	}
	return record
}

// requestCall asks the endpoint to run one call, on a connection of its own,
// as the endpoint's transport role takes it.
func (s *fixtureSession) requestCall(turn, id, tool string, arguments json.RawMessage) ([]string, bool, error) {
	s.tools.mu.Lock()
	path := s.tools.endpoint
	s.tools.mu.Unlock()
	if path == "" {
		return nil, false, fmt.Errorf("no endpoint was offered")
	}
	conn, err := net.Dial("unix", path)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(fixtureAskBound))
	reader := bufio.NewReaderSize(conn, 1<<16)
	if _, err := conn.Write([]byte(`{"role":"transport"}` + "\n")); err != nil {
		return nil, false, err
	}
	var greeted struct {
		Error string `json:"error"`
	}
	if line, err := reader.ReadBytes('\n'); err != nil || json.Unmarshal(line, &greeted) != nil || greeted.Error != "" {
		return nil, false, fmt.Errorf("the endpoint's hello: %v %s", err, greeted.Error)
	}
	call := map[string]any{
		"tool": tool, "callId": id, "conversation": fixtureThread, "turn": turn,
		"turnsNeverReused": os.Getenv(shimReusedTurns) == "", "arguments": arguments,
	}
	request, _ := json.Marshal(map[string]any{"id": 1, "op": "call", "call": call})
	if _, err := conn.Write(append(request, '\n')); err != nil {
		return nil, false, err
	}
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return nil, false, fmt.Errorf("no answer: %w", err)
	}
	var answered struct {
		Error  string `json:"error"`
		Answer *struct {
			Texts   []string `json:"texts"`
			IsError bool     `json:"isError"`
		} `json:"answer"`
	}
	if json.Unmarshal(line, &answered) != nil || answered.Answer == nil {
		return nil, false, fmt.Errorf("the endpoint: %s", answered.Error)
	}
	return answered.Answer.Texts, answered.Answer.IsError, nil
}
