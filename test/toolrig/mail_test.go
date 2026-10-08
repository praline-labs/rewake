//go:build rewakefixture

package toolrig

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/praline-labs/rewake/internal/cli"
	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/state"
)

// What the rig reads of the mail, and the other channels beside the tool: the
// shell, and the journal of an end the way the wrapper publishes it.

// shell runs words as api's shell does, with the rig's fault plan.
func (r *rig) shell(words ...string) (string, error) {
	command := exec.Command(binary, words...)
	command.Env = r.env()
	out, err := command.CombinedOutput()
	return string(out), err
}

// journal publishes the end the gate noted the way the wrapper does.
func (r *rig) journal(boundary *inbox.ReadBoundary, started int64) error {
	return cli.ReportCompletion(context.Background(), r.dir, r.self, harness.Completion{
		Boundary: boundary, ID: thread + "/" + r.turnID(), Thread: thread,
		Kind: inbox.Finished, Text: "the turn's last words", Started: started, Ended: r.endpoint.Gate().Noted(),
	})
}

// report is the kind of the report api's end sent web.
func report(r *rig) inbox.Kind {
	r.t.Helper()
	for _, dir := range []string{state.InboxPath(r.dir, "web"), state.UnreadPath(r.dir, "web")} {
		entries, _ := os.ReadDir(dir)
		for _, entry := range entries {
			raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil || entry.IsDir() {
				continue
			}
			var message inbox.Message
			if json.Unmarshal(raw, &message) == nil && message.From == "api" && message.Kind != inbox.Note {
				return message.Kind
			}
		}
	}
	return ""
}

const (
	faultLetter  = "the letter of the fault test"
	faultHeadsUp = "the heads-up of the fault test"
)

// headsUps counts the copies of the heads-up in web's mailbox.
func headsUps(r *rig) int {
	r.t.Helper()
	count := 0
	for _, dir := range []string{state.InboxPath(r.dir, "web"), state.UnreadPath(r.dir, "web"), state.DonePath(r.dir, "web")} {
		entries, _ := os.ReadDir(dir)
		for _, entry := range entries {
			raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil || entry.IsDir() {
				continue
			}
			var message inbox.Message
			if json.Unmarshal(raw, &message) == nil && message.Text == faultHeadsUp && message.From == "api" {
				count++
			}
		}
	}
	return count
}

// marks counts api's pending marks.
func marks(r *rig) int {
	found, _ := filepath.Glob(filepath.Join(state.InboxPath(r.dir, "api"), "pending", "marks", r.self.Epoch(), "*"))
	return len(found)
}

// letterID is the id of the rig's one letter.
func letterID(r *rig) string {
	r.t.Helper()
	entries, err := os.ReadDir(state.InboxPath(r.dir, "api"))
	if err != nil {
		r.t.Fatal(err)
	}
	for _, entry := range entries {
		if id, ok := strings.CutSuffix(entry.Name(), ".json"); ok {
			return id
		}
	}
	r.t.Fatal("the rig holds no letter")
	return ""
}

func letterRead(r *rig) bool { return !r.unread(letterID(r)) }
