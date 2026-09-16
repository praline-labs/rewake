package cli

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// maxPayload bounds what is read from a hook. The payload is a small JSON
// object; anything much larger is not one.
const maxPayload = 4 << 20

// silentEnd is the text of a finished notice when the turn ended without a reply.
const silentEnd = "(the turn ended without a final message)"

// handleTurnEnded is called by a harness at the end of every turn of its
// session. It tells each session whose message was read during that turn that
// the turn is over, and passes the last reply along.
//
// It never fails loudly. It runs inside the harness's own machinery — a Claude
// Code Stop hook, a Codex notify program — where an error is at best noise on
// the screen and at worst a turn that will not end.
func handleTurnEnded(ctx *Context, call Call) error {
	name := os.Getenv(state.SessionEnv)
	dir, err := state.Dir()
	if name == "" || err != nil {
		return nil
	}

	var payload []byte
	if len(call.Positionals) > 0 {
		// Codex passes the payload as the last argument.
		payload = []byte(call.Positionals[len(call.Positionals)-1])
	} else if !isTerminal(os.Stdin) {
		// A Claude Code hook gets it on stdin.
		payload, _ = io.ReadAll(io.LimitReader(os.Stdin, maxPayload))
	}
	reply, ok := lastReply(payload)
	if !ok {
		return nil
	}
	if strings.TrimSpace(reply) == "" {
		reply = silentEnd
	}

	for _, peer := range inbox.TakeAwaiting(dir, name) {
		session, err := registry.Lookup(dir, peer)
		if errors.Is(err, registry.ErrNotFound) || err != nil {
			continue
		}
		_ = inbox.Put(dir, inbox.Message{
			ID:        inbox.NewID(),
			From:      name,
			To:        session.Name,
			ToEpoch:   session.Epoch(),
			Kind:      inbox.Finished,
			Text:      reply,
			CreatedAt: time.Now(),
		})
	}
	return nil
}

// lastReply reads the last reply of the turn from a hook payload. The two
// harnesses name the field differently, and Codex also calls its program for
// events that are not the end of a turn.
func lastReply(payload []byte) (string, bool) {
	var fields map[string]any
	if err := json.Unmarshal(payload, &fields); err != nil {
		return "", false
	}
	if event, present := fields["type"]; present && event != "agent-turn-complete" {
		return "", false
	}
	for _, key := range []string{"last_assistant_message", "last-assistant-message"} {
		if text, ok := fields[key].(string); ok {
			return text, true
		}
	}
	return "", true
}

// isTerminal reports whether a file is a character device, which is what an
// interactive stdin is. Reading from one would wait for a person.
func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
