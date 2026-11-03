package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/reportbrief"
	"github.com/praline-labs/rewake/internal/core/taskbrief"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// reportedBrief is main "lead" with brief 1 given to the worker, which commented
// before the task, ticked one item and commented after it, and then reported:
// the report waits unread in main's inbox.
func reportedBrief(t *testing.T) (string, registry.Session) {
	t.Helper()
	dir, worker := briefWorld(t)
	home := taskbrief.Home{Room: dir}
	lead, err := registry.Lookup(dir, "lead")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := taskbrief.New(home, taskbrief.Author{Name: "lead", Epoch: lead.Epoch()}, "# Login\n- [ ] move it\n- [ ] test it\n", "", nil); err != nil {
		t.Fatal(err)
	}
	run1 := taskbrief.Author{Name: worker.Name, Epoch: worker.Epoch()}
	if err := taskbrief.Bind(home, 1, run1, taskbrief.Work); err != nil {
		t.Fatal(err)
	}
	if _, _, err := taskbrief.Comment(home, 1, run1, "from an earlier task"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	task := mail.Message{ID: "task-1", From: "lead", FromEpoch: lead.Epoch(), To: worker.Name, ToEpoch: worker.Epoch(), Kind: mail.Task, Text: "do the login", Brief: "1", CreatedAt: time.Now()}
	done := state.DonePath(dir, worker.Name)
	if err := state.EnsureSubdir(done); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(task)
	if err := os.WriteFile(filepath.Join(done, task.ID+".json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := taskbrief.Tick(home, 1, run1, 1, 1, true); err != nil {
		t.Fatal(err)
	}
	if _, _, err := taskbrief.Comment(home, 1, run1, "Part A verdict: moved\n- second line"); err != nil {
		t.Fatal(err)
	}
	leaveUnread(t, dir, mail.Message{From: worker.Name, FromEpoch: worker.Epoch(), To: "lead", ToEpoch: lead.Epoch(), Kind: mail.Finished, Text: "login moved", InReplyTo: []string{task.ID}})
	return dir, worker
}

// A report on a brief's task is printed with the brief's state: what is still
// open, and the comments made since the task, each with author and capacity.
func TestAReportOnABriefTaskCarriesTheBriefsState(t *testing.T) {
	reportedBrief(t)
	code, out, errOut := run("inbox")
	if code != ExitOK {
		t.Fatalf("inbox: %d %q", code, errOut)
	}
	for _, want := range []string{"login moved", "── brief 1 · 1/2", "unticked:\n  #2 test it", "comments since the task:", "worker · work · ", "Part A verdict: moved\n    - second line"} {
		if !strings.Contains(out, want) {
			t.Errorf("the report lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "from an earlier task") {
		t.Errorf("a comment made before the task is shown:\n%s", out)
	}
}

func TestTheBriefsStateIsInTheJSON(t *testing.T) {
	reportedBrief(t)
	code, out, errOut := run("inbox", "--json")
	if code != ExitOK {
		t.Fatalf("inbox: %d %q", code, errOut)
	}
	var model struct {
		Messages []struct {
			BriefState []reportbrief.Block `json:"briefState"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(out), &model); err != nil || len(model.Messages) != 1 || len(model.Messages[0].BriefState) != 1 {
		t.Fatalf("model = %s (%v)", out, err)
	}
	block := model.Messages[0].BriefState[0]
	if block.ID != 1 || block.Done != 1 || block.Total != 2 || len(block.Unticked) != 1 || block.Unticked[0].N != 2 || len(block.Comments) != 1 || block.Comments[0].Capacity != taskbrief.Work {
		t.Fatalf("block = %+v", block)
	}
}

// A closed brief shows its last state; one that cannot be read leaves the
// report as it is, with a line saying so.
func TestAClosedOrUnreadableBriefLeavesTheReportReadable(t *testing.T) {
	dir, _ := reportedBrief(t)
	if _, err := taskbrief.Close(taskbrief.Home{Room: dir}, 1); err != nil {
		t.Fatal(err)
	}
	if _, out, _ := run("inbox", "--peek"); !strings.Contains(out, "login moved") {
		t.Fatalf("peek = %s", out)
	}
	_, out, _ := run("inbox")
	if !strings.Contains(out, "── brief 1 · 1/2 · closed") || !strings.Contains(out, "#2 test it") {
		t.Fatalf("a closed brief:\n%s", out)
	}

	dir, _ = reportedBrief(t)
	if err := os.RemoveAll(filepath.Join(dir, "briefs", "1", "brief.json")); err != nil {
		t.Fatal(err)
	}
	_, out, _ = run("inbox")
	if !strings.Contains(out, "login moved") || !strings.Contains(out, "── brief 1 could not be read") {
		t.Fatalf("an unreadable brief:\n%s", out)
	}
}

// A report that answers a task without a brief carries nothing.
func TestAReportOnATaskWithoutABriefCarriesNothing(t *testing.T) {
	dir, worker := briefWorld(t)
	lead, _ := registry.Lookup(dir, "lead")
	leaveUnread(t, dir, mail.Message{From: worker.Name, FromEpoch: worker.Epoch(), To: "lead", ToEpoch: lead.Epoch(), Kind: mail.Finished, Text: "done", InReplyTo: []string{"nothing"}})
	_, out, _ := run("inbox")
	if strings.Contains(out, "──") {
		t.Fatalf("a block under a plain report:\n%s", out)
	}
}
