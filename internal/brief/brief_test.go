package brief

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/praline-labs/rewake/internal/role"
)

func TestEveryRoleHasReviewedBriefings(t *testing.T) {
	for _, part := range role.All() {
		for _, tool := range []bool{false, true} {
			name := golden(part.ID, tool)
			t.Run(name, func(t *testing.T) {
				expected, err := os.ReadFile(filepath.Join("testdata", name))
				if err != nil {
					t.Fatal(err)
				}
				got := Intro(Context{Name: "api", Room: "work", Role: part, Reason: "selected explicitly", Tool: tool}) + "\n"
				if got != string(expected) {
					t.Fatalf("briefing changed:\n%s\nwant:\n%s", got, expected)
				}
			})
		}
	}
}

// golden names a reviewed briefing: a run without the tool, or with it.
func golden(role string, tool bool) string {
	if tool {
		return fmt.Sprintf("%s-intro-tool.golden", role)
	}
	return fmt.Sprintf("%s-intro.golden", role)
}
