package endpoint

import (
	"encoding/json"
	"strings"

	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/channel"
	"github.com/praline-labs/rewake/internal/receipt"
	"github.com/praline-labs/rewake/internal/state"
)

// The adapters: each harness's own record of a call, read into an
// observation, and of its result, into the evidence a read's acknowledgment
// needs (docs/mail-bridge-turns.md#acknowledging-a-read).

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
		e.turnStarted(params.Turn.ID)
	case "turn/completed":
		e.turnEnded(params.Turn.ID)
	case serverStatus:
		// The status's own text is dropped: the class is the endpoint's.
		if params.Name == toolName && params.Status == "failed" {
			e.tell(channel.Event{Kind: channel.StartupFailed})
		}
	case "item/started", "item/completed":
		item := params.Item
		if item.Kind != "mcpToolCall" || item.Server != toolName || item.Tool != toolName {
			return
		}
		if message.Method == "item/started" {
			digest, refusal := e.observedWords(wordsOf(item.Arguments))
			e.observe(item.ID, observation{conversation: params.Thread, turn: params.TurnID, digest: digest, refusal: refusal})
			return
		}
		succeeded := item.Status == "completed" && item.Result != nil && !item.Result.IsError && isNull(item.Error)
		var content json.RawMessage
		if item.Result != nil {
			content = item.Result.Content
		}
		e.complete(item.ID, exposureOf(item.ID, content, succeeded, true))
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
		digest, refusal := e.observedWords(wordsOf(input.Input))
		if nested {
			refusal = "a nested agent's call does not run through the tool"
		}
		e.observe(input.UseID, observation{conversation: input.Session, prompt: input.Prompt, digest: digest, refusal: refusal, limits: limits})
	case "PostToolUse":
		evidence := exposureOf(input.UseID, input.Response, true, !nested)
		if !e.limitsAllow(input.UseID, limits) {
			// Which limit the harness applied to the result is unknown: the
			// call is completed, and acknowledges nothing.
			evidence = bridge.Exposure{}
		}
		e.complete(input.UseID, evidence)
	}
}

// wordsOf reads a call's arguments: one key, words, an array of strings.
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

// exposureOf reads the result a harness recorded: a list of content items,
// of which the first, a text, is the answer. Anything else — a string where
// the harness kept the result in a file, an image, nothing — is a result
// shortened or replaced, which proves no exposure.
func exposureOf(id string, content json.RawMessage, succeeded, direct bool) bridge.Exposure {
	evidence := bridge.Exposure{CallID: id, Direct: direct, Succeeded: succeeded}
	var items []struct {
		Kind string  `json:"type"`
		Text *string `json:"text"`
	}
	if json.Unmarshal(content, &items) != nil || len(items) == 0 {
		evidence.Shortened = true
		return evidence
	}
	texts := make([]string, 0, len(items))
	for _, item := range items {
		if item.Kind != "text" || item.Text == nil {
			evidence.Shortened = true
			return evidence
		}
		texts = append(texts, *item.Text)
	}
	evidence.Answer = []byte(texts[0])
	evidence.ResultBytes = bridge.EncodedSize(texts[0], strings.Join(texts[1:], ""))
	return evidence
}

// complete hands a call's result to the acknowledgment, which runs on its
// own: the event's reader never waits for it. Only a call whose ticket a
// child used can have shown anything, and only its first result is handled:
// the attempt is spent before anything is read or waited for, so a repeat
// cannot retry an acknowledgment the first one dropped
// (docs/mail-bridge-turns.md#acknowledging-a-read).
func (e *Endpoint) complete(id string, evidence bridge.Exposure) {
	c := e.calls
	c.mu.Lock()
	entry := c.byCall[id]
	if entry == nil || entry.completed {
		c.mu.Unlock()
		return
	}
	entry.completed = true
	var ticket bridge.Ticket
	used := entry.issued != nil && entry.issued.used
	if used {
		ticket = entry.issued.ticket
	}
	c.mu.Unlock()
	if !used || e.cfg.Acknowledge == nil {
		return
	}
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return
	}
	e.acks.Add(1)
	e.mu.Unlock()
	go func() {
		defer e.acks.Done()
		token, err := receipt.Bound(e.cfg.Dir, e.cfg.Name, e.cfg.Epoch, bridge.CallKey(ticket.Transport, ticket.Conversation, ticket.CallID))
		if err != nil {
			// Missing or unreadable: which read the call showed is
			// unknown, and a later call shows the letter under its own.
			return
		}
		state.Step("acknowledge")
		_ = e.cfg.Acknowledge(e.cfg.Dir, e.cfg.Name, e.cfg.Epoch, token, evidence, e.cfg.Gate)
	}()
}
