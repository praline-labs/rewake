package codex

import (
	"context"
	"encoding/json"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
)

type serverThread struct {
	ID           string          `json:"id"`
	Source       json.RawMessage `json:"source"`
	Originator   string          `json:"originator"`
	Parent       *string         `json:"parentThreadId"`
	ThreadSource string          `json:"threadSource"`
}

func tuiThread(thread serverThread) bool {
	var source string
	if json.Unmarshal(thread.Source, &source) != nil {
		return false
	}
	if thread.Parent != nil || thread.ThreadSource != "" && thread.ThreadSource != "user" {
		return false
	}
	if source != "cli" && source != "vscode" {
		return false
	}
	switch thread.Originator {
	case "rewake", "codex-tui", "codex_cli_rs":
		return true
	}
	return false
}

type serverItem struct {
	Kind string `json:"type"`
	Text string `json:"text"`
}

func (s *serverSession) event(method string, raw json.RawMessage) {
	var params struct {
		Thread   serverThread `json:"thread"`
		ThreadID string       `json:"threadId"`
		TurnID   string       `json:"turnId"`
		Item     serverItem   `json:"item"`
		Turn     struct {
			ID     string       `json:"id"`
			Status string       `json:"status"`
			Items  []serverItem `json:"items"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		} `json:"turn"`
	}
	switch method {
	case "thread/started", "thread/closed", "turn/completed", "item/completed":
	default:
		return
	}
	if json.Unmarshal(raw, &params) != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if method == "thread/started" {
		if tuiThread(params.Thread) {
			s.generation++
			if s.current != params.Thread.ID {
				s.messages = make(map[string]string)
			}
			s.current = params.Thread.ID
			s.signal()
		}
		return
	}
	if params.ThreadID != s.current || s.current == "" {
		return
	}
	if method == "thread/closed" {
		s.generation++
		s.current = ""
		s.signal()
		return
	}
	if method == "item/completed" {
		if params.Item.Kind == "agentMessage" {
			s.messages[params.ThreadID+"/"+params.TurnID] = params.Item.Text
		}
		return
	}
	if params.Turn.ID == "" {
		return
	}
	result := harness.Completion{ID: params.ThreadID + "/" + params.Turn.ID, Thread: params.ThreadID}
	switch params.Turn.Status {
	case "completed":
		result.Kind = inbox.Finished
		result.Text = s.messages[result.ID]
		for _, item := range params.Turn.Items {
			if item.Kind == "agentMessage" {
				result.Text = item.Text
			}
		}
	case "failed":
		result.Kind = inbox.Error
		if params.Turn.Error != nil {
			result.Text = params.Turn.Error.Message
		}
	case "interrupted":
		result.Kind = inbox.Stopped
		result.Text = "the person at the keyboard stopped this turn"
	default:
		return
	}
	delete(s.messages, result.ID)
	s.outcomes = append(s.outcomes, result)
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *serverSession) report(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.wake:
		}
		for {
			s.mu.Lock()
			if len(s.outcomes) == 0 {
				s.mu.Unlock()
				break
			}
			result := s.outcomes[0]
			s.mu.Unlock()
			if s.emit != nil {
				if err := s.emit(result); err != nil {
					select {
					case <-ctx.Done():
						return
					case <-time.After(250 * time.Millisecond):
						continue
					}
				}
			}
			s.mu.Lock()
			s.outcomes = s.outcomes[1:]
			s.mu.Unlock()
		}
	}
}
