package brief

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/praline-labs/rewake/internal/role"
)

// Regenerates the reviewed briefings. Run deliberately, never in an ordinary
// check: the golden files exist so that a change to what a session is told is
// looked at by a person.
func TestRegenerateGolden(t *testing.T) {
	if os.Getenv("REWAKE_REGENERATE") == "" {
		t.Skip("set REWAKE_REGENERATE to rewrite the reviewed briefings")
	}
	for _, part := range role.All() {
		text := Intro(Context{Name: "api", Room: "work", Role: part, Reason: "selected explicitly"}) + "\n"
		if err := os.WriteFile(filepath.Join("testdata", part.ID+"-intro.golden"), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
