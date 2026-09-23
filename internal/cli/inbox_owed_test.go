package cli

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/boottime"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// owedSetup is a session api that has read a task and a question from web.
func owedSetup(t *testing.T) (dir string, self, web registry.Session, task, question string) {
	t.Helper()
	dir = liveSession(t, "api")
	web = otherRun(t, dir, "web")
	self, _ = registry.Lookup(dir, "api")
	t.Setenv(state.SessionEnv, "api")
	t.Setenv(epochEnv, self.Epoch())
	task = rawUnread(t, dir, "api", map[string]any{"from": "web", "fromEpoch": web.Epoch(), "toEpoch": self.Epoch(), "kind": "task", "text": "rerun the smoke\nand paste the tail"})
	question = rawUnread(t, dir, "api", map[string]any{"from": "web", "fromEpoch": web.Epoch(), "toEpoch": self.Epoch(), "kind": "question", "text": "which port?"})
	if code, _, errOut := run("inbox"); code != ExitOK {
		t.Fatalf("inbox: %d %s", code, errOut)
	}
	return dir, self, web, task, question
}

func owed(t *testing.T) owedModel {
	t.Helper()
	code, out, errOut := run("inbox", "--owed", "--json")
	if code != ExitOK {
		t.Fatalf("inbox --owed: %d %s %s", code, out, errOut)
	}
	var model owedModel
	if err := json.Unmarshal([]byte(out), &model); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	return model
}

func owedIDs(model owedModel) []string {
	var ids []string
	for _, message := range model.Messages {
		ids = append(ids, message.ID)
	}
	return ids
}

// A read task and a read question are shown again in full, in the order they
// were read, with sender, kind, id and time.
func TestOwedShowsWhatWasReadInFull(t *testing.T) {
	_, _, _, task, question := owedSetup(t)
	code, out, errOut := run("inbox", "--owed")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	for _, want := range []string{"2 read and still owed a report", "from web · task · ", task, "rerun the smoke\nand paste the tail", "from web · question · ", question, "which port?"} {
		if !strings.Contains(out, want) {
			t.Errorf("the output lacks %q:\n%s", want, out)
		}
	}
	if strings.Index(out, task) > strings.Index(out, question) {
		t.Errorf("not in the order read:\n%s", out)
	}
	model := owed(t)
	if len(model.Messages) != 2 || !model.Messages[0].Kept || model.Messages[0].Text != "rerun the smoke\nand paste the tail" ||
		model.Messages[1].Kind != inbox.Question || model.Messages[0].From != "web" || model.Messages[0].CreatedAt.IsZero() {
		t.Fatalf("got %+v", model)
	}
}

// A report settles what it answers, and then nothing is owed.
func TestOwedIsEmptyOnceReported(t *testing.T) {
	dir, self, _, _, _ := owedSetup(t)
	if err := completeTurn(dir, self, turnResult{ID: "t/1", Text: "done", Started: boottime.Now(), Ended: boottime.Now()}, "t"); err != nil {
		t.Fatal(err)
	}
	code, out, _ := run("inbox", "--owed")
	if code != ExitOK || strings.Count(strings.TrimSpace(out), "\n") != 0 || !strings.HasPrefix(out, "Nothing read is owed a report to another session.") || !strings.Contains(out, "plain shell") {
		t.Fatalf("exit %d: %q", code, out)
	}
	if model := owed(t); len(model.Messages) != 0 {
		t.Fatalf("got %+v", model)
	}
}

// A pending mark, and the interim turn end it makes, keep the work owed.
func TestOwedSurvivesPendingAndAnInterim(t *testing.T) {
	dir, self, _, task, question := owedSetup(t)
	turnStarted(t, dir, self, markAt-1)
	if code, _, errOut := run("pending", "the suite is running"); code != ExitOK {
		t.Fatalf("pending: %s", errOut)
	}
	if got := owedIDs(owed(t)); strings.Join(got, ",") != task+","+question {
		t.Fatalf("with the mark in place: %v", got)
	}
	if err := completeTurn(dir, self, turnResult{ID: "t/1", Text: "waiting", Started: markAt - 1, Ended: boottime.Now()}, "t"); err != nil {
		t.Fatal(err)
	}
	if kinds := kinds(reportsTo(t, dir, "web")); strings.Join(kinds, ",") != "pending" {
		t.Fatalf("the interim was not sent: %v", kinds)
	}
	if got := owedIDs(owed(t)); strings.Join(got, ",") != task+","+question {
		t.Fatalf("after the interim: %v", got)
	}
}

// Mail not read — held by the harness, or still unread — owes nothing yet.
func TestOwedLeavesOutWhatWasNotRead(t *testing.T) {
	dir := liveSession(t, "api")
	web := otherRun(t, dir, "web")
	self, _ := registry.Lookup(dir, "api")
	t.Setenv(state.SessionEnv, "api")
	t.Setenv(epochEnv, self.Epoch())
	held := rawUnread(t, dir, "api", map[string]any{"from": "web", "fromEpoch": web.Epoch(), "toEpoch": self.Epoch(), "kind": "task", "text": "held"})
	status, _ := json.Marshal(inbox.Status{State: inbox.Held, Via: "socket", At: time.Now()})
	if err := os.WriteFile(filepath.Join(state.InboxPath(dir, "api"), held+".status"), status, 0o600); err != nil {
		t.Fatal(err)
	}
	if model := owed(t); len(model.Messages) != 0 {
		t.Fatalf("unread mail was shown as owed: %+v", model)
	}
}

// A task owed long enough for its text to be gone is still named.
func TestOwedNamesWhatIsNoLongerKept(t *testing.T) {
	dir, _, _, task, _ := owedSetup(t)
	if err := os.Remove(filepath.Join(state.DonePath(dir, "api"), task+".json")); err != nil {
		t.Fatal(err)
	}
	model := owed(t)
	if len(model.Messages) != 2 || model.Messages[0].ID != task || model.Messages[0].Kept || model.Messages[0].From != "web" {
		t.Fatalf("got %+v", model)
	}
	if _, out, _ := run("inbox", "--owed"); !strings.Contains(out, "from web · "+task+" · the text is no longer kept") {
		t.Fatalf("got %s", out)
	}
}

// Showing what is owed marks, records and announces nothing: the state
// directory is the same file for file, byte for byte, after it.
func TestOwedChangesNothing(t *testing.T) {
	dir, _, _, _, _ := owedSetup(t)
	before := snapshotTree(t, dir)
	run("inbox", "--owed")
	run("inbox", "--owed", "--json")
	if after := snapshotTree(t, dir); after != before {
		t.Fatalf("the state directory changed:\n--- before\n%s\n--- after\n%s", before, after)
	}
}

func snapshotTree(t *testing.T, root string) string {
	t.Helper()
	var lines []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		line := path + " " + info.Mode().String() + " " + info.ModTime().Format(time.RFC3339Nano)
		if entry.Type().IsRegular() {
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			line += " " + string(raw)
		}
		lines = append(lines, line)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(lines, "\n")
}

func TestOwedRefusals(t *testing.T) {
	dir, self, _, _, _ := owedSetup(t)
	for _, args := range [][]string{
		{"inbox", "--owed", "--peek"},
		{"inbox", "--owed", "--message=1780000000000000000-012345abcdef"},
		{"inbox", "--owed=yes"},
	} {
		if code, _, errOut := run(args...); code != ExitUsage || !strings.Contains(errOut, "--owed") {
			t.Errorf("%v: exit %d, %s", args, code, errOut)
		}
	}

	t.Run("main owes nothing", func(t *testing.T) {
		markMain(t, dir, self.Name)
		if code, _, errOut := run("inbox", "--owed"); code != ExitUsage || !strings.Contains(errOut, "owes no reports") {
			t.Fatalf("exit %d, %s", code, errOut)
		}
	})
	t.Run("outside a session", func(t *testing.T) {
		t.Setenv(state.SessionEnv, "")
		if code, _, errOut := run("inbox", "--owed"); code != ExitUsage || !strings.Contains(errOut, "not part of a rewake session") {
			t.Fatalf("exit %d, %s", code, errOut)
		}
	})
}

// A turn a person stopped reports the stop and keeps the work owed.
func TestOwedSurvivesAStoppedTurn(t *testing.T) {
	dir, self, _, task, question := owedSetup(t)
	if err := completeTurn(dir, self, turnResult{ID: "t/1", Stopped: true, Started: boottime.Now(), Ended: boottime.Now()}, "t"); err != nil {
		t.Fatal(err)
	}
	if kinds := kinds(reportsTo(t, dir, "web")); strings.Join(kinds, ",") != "stopped" {
		t.Fatalf("the stop was not reported: %v", kinds)
	}
	if got := owedIDs(owed(t)); strings.Join(got, ",") != task+","+question {
		t.Fatalf("after the stop: %v", got)
	}
}

// What an earlier run of the name read is not the next run's to show.
func TestOwedKeepsToItsOwnRun(t *testing.T) {
	dir, _, _, _, _ := owedSetup(t)
	next := otherRun(t, dir, "api")
	t.Setenv(epochEnv, next.Epoch())
	if model := owed(t); len(model.Messages) != 0 {
		t.Fatalf("a new run was shown the old one's work: %+v", model)
	}
}
