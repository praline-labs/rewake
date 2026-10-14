package claude

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
)

// rewake's id names a new conversation, so a launch whose arguments already
// name one or continue one is refused rather than recorded under an id the
// harness never used.
func TestAConversationIdIsRefusedBesideOneTheArgumentsName(t *testing.T) {
	for _, args := range [][]string{
		{"--session-id", "1b0c7c43-5a8e-4c56-8a3f-2d0f6c1e9a77"},
		{"--session-id=1b0c7c43-5a8e-4c56-8a3f-2d0f6c1e9a77"},
		{"--resume", "x"},
		{"-c"},
		{"--fork-session"},
	} {
		dir := t.TempDir()
		request := adapter.LaunchRequest{
			Name: "api", Dir: dir, Socket: filepath.Join(dir, "a.sock"), Args: args,
			Conversation: "0f6f2d55-1c3a-4c43-9a11-6c1f4bd1e0c2",
		}
		if _, err := New().Launch(request); err == nil || !strings.Contains(err.Error(), "conversation") {
			t.Errorf("%q beside rewake's id was not refused for it: %v", args, err)
		}
	}
}
