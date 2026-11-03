package delivery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/taskbrief"
	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// The announcement of a report on a brief's task gets the note "brief N x/y",
// read from the brief when the notice is made; a report on a plain task, or a
// brief that cannot be read, gets none.
func TestNoteBriefsReadsTheBriefOfTheTaskAReportAnswers(t *testing.T) {
	base := t.TempDir()
	if err := os.Chmod(base, 0o700); err != nil {
		t.Fatal(err)
	}
	dir := buildtest.Room(t, base, "room")
	home := taskbrief.Home{Room: dir}
	lead := taskbrief.Author{Name: "lead", Epoch: "1.1.boot"}
	if _, err := taskbrief.New(home, lead, "# Login\n- [ ] move it\n- [ ] test it\n", "", nil); err != nil {
		t.Fatal(err)
	}
	worker := taskbrief.Author{Name: "worker", Epoch: "2.2.boot"}
	if err := taskbrief.Bind(home, 1, worker, taskbrief.Work); err != nil {
		t.Fatal(err)
	}
	if _, err := taskbrief.Tick(home, 1, worker, 1, 1, true); err != nil {
		t.Fatal(err)
	}
	for _, task := range []mail.Message{
		{ID: "t-brief", From: "lead", To: "worker", Kind: mail.Task, Brief: "1", CreatedAt: time.Now()},
		{ID: "t-plain", From: "lead", To: "worker", Kind: mail.Task, CreatedAt: time.Now()},
	} {
		raw, _ := json.Marshal(task)
		if err := state.EnsureSubdir(state.DonePath(dir, "worker")); err != nil {
			t.Fatal(err)
		}
		if err := state.WriteAtomic(filepath.Join(state.DonePath(dir, "worker"), task.ID+".json"), raw); err != nil {
			t.Fatal(err)
		}
	}
	ready := []mail.Message{
		{ID: "r1", From: "worker", To: "lead", Kind: mail.Finished, InReplyTo: []string{"t-brief"}},
		{ID: "r2", From: "worker", To: "lead", Kind: mail.Finished, InReplyTo: []string{"t-plain"}},
		{ID: "r3", From: "worker", To: "lead", Kind: mail.Note, Text: "fyi"},
	}
	(&Server{Dir: dir}).noteBriefs(ready)
	if ready[0].BriefNote() != "brief 1 1/2" || ready[1].BriefNote() != "" || ready[2].BriefNote() != "" {
		t.Fatalf("notes = %q, %q, %q", ready[0].BriefNote(), ready[1].BriefNote(), ready[2].BriefNote())
	}
	if err := os.RemoveAll(filepath.Join(dir, "briefs", "1", "brief.json")); err != nil {
		t.Fatal(err)
	}
	ready[0] = ready[0].WithBriefNote("")
	(&Server{Dir: dir}).noteBriefs(ready)
	if ready[0].BriefNote() != "" {
		t.Fatalf("an unreadable brief gave the note %q", ready[0].BriefNote())
	}
}
