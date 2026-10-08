package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

// leaveUnread puts a message where a serving process puts one it has announced.
func leaveUnread(t *testing.T, dir string, message inbox.Message) {
	t.Helper()
	message.ID, message.CreatedAt = inbox.NewID(), time.Now()
	unread := state.UnreadPath(dir, message.To)
	if err := state.EnsureSubdir(state.InboxPath(dir, message.To)); err != nil {
		t.Fatalf("mailbox: %v", err)
	}
	if err := state.EnsureSubdir(unread); err != nil {
		t.Fatalf("unread: %v", err)
	}
	encoded, _ := json.Marshal(message)
	if err := os.WriteFile(filepath.Join(unread, message.ID+".json"), encoded, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func epochOf(t *testing.T, dir, name string) string {
	t.Helper()
	session, err := registry.Lookup(dir, name)
	if err != nil {
		t.Fatalf("lookup %s: %v", name, err)
	}
	return session.Epoch()
}

func TestInboxShowsTheTextOnce(t *testing.T) {
	dir := liveSession(t, "api")
	t.Setenv(state.SessionEnv, "api")
	leaveUnread(t, dir, inbox.Message{From: "web", To: "api", ToEpoch: epochOf(t, dir, "api"), Kind: inbox.Question, Text: "which port?"})

	code, out, errOut := run("inbox")
	if code != ExitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, errOut)
	}
	if !strings.Contains(out, "from web · question") || !strings.Contains(out, "which port?") {
		t.Errorf("inbox = %q, want the sender, the kind and the text", out)
	}

	_, again, _ := run("inbox")
	if again != "Rewake: no new messages.\n" {
		t.Errorf("second read = %q, want nothing new", again)
	}
}

func TestInboxLeavesAnEarlierSessionsMail(t *testing.T) {
	dir := liveSession(t, "api")
	t.Setenv(state.SessionEnv, "api")
	leaveUnread(t, dir, inbox.Message{From: "web", To: "api", ToEpoch: "1.1", Text: "for someone else"})

	_, out, _ := run("inbox")
	if strings.Contains(out, "for someone else") {
		t.Errorf("inbox = %q, showed mail addressed to an earlier session with this name", out)
	}
}

func TestInboxOutsideASessionIsRefused(t *testing.T) {
	liveSession(t, "api")
	t.Setenv(state.SessionEnv, "")

	code, _, errOut := run("inbox")
	if code != ExitUsage || !strings.Contains(errOut, "not part of a rewake session") {
		t.Errorf("exit = %d, stderr = %q; want a refusal that says why", code, errOut)
	}
}

const turnPayload = `{"type":"agent-turn-complete","thread-id":"t","last-assistant-message":"the smoke is green"}`

// The end of a turn is reported to whoever wrote during it, with the last reply,
// and only once.
func TestTurnEndTellsTheSessionsThatWrote(t *testing.T) {
	dir := liveSession(t, "api")
	web := otherRun(t, dir, "web")
	t.Setenv(state.SessionEnv, "api")
	leaveUnread(t, dir, inbox.Message{From: "web", FromEpoch: web.Epoch(), To: "api", ToEpoch: epochOf(t, dir, "api"), Text: "rerun the smoke"})
	if code, _, errOut := run("inbox"); code != ExitOK {
		t.Fatalf("inbox exit = %d (%s)", code, errOut)
	}

	if code, out, errOut := run("turn-ended", turnPayload); code != ExitOK || out != "" || errOut != "" {
		t.Fatalf("turn-ended = %d %q %q, want a silent success", code, out, errOut)
	}
	waiting, _ := filepath.Glob(filepath.Join(state.InboxPath(dir, "web"), "*.json"))
	if len(waiting) != 1 {
		t.Fatalf("web holds %v, want one finished notice", waiting)
	}
	raw, _ := os.ReadFile(waiting[0])
	var notice inbox.Message
	if err := json.Unmarshal(raw, &notice); err != nil {
		t.Fatalf("notice: %v", err)
	}
	if notice.Kind != inbox.Finished || notice.From != "api" || notice.Text != "the smoke is green" || notice.ToEpoch != web.Epoch() {
		t.Errorf("notice = %+v, want a finished message from api with the last reply, for this run of web", notice)
	}

	run("turn-ended", turnPayload)
	again, _ := filepath.Glob(filepath.Join(state.InboxPath(dir, "web"), "*.json"))
	if len(again) != 1 {
		t.Errorf("web holds %d messages after a second turn, want the one notice", len(again))
	}
}

// A direct message does not settle what a turn owes: it may be a note sent
// mid-turn or a new request, and the other side still waits for the end.
func TestTurnEndReportsEvenAfterADirectMessage(t *testing.T) {
	dir := liveSession(t, "api")
	web := otherRun(t, dir, "web")
	t.Setenv(state.SessionEnv, "api")
	leaveUnread(t, dir, inbox.Message{From: "web", FromEpoch: web.Epoch(), To: "api", ToEpoch: epochOf(t, dir, "api"), Text: "rerun the smoke"})
	run("inbox")
	run("send", "web", "started", "--wait", "0")

	run("turn-ended", turnPayload)
	waiting, _ := filepath.Glob(filepath.Join(state.InboxPath(dir, "web"), "*.json"))
	if len(waiting) != 2 {
		t.Errorf("web holds %d messages, want the note and the report", len(waiting))
	}
}

// Codex calls its notify program for other events too; only the end of a turn
// is one.
func TestTurnEndIgnoresOtherEvents(t *testing.T) {
	dir := liveSession(t, "api")
	web := otherRun(t, dir, "web")
	t.Setenv(state.SessionEnv, "api")
	leaveUnread(t, dir, inbox.Message{From: "web", FromEpoch: web.Epoch(), To: "api", ToEpoch: epochOf(t, dir, "api"), Text: "rerun"})
	run("inbox")
	if waiting := inbox.Waiters(dir, "api", epochOf(t, dir, "api")); len(waiting) != 1 {
		t.Fatalf("waiting = %v; without a waiter this test proves nothing", waiting)
	}

	run("turn-ended", `{"type":"approval-requested"}`)
	waiting, _ := filepath.Glob(filepath.Join(state.InboxPath(dir, "web"), "*.json"))
	if len(waiting) != 0 {
		t.Errorf("web holds %d messages after an event that is not the end of a turn", len(waiting))
	}
}

func TestAQuestionFromAShellIsRefused(t *testing.T) {
	liveSession(t, "api")
	t.Setenv(state.SessionEnv, "")
	code, _, errOut := run("send", "api", "which port?", "--question", "--wait", "0")
	if code != ExitUsage || !strings.Contains(errOut, "needs a rewake session") {
		t.Errorf("exit = %d, stderr = %q; want a refusal that says a question needs a session", code, errOut)
	}
}

func TestTheKindFlagsExcludeEachOther(t *testing.T) {
	liveSession(t, "api")
	code, _, errOut := run("send", "api", "hm", "--question", "--notify")
	if code != ExitUsage || !strings.Contains(errOut, "exclude each other") {
		t.Errorf("exit = %d, stderr = %q; want a refusal", code, errOut)
	}
}

// The kind reaches the message: a task by default, the others by flag.
func TestSendWritesTheKind(t *testing.T) {
	for flag, want := range map[string]string{"": "task", "--notify": "notify"} {
		dir := liveSession(t, "api")
		args := []string{"send", "api", "hello", "--wait", "0"}
		if flag != "" {
			args = append(args, flag)
		}
		run(args...)
		waiting, _ := filepath.Glob(filepath.Join(state.InboxPath(dir, "api"), "*.json"))
		if len(waiting) != 1 {
			t.Fatalf("%s: mailbox holds %v, want one message", flag, waiting)
		}
		raw, _ := os.ReadFile(waiting[0])
		if !strings.Contains(string(raw), `"kind": "`+want+`"`) {
			t.Errorf("%s: message = %s, want kind %s", flag, raw, want)
		}
	}
}

func TestInternalCommandsStayOutOfTheGuide(t *testing.T) {
	_, out, _ := run()
	if strings.Contains(out, "turn-ended") {
		t.Error("the guide lists turn-ended, which agents have no reason to run")
	}
	if !strings.Contains(out, "rewake inbox") {
		t.Error("the guide does not say how to read a message")
	}
}
