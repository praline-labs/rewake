package brief

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/role"
)

func TestEveryRoleHasReviewedBriefings(t *testing.T) {
	for _, part := range role.All() {
		for name, render := range map[string]func(Context) string{"intro": Intro} {
			t.Run(part.ID+"/"+name, func(t *testing.T) {
				expected, err := os.ReadFile(filepath.Join("testdata", part.ID+"-"+name+".golden"))
				if err != nil {
					t.Fatal(err)
				}
				got := render(Context{Name: "api", Room: "work", Role: part, Reason: "selected explicitly"}) + "\n"
				if got != string(expected) {
					t.Fatalf("briefing changed:\n%s\nwant:\n%s", got, expected)
				}
			})
		}
	}
}
