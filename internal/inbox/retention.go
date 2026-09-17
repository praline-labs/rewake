package inbox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/state"
)

type answerLifetime struct {
	ReleasedAt time.Time `json:"releasedAt"`
}

func retentionPath(dir, name string) string {
	return filepath.Join(state.InboxPath(dir, name), "retention")
}

// Only a real reservation earns a new delivery window. Its release is durable
// and recorded once, so retries and server restarts cannot renew it forever.
func (s *Server) answerExpired(message Message, reserved bool) (bool, error) {
	path := filepath.Join(retentionPath(s.Dir, s.Name), message.ID)
	raw, err := os.ReadFile(path)
	var lifetime answerLifetime
	if err == nil {
		if err := json.Unmarshal(raw, &lifetime); err != nil {
			return false, err
		}
	} else if !os.IsNotExist(err) {
		return false, err
	}
	if reserved && os.IsNotExist(err) {
		if err := state.EnsureSubdir(retentionPath(s.Dir, s.Name)); err != nil {
			return false, err
		}
		return false, state.WriteAtomic(path, []byte(`{"releasedAt":"0001-01-01T00:00:00Z"}`))
	}
	if reserved {
		return false, nil
	}
	if os.IsNotExist(err) {
		return s.expired(message), nil
	}
	if lifetime.ReleasedAt.IsZero() {
		lifetime.ReleasedAt = time.Now()
		encoded, err := json.Marshal(lifetime)
		if err != nil {
			return false, err
		}
		if err := state.WriteAtomic(path, encoded); err != nil {
			return false, err
		}
		unread := filepath.Join(state.UnreadPath(s.Dir, s.Name), message.ID+".json")
		_ = os.Chtimes(unread, lifetime.ReleasedAt, lifetime.ReleasedAt)
	}
	return s.expired(Message{CreatedAt: lifetime.ReleasedAt}), nil
}

func retainedReceipts(dir, name string) map[string]bool {
	keep := map[string]bool{}
	for _, directory := range []string{state.InboxPath(dir, name), state.UnreadPath(dir, name), state.DonePath(dir, name)} {
		messages, _ := listIn(directory)
		for _, message := range messages {
			for _, id := range message.InReplyTo {
				keep[id] = true
			}
		}
	}
	return keep
}
