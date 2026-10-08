package endpoint

import (
	"encoding/json"

	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/harness"
)

// The parser of 1.x that is left: the hook record of a call, decoded and
// handed to the neutral input (input.go), and of its result, into the result
// a read's acknowledgment needs (docs/mail-bridge-turns.md#acknowledging-a-read).
// It leaves with the hooks in S9; the helpers below it decode a call's words
// and result for any caller.

// The tool as the hook names it.
const (
	toolName       = "rewake"
	claudeToolName = "mcp__rewake__rewake"
)

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

// hook takes what a hook was handed. The hook has exited only
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
