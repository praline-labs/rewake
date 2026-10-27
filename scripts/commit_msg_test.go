package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const trailers = "Co-Authored-By: Codex (gpt-9) <noreply@openai.com>\nCo-Authored-By: Claude Whatever 9 <noreply@anthropic.com>\n"

func TestCommitMessageCheck(t *testing.T) {
	cases := []struct {
		name, message, refusal string // refusal is empty when accepted
	}{
		{"good", "Add a thing\n\n" + trailers, ""},
		{"other model names", "Add a thing\n\nCo-Authored-By: Codex (x) <a@b.c>\nco-authored-by: Claude Q <a@b.c>\n", ""},
		{"git comments ignored", "Add a thing\n\n" + trailers + "# Please enter\n", ""},
		{"merge commit", "Merge branch 'x'\n\n" + trailers, ""},
		{"merge commit without trailers", "Merge branch 'x'\n", "Codex"},
		{"no Codex", "Add a thing\n\nCo-Authored-By: Claude Q <noreply@anthropic.com>\n", "Codex"},
		{"no Claude", "Add a thing\n\nCo-Authored-By: Codex (x) <noreply@openai.com>\n", "Claude"},
		{"body text", "Add a thing\n\nBecause it is needed.\n\n" + trailers, "Because it is needed."},
		{"empty subject", "\n\n" + trailers, "subject"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "COMMIT_EDITMSG")
			if err := os.WriteFile(file, []byte(c.message), 0o644); err != nil {
				t.Fatal(err)
			}
			out, err := exec.Command("sh", "commit-msg-check.sh", file).CombinedOutput()
			if c.refusal == "" {
				if err != nil {
					t.Fatalf("refused: %v\n%s", err, out)
				}
				return
			}
			if err == nil {
				t.Fatalf("accepted, want a refusal naming %q", c.refusal)
			}
			if !strings.Contains(string(out), c.refusal) || !strings.Contains(string(out), "never amend") {
				t.Fatalf("refusal does not name %q and the rule:\n%s", c.refusal, out)
			}
		})
	}
}
