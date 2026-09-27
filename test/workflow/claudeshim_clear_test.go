package workflow

// The conversation this column's session is in, and the /clear that starts a
// new one.
//
// The real harness names its conversation in every hook and status line as
// session_id, and /clear replaces it (docs/research.md). rewake pins a
// delivery to the one its collector heard last and compares that with the
// session_id of the turn end that reports on it, so the fixture has to name
// one conversation consistently — a status line naming another would read as
// a /clear nobody made — and change it only when a /clear is played.

import (
	"encoding/json"
	"os"
	"sync"
)

// shimClearBeforeTurn makes the session start a new conversation, as /clear
// does, after its first task was delivered and before the turn that works it.
const shimClearBeforeTurn = "RW_SHIM_CLEAR_BEFORE_TURN"

// clearedSuffix marks the conversation a /clear started.
const clearedSuffix = "-cleared"

var shimConversationState struct {
	sync.Mutex
	cleared bool
	once    sync.Once
	// resumed is the conversation a --resume named (claudeshim_resume_test.go).
	resumed string
}

// shimConversation is the session_id the session's hooks and status line
// carry now: the one a --resume named, or else the session's name, until a
// /clear, then a new one.
func shimConversation() string {
	shimConversationState.Lock()
	defer shimConversationState.Unlock()
	conversation := os.Getenv(sessionNameEnv)
	if shimConversationState.resumed != "" {
		conversation = shimConversationState.resumed
	}
	if shimConversationState.cleared {
		return conversation + clearedSuffix
	}
	return conversation
}

// clearIfAsked plays /clear once, when the switch asks for it: the harness
// starts the session again with source "clear" and the new session_id, and
// every event after it carries that id.
func (s *claudeSession) clearIfAsked() {
	if os.Getenv(shimClearBeforeTurn) == "" {
		return
	}
	shimConversationState.once.Do(func() {
		shimConversationState.Lock()
		shimConversationState.cleared = true
		shimConversationState.Unlock()
		if payload, err := json.Marshal(map[string]any{
			"hook_event_name": "SessionStart", "session_id": shimConversation(),
			"cwd": workingDirectory(), "source": "clear",
		}); err == nil {
			s.runHook("SessionStart", s.launch.settings.observe, payload)
		}
	})
}
