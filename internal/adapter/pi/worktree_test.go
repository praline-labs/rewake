package pi

import "testing"

func TestWorktreeNamesNewConversationButDoesNotResume(t *testing.T) {
	h := harness{}
	for _, args := range [][]string{
		{"--session-id", "fresh"},
		{"--session-id=fresh"},
		{"--", "--continue"},
	} {
		if err := h.WorktreeRefusal(args); err != nil {
			t.Errorf("fresh conversation %v refused: %v", args, err)
		}
	}
	for _, args := range [][]string{
		{"--continue"},
		{"-c"},
		{"--resume"},
		{"-r"},
		{"--session", "existing"},
		{"--session=existing"},
		{"--fork", "existing"},
	} {
		if err := h.WorktreeRefusal(args); err == nil {
			t.Errorf("existing conversation %v accepted", args)
		}
	}
}
