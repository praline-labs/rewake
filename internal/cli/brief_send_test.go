package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/taskbrief"
)

// briefSender is main "web" with brief 1 made, sending to the worker "api",
// this process.
func briefSender(t *testing.T) (string, registry.Session, registry.Session) {
	t.Helper()
	dir, web := sender(t)
	markMain(t, dir, "web")
	if code, _, errOut := run("brief", "new", "# Login\n- [ ] move it\n- [ ] test it\n"); code != ExitOK {
		t.Fatalf("new: %d %q", code, errOut)
	}
	api, err := registry.Lookup(dir, "api")
	if err != nil {
		t.Fatal(err)
	}
	return dir, web, api
}

// send --brief binds the recipient's run before the letter is written, and
// the letter carries the id, not the text; an edit's replacement keeps it.
func TestSendBriefBindsTheRecipientsRun(t *testing.T) {
	dir, _, api := briefSender(t)
	id := sendOK(t, "api", "do the login", "--brief", "1")
	if letter := stored(t, dir, id); letter.Brief != "1" || strings.Contains(letter.Text, "move it") {
		t.Fatalf("the letter = %+v", letter)
	}
	brief, err := taskbrief.Load(dir, 1)
	if err != nil || !brief.Has(taskbrief.Author{Name: "api", Epoch: api.Epoch()}) {
		t.Fatalf("bound = %+v, %v", brief.Bound, err)
	}
	code, out, errOut := run("edit", id, "do the login, then the logout")
	replacement := idLine.FindStringSubmatch(out)
	if code != ExitOK || replacement == nil {
		t.Fatalf("edit: %d %q %q", code, out, errOut)
	}
	if letter := stored(t, dir, replacement[1]); letter.Replaces != id || letter.Brief != "1" {
		t.Fatalf("the replacement = %+v", letter)
	}
}

func TestSendBriefIsRefusedBeforeAnythingIsSent(t *testing.T) {
	dir, _, _ := briefSender(t)
	run("brief", "new", "closed one\n")
	run("brief", "close", "2")
	for _, c := range []struct {
		argv []string
		code int
	}{
		{[]string{"send", "api", "fyi", "--notify", "--brief", "1"}, ExitUsage},
		{[]string{"send", "api", "do it", "--brief", "x"}, ExitUsage},
		{[]string{"send", "api", "do it", "--brief", "9"}, ExitUsage},
		{[]string{"send", "api", "do it", "--brief", "2"}, ExitFailed},
	} {
		if code, _, errOut := run(c.argv...); code != c.code {
			t.Errorf("%q: %d %q", c.argv, code, errOut)
		}
	}
	if brief, err := taskbrief.Load(dir, 1); err != nil || len(brief.Bound) != 0 {
		t.Fatalf("a refused send bound %+v (%v)", brief.Bound, err)
	}
	worker := otherRun(t, dir, "other")
	as(t, worker)
	if code, _, errOut := run("send", "api", "do it", "--brief", "1"); code != ExitFailed || !strings.Contains(errOut, "only main gives a brief") {
		t.Fatalf("a worker's --brief: %d %q", code, errOut)
	}
}

// The worker reads the brief with the letter, as it stands then: --owed
// prints it again with main's edits and the ticks since.
func TestTheInboxPrintsTheBriefWithTheLetter(t *testing.T) {
	dir, web, api := briefSender(t)
	if err := taskbrief.Bind(dir, 1, taskbrief.Author{Name: "api", Epoch: api.Epoch()}); err != nil {
		t.Fatal(err)
	}
	leaveUnread(t, dir, mail.Message{From: "web", FromEpoch: web.Epoch(), To: "api", ToEpoch: api.Epoch(), Kind: mail.Task, Text: "do the login", Brief: "1"})
	as(t, api)
	code, out, errOut := run("inbox")
	if code != ExitOK {
		t.Fatalf("inbox: %d %q", code, errOut)
	}
	for _, want := range []string{"do the login\n\nbrief 1 · Login · edition 1 · 0 of 2 done · 0 comments\n", "rewake brief tick 1 <n> --edition 1", "- [ ] #1 move it\n- [ ] #2 test it"} {
		if !strings.Contains(out, want) {
			t.Fatalf("inbox = %q, want %q", out, want)
		}
	}
	run("brief", "tick", "1", "1", "--edition", "1")
	_, owed, _ := run("inbox", "--owed")
	if !strings.Contains(owed, "1 of 2 done") || !strings.Contains(owed, "- [x] #1 move it") {
		t.Fatalf("owed = %q", owed)
	}
	_, raw, _ := run("inbox", "--owed", "--json")
	var model struct {
		Messages []struct {
			Brief struct {
				ID      int    `json:"id"`
				Edition int    `json:"edition"`
				Text    string `json:"text"`
			} `json:"brief"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(raw), &model); err != nil || len(model.Messages) != 1 || model.Messages[0].Brief.ID != 1 || model.Messages[0].Brief.Edition != 1 || !strings.Contains(model.Messages[0].Brief.Text, "- [x] move it") {
		t.Fatalf("owed --json = %s (%v)", raw, err)
	}
}

// A brief that cannot be read leaves the letter whole and says how to read
// it again; a closed one prints as closed.
func TestALettersBriefThatDoesNotReadLeavesTheLetter(t *testing.T) {
	dir, web, api := briefSender(t)
	run("brief", "close", "1")
	leaveUnread(t, dir, mail.Message{From: "web", FromEpoch: web.Epoch(), To: "api", ToEpoch: api.Epoch(), Kind: mail.Task, Text: "first", Brief: "1"})
	leaveUnread(t, dir, mail.Message{From: "web", FromEpoch: web.Epoch(), To: "api", ToEpoch: api.Epoch(), Kind: mail.Task, Text: "second", Brief: "7"})
	as(t, api)
	code, out, errOut := run("inbox")
	if code != ExitOK || !strings.Contains(out, "first\n\nbrief 1 · Login · edition 1 · 0 of 2 done · 0 comments · closed\nRewake: this brief is closed") || !strings.Contains(out, "second\n\nRewake: this letter gives brief 7, which could not be read: no such brief. Read it with rewake brief show 7.") {
		t.Fatalf("inbox: %d %q %q", code, out, errOut)
	}
}
