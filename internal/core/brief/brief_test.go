package brief

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/praline-labs/rewake/internal/core/role"
)

func TestEveryRoleHasReviewedBriefings(t *testing.T) {
	for _, part := range role.All() {
		name := golden(part.ID)
		t.Run(name, func(t *testing.T) {
			expected, err := os.ReadFile(filepath.Join("testdata", name))
			if err != nil {
				t.Fatal(err)
			}
			got := Intro(Context{Name: "api", Room: "work", Role: part, Reason: "selected explicitly"}) + "\n"
			if got != string(expected) {
				t.Fatalf("briefing changed:\n%s\nwant:\n%s", got, expected)
			}
		})
	}
}

// golden names a reviewed briefing.
func golden(role string) string { return role + "-intro.golden" }
