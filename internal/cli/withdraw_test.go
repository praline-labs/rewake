package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// sender is web writing to api, with deliveries answered at once: no server
// serves api's mailbox here.
func sender(t *testing.T) (string, registry.Session) {
	t.Helper()
	dir, web := questionSender(t)
	awaitStatus = func(string, string, string, time.Duration) (inbox.Status, bool) {
		return inbox.Status{State: inbox.Delivered, Via: "socket"}, true
	}
	return dir, web
}

var idLine = regexp.MustCompile(`(?m)^id (\S+)$`)

// sendOK sends and returns the id the output ends with.
func sendOK(t *testing.T, argv ...string) string {
	t.Helper()
	code, out, errOut := run(append([]string{"send"}, argv...)...)
	if code != ExitOK {
		t.Fatalf("send %v: %d %s %s", argv, code, out, errOut)
	}
	found := idLine.FindStringSubmatch(out)
	if found == nil {
		t.Fatalf("send printed no id: %q", out)
	}
	return found[1]
}

func stored(t *testing.T, dir, id string) inbox.Message {
	t.Helper()
	for _, message := range inbox.SentMatching(dir, "web", epochOf(t, dir, "web"), id) {
		return message
	}
	t.Fatalf("%s is not kept", id)
	return inbox.Message{}
}

func TestWithdrawTakesAUniquePrefix(t *testing.T) {
	dir, _ := sender(t)
	id := sendOK(t, "api", "rerun the smoke")
	code, out, errOut := run("withdraw", shortRef(id)[:6])
	if code != ExitOK || !strings.Contains(out, "withdrew your task "+id+" from api before its notice went out") {
		t.Fatalf("withdraw: %d %q %q", code, out, errOut)
	}
	if status, _ := inbox.ReadStatus(dir, "api", id); !status.Withdrawn || status.Detail != "withdrawn by web" {
		t.Fatalf("status %+v", status)
	}
	if code, out, _ := run("withdraw", id); code != ExitOK || !strings.Contains(out, "already withdrawn") {
		t.Fatalf("again: %d %q", code, out)
	}
}

func TestAnAmbiguousPrefixListsTheCandidates(t *testing.T) {
	sender(t)
	first := sendOK(t, "api", "one")
	second := sendOK(t, "api", "two")
	code, _, errOut := run("withdraw", first[:6])
	// The ready line keeps its placeholder: any id filled in would be a guess.
	if code != ExitFailed || !strings.Contains(errOut, first) || !strings.Contains(errOut, second) || !strings.HasSuffix(errOut, "then: rewake withdraw <id>\n") {
		t.Fatalf("ambiguous: %d %q", code, errOut)
	}
	if code, _, errOut := run("withdraw", "ab"); code != ExitUsage || !strings.Contains(errOut, "too short") {
		t.Fatalf("short: %d %q", code, errOut)
	}
	if code, _, errOut := run("withdraw", "ffffffff"); code != ExitFailed || !strings.Contains(errOut, "rewake inbox --awaited") {
		t.Fatalf("none: %d %q", code, errOut)
	}
}

func TestWithdrawRefusesAReadMessageAndPointsToAnAddendum(t *testing.T) {
	dir, _ := sender(t)
	id := sendOK(t, "api", "rerun the smoke")
	if err := inbox.MarkRead(dir, "api", epochOf(t, dir, "api"), stored(t, dir, id), true); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := run("withdraw", id)
	if code != ExitFailed || !strings.Contains(errOut, "a read message is final; add to it with: rewake send api \"...\" --to "+shortRef(id)) {
		t.Fatalf("read: %d %q", code, errOut)
	}
}

func TestWithdrawNeedsTheSendingRun(t *testing.T) {
	sender(t)
	id := sendOK(t, "api", "rerun the smoke")
	t.Setenv(state.SessionEnv, "")
	if code, _, errOut := run("withdraw", id); code != ExitUsage || !strings.Contains(errOut, "Only the session run that sent a message") {
		t.Fatalf("shell: %d %q", code, errOut)
	}
}

func TestEditSendsAReplacementInOneStep(t *testing.T) {
	dir, _ := sender(t)
	old := sendOK(t, "api", "rerun the smoke")
	fresh := func() string {
		code, out, errOut := run("edit", shortRef(old), "rerun the smoke on staging")
		if code != ExitOK {
			t.Fatalf("edit: %d %q %q", code, out, errOut)
		}
		return idLine.FindStringSubmatch(out)[1]
	}()
	if replacement := stored(t, dir, fresh); replacement.Replaces != old || replacement.Text != "rerun the smoke on staging" || inbox.KindOf(replacement) != inbox.Task {
		t.Fatalf("replacement %+v", replacement)
	}
	if status, _ := inbox.ReadStatus(dir, "api", old); !status.Withdrawn || !strings.Contains(status.Detail, "replaced by "+fresh) {
		t.Fatalf("old status %+v", status)
	}
	if code, _, errOut := run("edit", old, "again"); code != ExitFailed || !strings.Contains(errOut, "already withdrawn") {
		t.Fatalf("edit of a withdrawn one: %d %q", code, errOut)
	}
}

func TestAnAddendumGoesWithItsTask(t *testing.T) {
	dir, web := sender(t)
	root := sendOK(t, "api", "rerun the smoke")
	for _, refused := range [][]string{{"--notify"}, {"--question"}, {"--grant-git"}} {
		if code, _, errOut := run(append([]string{"send", "api", "also lint", "--to", root}, refused...)...); code != ExitUsage {
			t.Errorf("--to %v: %d %q", refused, code, errOut)
		}
	}
	first := sendOK(t, "api", "also rerun the lint", "--to", shortRef(root))
	second := sendOK(t, "api", "and the vet", "--to", first)
	for _, id := range []string{first, second} {
		if message := stored(t, dir, id); message.AddendumTo != root || inbox.KindOf(message) != inbox.Task {
			t.Fatalf("addendum %+v", message)
		}
	}

	otherRun(t, dir, "db")
	if code, _, errOut := run("send", "db", "x", "--to", root); code != ExitFailed || !strings.Contains(errOut, "rewake send api \"...\" --to "+shortRef(root)) {
		t.Fatalf("to another recipient: %d %q", code, errOut)
	}

	// Once the task is reported on, an addendum would wait for a report
	// nobody owes: refused, with the new task to send instead.
	rawUnread(t, dir, "web", map[string]any{"from": "api", "fromEpoch": epochOf(t, dir, "api"), "kind": "finished", "toEpoch": web.Epoch(), "inReplyTo": []string{root}, "text": "done"})
	if code, _, errOut := run("send", "api", "one more", "--to", root); code != ExitFailed || !strings.Contains(errOut, "already reported on") {
		t.Fatalf("after the report: %d %q", code, errOut)
	}
}

// After a compaction the worker re-reads its task with --owed, and the
// addendum comes with it.
func TestOwedShowsAnAddendumUnderItsTask(t *testing.T) {
	dir := liveSession(t, "api")
	web := otherRun(t, dir, "web")
	t.Setenv(state.SessionEnv, "api")
	root := rawUnread(t, dir, "api", map[string]any{"from": "web", "fromEpoch": web.Epoch(), "toEpoch": epochOf(t, dir, "api"), "kind": "task", "text": "rerun the smoke"})
	rawUnread(t, dir, "api", map[string]any{"from": "db", "fromEpoch": web.Epoch(), "toEpoch": epochOf(t, dir, "api"), "kind": "task", "text": "an unrelated task"})
	rawUnread(t, dir, "api", map[string]any{"from": "web", "fromEpoch": web.Epoch(), "toEpoch": epochOf(t, dir, "api"), "kind": "task", "addendumTo": root, "text": "also rerun the lint"})
	code, out, _ := run("inbox")
	if code != ExitOK || !strings.Contains(out, "· addendum to "+root) {
		t.Fatalf("inbox: %d %q", code, out)
	}
	_, out, _ = run("inbox", "--owed")
	smoke, lint, unrelated := strings.Index(out, "rerun the smoke"), strings.Index(out, "+ addendum from web"), strings.Index(out, "an unrelated task")
	if smoke < 0 || lint < smoke || unrelated < lint {
		t.Fatalf("the addendum is not under its task:\n%s", out)
	}
}

// A tombstone among the candidates is named for what it was, not by its
// tombstone text.
func TestAnAmbiguousPrefixNamesAWithdrawnCandidate(t *testing.T) {
	sender(t)
	first := sendOK(t, "api", "one")
	second := sendOK(t, "api", "two")
	if code, out, errOut := run("withdraw", first); code != ExitOK {
		t.Fatalf("withdraw: %d %q %q", code, out, errOut)
	}
	_, _, errOut := run("withdraw", first[:6])
	if !strings.Contains(errOut, first+" · task (withdrawn) to api · ") || strings.Contains(errOut, "disregard") || !strings.Contains(errOut, second+" · task to api · ") {
		t.Fatalf("ambiguous: %q", errOut)
	}
}

// announce makes a sent message readable, as the server does right before its
// notice: what withdraw reads as a notice that may have gone out.
func announce(t *testing.T, dir, id string) {
	t.Helper()
	unread := state.UnreadPath(dir, "api")
	if err := state.EnsureSubdir(unread); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(filepath.Join(state.InboxPath(dir, "api"), id+".json"), filepath.Join(unread, id+".json")); err != nil {
		t.Fatal(err)
	}
}

// recalls are the notes api holds telling it not to act on a notice.
func recalls(t *testing.T, dir string) []inbox.Message {
	t.Helper()
	var out []inbox.Message
	files, _ := filepath.Glob(filepath.Join(state.InboxPath(dir, "api"), "*.json"))
	for _, file := range files {
		raw, err := os.ReadFile(file)
		var message inbox.Message
		if err == nil && json.Unmarshal(raw, &message) == nil && message.Recall != nil {
			out = append(out, message)
		}
	}
	return out
}

// After its notice, a withdrawal also tells the recipient, at once, not to act
// on it: an agent may act on the preview without reading its inbox. Before
// the notice it stays silent.
func TestAWithdrawalAfterTheNoticeTellsTheRecipient(t *testing.T) {
	dir, _ := sender(t)
	quiet := sendOK(t, "api", "rerun the smoke")
	if code, _, errOut := run("withdraw", quiet); code != ExitOK || len(recalls(t, dir)) != 0 {
		t.Fatalf("before the notice: %d %q, recalls %+v", code, errOut, recalls(t, dir))
	}
	loud := sendOK(t, "api", "delete the staging bucket")
	announce(t, dir, loud)
	code, out, errOut := run("withdraw", loud)
	if code != ExitOK || !strings.Contains(out, "may have gone out, so api is told not to act on it") {
		t.Fatalf("after the notice: %d %q %q", code, out, errOut)
	}
	told := recalls(t, dir)
	want := "Do not act on task " + shortRef(loud) + " from web ("
	if len(told) != 1 || told[0].Recall.ID != loud || inbox.KindOf(told[0]) != inbox.Note || told[0].From != "web" ||
		!strings.HasPrefix(told[0].Text, want) || !strings.HasSuffix(told[0].Text, "): withdrawn unread.") {
		t.Fatalf("recalls %+v", told)
	}
	if len(inbox.SentMatching(dir, "web", epochOf(t, dir, "web"), told[0].ID)) != 0 {
		t.Fatal("the recall is taken for a letter of the sender's own")
	}
	if code, _, _ := run("withdraw", loud); code != ExitOK || len(recalls(t, dir)) != 1 {
		t.Fatalf("a second withdraw told api again: %+v", recalls(t, dir))
	}
}

// An edit sends no recall: its replacement's preview names the old message,
// and a recall announced beside it would be one more line before the new work.
func TestAnEditAfterTheNoticeSendsNoRecall(t *testing.T) {
	dir, _ := sender(t)
	old := sendOK(t, "api", "rerun the lint")
	announce(t, dir, old)
	code, out, errOut := run("edit", old, "rerun the lint and the vet")
	if code != ExitOK || idLine.FindStringSubmatch(out) == nil || strings.Contains(out, "told api") {
		t.Fatalf("edit: %d %q %q", code, out, errOut)
	}
	if told := recalls(t, dir); len(told) != 0 {
		t.Fatalf("an edit sent a recall: %+v", told)
	}
}
