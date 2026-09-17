package codex

import (
	"context"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
)

func (s *serverSession) Deliver(ctx context.Context, message inbox.Message) inbox.Result {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := s.ensureThread(ctx); err != nil {
		return inbox.Result{State: inbox.Failed, Detail: "app-server has no ready TUI thread: " + err.Error()}
	}
	for {
		s.mu.Lock()
		client, thread, changed := s.client, s.current, s.changed
		s.mu.Unlock()
		if client != nil && thread != "" {
			if message.DeliveryThread != "" && message.DeliveryThread != thread {
				return inbox.Result{State: inbox.Failed, Detail: "the conversation changed before delivery; read the task and resend if it still applies"}
			}
			input := []map[string]string{{"type": "text", "text": noticePrefix(message) + " " + harness.Notice(message)}}
			var result struct {
				Turn struct {
					ID string `json:"id"`
				} `json:"turn"`
			}
			err := client.call(ctx, "turn/start", map[string]any{"threadId": thread, "clientUserMessageId": message.ID, "input": input}, &result)
			if err != nil {
				return inbox.Result{State: inbox.Failed, Detail: err.Error() + "; delivery was not retried automatically"}
			}
			s.mu.Lock()
			stillCurrent := s.current == thread
			s.mu.Unlock()
			if !stillCurrent {
				return inbox.Result{State: inbox.Failed, Detail: "the conversation changed during delivery; the notice went to the previous thread"}
			}
			if result.Turn.ID == "" {
				return inbox.Result{State: inbox.Failed, Detail: "app-server returned no turn id"}
			}
			return inbox.Result{State: inbox.Delivered, Via: "app-server"}
		}
		select {
		case <-ctx.Done():
			return inbox.Result{State: inbox.Failed, Detail: "app-server has no ready TUI thread: " + ctx.Err().Error()}
		case <-s.exited:
			return inbox.Result{State: inbox.Failed, Detail: "session ended"}
		case <-changed:
		}
	}
}
