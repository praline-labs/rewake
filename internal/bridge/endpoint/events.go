package endpoint

import (
	"encoding/json"

	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/channel"
	"github.com/praline-labs/rewake/internal/harness"
)

// The two parsers of 1.x: each harness's own record of a call, decoded and
// handed to the neutral input (input.go), and of its result, into the result a
// read's acknowledgment needs (docs/mail-bridge-turns.md#acknowledging-a-read).
// Each leaves with its adapter, in S8 and S9.

// The tool as each harness names it.
const (
	toolName       = "rewake"
	serverStatus   = "mcpServer/startupStatus/updated"
	claudeToolName = "mcp__rewake__rewake"
)

// codexMessage is the part of a Codex server notification the adapter reads.
type codexMessage struct {
	Method string `json:"method"`
	Params struct {
		Thread string `json:"threadId"`
		TurnID string `json:"turnId"`
		// Name and Status are an MCP server's startup status.
		Name   string `json:"name"`
		Status string `json:"status"`
		Turn   struct {
			ID string `json:"id"`
		} `json:"turn"`
		Item struct {
			Kind      string          `json:"type"`
			ID        string          `json:"id"`
			Server    string          `json:"server"`
			Tool      string          `json:"tool"`
			Status    string          `json:"status"`
			Arguments json.RawMessage `json:"arguments"`
			Result    *struct {
				Content json.RawMessage `json:"content"`
				IsError bool            `json:"isError"`
			} `json:"result"`
			Error json.RawMessage `json:"error"`
		} `json:"item"`
	} `json:"params"`
}

// CodexEvent takes one server notification of the run's primary thread, as
// the gateway read it, or an MCP server's startup status of any thread. It does no filesystem work and never waits: the
// gateway's reader goes on at once.
func (e *Endpoint) CodexEvent(raw []byte) {
	var message codexMessage
	if json.Unmarshal(raw, &message) != nil {
		return
	}
	params := message.Params
	switch message.Method {
	case "turn/started":
		// The event carries no time of the turn's own, and the time it
		// arrived may be later than a command the turn ran: none is given.
		e.TurnStarted(params.Thread, params.Turn.ID, 0)
	case "turn/completed":
		e.TurnEnded(params.Thread, params.Turn.ID)
	case serverStatus:
		// The status's own text is dropped. Its thread, absent or not,
		// decides whose channel it fails.
		if params.Name == toolName && params.Status == "failed" {
			e.StartupFailed(params.Thread)
		}
	case "item/started", "item/completed":
		item := params.Item
		if item.Kind != "mcpToolCall" || item.Server != toolName || item.Tool != toolName {
			return
		}
		if message.Method == "item/started" {
			words, _ := wordsOf(item.Arguments)
			e.CallSeen(harness.ObservedCall{ID: item.ID, Conversation: params.Thread, Turn: params.TurnID, Words: words})
			return
		}
		succeeded := item.Status == "completed" && item.Result != nil && !item.Result.IsError && isNull(item.Error)
		var content json.RawMessage
		if item.Result != nil {
			content = item.Result.Content
		}
		e.CallResult(item.ID, resultOf(content, succeeded, true))
	}
}

// claudeHook is the part of a Claude Code hook input the adapter reads.
type claudeHook struct {
	Session  string          `json:"session_id"`
	Prompt   string          `json:"prompt_id"`
	Event    string          `json:"hook_event_name"`
	Tool     string          `json:"tool_name"`
	Input    json.RawMessage `json:"tool_input"`
	Response json.RawMessage `json:"tool_response"`
	UseID    string          `json:"tool_use_id"`
	Server   *struct {
		Name string `json:"name"`
	} `json:"mcp_server"`
	AgentID   string `json:"agent_id"`
	AgentType string `json:"agent_type"`
}

// hook takes what `rewake bridge-hook` was handed. The hook has exited only
// once this returns, and the harness runs the call only after the hook
// exits: so the observation is in place before the call reaches the server.
func (e *Endpoint) hook(payload []byte) { e.hookWith(payload, nil) }

// hookWith is hook with the limits the hook saw, nil from one that sent
// none.
func (e *Endpoint) hookWith(payload []byte, limits *HookLimits) {
	var input claudeHook
	if json.Unmarshal(payload, &input) != nil || input.Tool != claudeToolName || input.Server != nil && input.Server.Name != toolName {
		return
	}
	nested := input.AgentID != "" || input.AgentType != ""
	switch input.Event {
	case "PreToolUse":
		// Claude Code starts its server on demand: a call seen while none
		// lives opens the hello timer.
		e.tell(channel.Event{Kind: channel.CallSeen})
		e.promptSeen(input.Prompt)
		words, _ := wordsOf(input.Input)
		e.callSeen(harness.ObservedCall{ID: input.UseID, Conversation: input.Session, Words: words, Nested: nested}, input.Prompt, limits)
	case "PostToolUse":
		result := resultOf(input.Response, true, !nested)
		if !e.limitsAllow(input.UseID, limits) {
			// Which limit the harness applied to the result is unknown: the
			// call is completed, and acknowledges nothing.
			result = harness.ToolResult{}
		}
		e.CallResult(input.UseID, result)
	}
}

// wordsOf reads a call's arguments: one key, words, an array of strings; nil
// for anything else.
func wordsOf(arguments json.RawMessage) ([]string, bool) {
	var shape map[string]json.RawMessage
	if json.Unmarshal(arguments, &shape) != nil || len(shape) != 1 {
		return nil, false
	}
	var words []string
	if json.Unmarshal(shape["words"], &words) != nil || words == nil {
		return nil, false
	}
	return words, true
}

func isNull(raw json.RawMessage) bool {
	return len(raw) == 0 || string(raw) == "null"
}

// resultOf reads the result an MCP harness recorded: a list of content items,
// all text. Anything else — a string where the harness kept the result in a
// file, an image, nothing — is a result shortened or replaced.
func resultOf(content json.RawMessage, succeeded, direct bool) harness.ToolResult {
	result := harness.ToolResult{Succeeded: succeeded, Direct: direct}
	var items []struct {
		Kind string  `json:"type"`
		Text *string `json:"text"`
	}
	if json.Unmarshal(content, &items) != nil || len(items) == 0 {
		result.Shortened = true
		return result
	}
	for _, item := range items {
		if item.Kind != "text" || item.Text == nil {
			return harness.ToolResult{Succeeded: succeeded, Direct: direct, Shortened: true}
		}
		result.Texts = append(result.Texts, *item.Text)
	}
	return result
}

// exposureOf is the evidence an MCP result proves: its decoding, then the
// neutral reading of the whole result and its size.
func exposureOf(id string, content json.RawMessage, succeeded, direct bool) bridge.Exposure {
	return exposure(id, resultOf(content, succeeded, direct))
}
