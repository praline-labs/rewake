package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/grant/authority"
	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/mail/mailtest"
	"github.com/praline-labs/rewake/internal/core/person"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// decideLab is api stopped on a report to web whose outcome is unknown: its
// once mark does not read. web is this test process, so a decision verified
// as web is written by the process its run names, as by its wrapper.
type decideLab struct {
	dir    string
	web    registry.Session
	api    registry.Session
	report mail.Message
}

func newDecideLab(t *testing.T) decideLab {
	t.Helper()
	dir := liveSession(t, "web")
	web, err := registry.Lookup(dir, "web")
	if err != nil {
		t.Fatal(err)
	}
	api := otherRun(t, dir, "api")
	epoch := api.Epoch()
	question := mail.Message{ID: questionID, From: "web", FromEpoch: web.Epoch(), To: "api", ToEpoch: epoch, Kind: mail.Question, Text: "is it done?", CreatedAt: time.Now()}
	putRead(t, dir, question)
	if err := mailtest.Owe(dir, "api", epoch, "web", web.Epoch(), questionID); err != nil {
		t.Fatal(err)
	}
	waiters, err := mail.ReadWaiters(dir, "api", epoch)
	if err != nil || len(waiters) != 1 {
		t.Fatalf("the wait: %v %v", waiters, err)
	}
	report := mail.Message{
		ID: mail.ReportID("api", epoch, waiters[0]), From: "api", FromEpoch: epoch, To: "web", ToEpoch: web.Epoch(),
		Kind: mail.Finished, Text: "the answer", InReplyTo: []string{questionID}, CreatedAt: time.Now(),
	}
	if err := mail.WriteJournal(dir, "api", "end", mail.TurnJournal{Epoch: epoch, Reports: []mail.Message{report}, Clear: waiters}); err != nil {
		t.Fatal(err)
	}
	mark := mailtest.OncePath(dir, "web", web.Epoch(), report.ID)
	if err := os.MkdirAll(filepath.Dir(mark), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mark, []byte("damaged"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := state.WithMailboxLock(ctx, dir, "api", func() error { return mail.Reconcile(ctx, dir, "api") }); err == nil {
		t.Fatal("the barrier went past an unknown mark")
	}
	return decideLab{dir: dir, web: web, api: api, report: report}
}

// questionID is web's question to api, which api read and owes the report on.
const questionID = "1790370984617481100-00000000000a"

// putRead leaves a letter where its recipient's read leaves it: in done/,
// its status read.
func putRead(t *testing.T, dir string, message mail.Message) {
	t.Helper()
	for path, value := range map[string]any{
		filepath.Join(state.DonePath(dir, message.To), message.ID+".json"):    message,
		filepath.Join(state.InboxPath(dir, message.To), message.ID+".status"): mail.Status{State: mail.Read, At: time.Now()},
	} {
		encoded, _ := json.Marshal(value)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, encoded, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// route stands in for whom the command asks: below the recipient's wrapper
// or not, the wrapper's answer, the person's classification. A way the test
// did not expect fails it.
type route struct {
	below    bool
	wrapper  func(authority.Run, mail.DecideRequest) (mail.DecideAnswer, error)
	classify error
	asked    int
	scanned  int
}

func (r *route) install(t *testing.T) {
	t.Helper()
	previous := [3]any{askWrapper, classifyPerson, runsBelow}
	runsBelow = func(authority.Run) bool { return r.below }
	askWrapper = func(_ string, run authority.Run, ask mail.DecideRequest, _ string) (mail.DecideAnswer, error) {
		r.asked++
		if r.wrapper == nil {
			t.Error("the wrapper was asked")
			return mail.DecideAnswer{}, authority.ErrUnreachable
		}
		return r.wrapper(run, ask)
	}
	classifyPerson = func(string) error {
		r.scanned++
		if r.below {
			t.Error("a caller below the recipient's wrapper was classified for the person")
		}
		return r.classify
	}
	t.Cleanup(func() {
		askWrapper = previous[0].(func(string, authority.Run, mail.DecideRequest, string) (mail.DecideAnswer, error))
		classifyPerson = previous[1].(func(string) error)
		runsBelow = previous[2].(func(authority.Run) bool)
	})
}

// asWrapper is the recipient's wrapper deciding, as decideHandler does.
func (l decideLab) asWrapper(_ authority.Run, ask mail.DecideRequest) (mail.DecideAnswer, error) {
	ask.By = mail.DecidedBy{Kind: mail.ByRecipient, Name: "web", Run: l.web.Epoch(), Verified: true}
	return mail.Decide(context.Background(), l.dir, ask)
}

func (l decideLab) decision(t *testing.T) *mail.Decision {
	t.Helper()
	views, err := mail.Decisions(l.dir, "api")
	if err != nil {
		t.Fatal(err)
	}
	for _, view := range views {
		if view.Report == l.report.ID {
			return view.Decision
		}
	}
	return nil
}

// The list names the subject and the two lines that decide it, in text and
// JSON, and writes nothing.
func TestDecideListsWhatCanBeDecided(t *testing.T) {
	lab := newDecideLab(t)
	(&route{}).install(t)
	before := snapshotTree(t, lab.dir)
	code, out, errOut := run("decide", "api")
	if code != ExitOK || !strings.Contains(out, lab.report.ID+" · ") || !strings.Contains(out, "unknown — ") ||
		!strings.Contains(out, "rewake decide api "+lab.report.ID+" --arrived | --not-arrived") || !strings.Contains(out, "answering "+questionID) {
		t.Fatalf("exit %d:\n%s%s", code, out, errOut)
	}
	code, out, errOut = run("decide", "api", "--json")
	var listed decideListModel
	if code != ExitOK || json.Unmarshal([]byte(out), &listed) != nil {
		t.Fatalf("exit %d:\n%s%s", code, out, errOut)
	}
	if len(listed.Subjects) != 1 || listed.Subjects[0].Report != lab.report.ID || listed.Subjects[0].Decided != nil ||
		listed.Subjects[0].Recipient != "web" || listed.Subjects[0].RecipientRun != lab.web.Epoch() || len(listed.Undecidable) != 0 {
		t.Fatalf("listed %+v", listed)
	}
	if after := snapshotTree(t, lab.dir); after != before {
		t.Fatalf("the list changed the state directory:\n--- before\n%s\n--- after\n%s", before, after)
	}
	if code, out, _ := run("decide", "web"); code != ExitOK || !strings.Contains(out, "web has no report to decide") {
		t.Fatalf("a mailbox with nothing to decide: %d %s", code, out)
	}
}

// The person decides in this process, unverified, and the answer is read
// from the mailbox: the resend, and api going on. The same words again say
// recorded before; the other choice is refused.
func TestDecideAsThePerson(t *testing.T) {
	lab := newDecideLab(t)
	r := &route{}
	r.install(t)
	code, out, errOut := run("decide", "api", lab.report.ID, "--not-arrived")
	d := lab.decision(t)
	if code != ExitOK || d == nil || d.By.Kind != mail.ByPerson || d.By.Verified || r.scanned != 1 || r.asked != 0 {
		t.Fatalf("exit %d, decision %+v, scanned %d, asked %d:\n%s%s", code, d, r.scanned, r.asked, out, errOut)
	}
	want := "Rewake: recorded: report " + lab.report.ID + " of api to web did not arrive — decided by the person (unverified), unproven; operation end"
	if !strings.HasPrefix(out, want) || !strings.Contains(out, "sent again as "+d.Resent()+" (") || !strings.HasSuffix(strings.TrimSpace(out), "api goes on.") {
		t.Fatalf("answered:\n%s", out)
	}
	code, out, _ = run("decide", "api", lab.report.ID, "--not-arrived", "--json")
	var again decideModel
	if code != ExitOK || json.Unmarshal([]byte(out), &again) != nil || !again.Before || !again.Recorded || again.Decision.ID != d.ID ||
		again.Decision.Delivery != "unproven" || again.Resent != d.Resent() || again.Stopped {
		t.Fatalf("again: exit %d %s", code, out)
	}
	if code, _, errOut := run("decide", "api", lab.report.ID, "--arrived"); code != ExitFailed || !strings.Contains(errOut, "(opposite)") {
		t.Fatalf("the other choice: exit %d %s", code, errOut)
	}
}

// Below the recipient's wrapper the command asks the wrapper, which writes
// verified; a wrapper out of reach is a refusal naming the tool, never the
// person's path.
func TestDecideBelowTheRecipientAsksItsWrapper(t *testing.T) {
	t.Run("unreachable", func(t *testing.T) {
		lab := newDecideLab(t)
		r := &route{below: true, wrapper: func(authority.Run, mail.DecideRequest) (mail.DecideAnswer, error) {
			return mail.DecideAnswer{}, errors.Join(authority.ErrUnreachable, os.ErrPermission)
		}}
		r.install(t)
		code, out, errOut := run("decide", "api", lab.report.ID, "--arrived")
		if code != ExitFailed || r.scanned != 0 || lab.decision(t) != nil || !strings.Contains(errOut, "rewake tool's decide") || !strings.Contains(errOut, "never taken for the person") {
			t.Fatalf("exit %d, scanned %d:\n%s%s", code, r.scanned, out, errOut)
		}
	})
	t.Run("verified", func(t *testing.T) {
		lab := newDecideLab(t)
		r := &route{below: true, wrapper: lab.asWrapper}
		r.install(t)
		code, out, errOut := run("decide", "api", lab.report.ID, "--arrived")
		d := lab.decision(t)
		if code != ExitOK || d == nil || !d.By.Verified || d.By.Run != lab.web.Epoch() || !strings.Contains(out, "arrived — decided by web (verified), unproven") ||
			strings.Contains(out, "sent again") || !strings.HasSuffix(strings.TrimSpace(out), "api goes on.") {
			t.Fatalf("exit %d, decision %+v:\n%s%s", code, d, out, errOut)
		}
	})
}

// A caller placed in a session that may not decide is a wrong call naming who
// may; one whose identity does not read is refused; neither writes.
func TestDecideRefusesWhoMayNot(t *testing.T) {
	cases := map[string]struct {
		classify error
		code     int
		want     string
	}{
		"below another session": {&person.BelowError{PID: 1, Session: "lead", Room: "default"}, ExitUsage, "is decided by web (run "},
		"unreadable":            {&person.UnreadableError{What: "process 7", Err: os.ErrPermission}, ExitFailed, "cannot be told"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			lab := newDecideLab(t)
			(&route{classify: c.classify}).install(t)
			before := snapshotTree(t, state.InboxPath(lab.dir, "api"))
			code, out, errOut := run("decide", "api", lab.report.ID, "--arrived")
			if code != c.code || !strings.Contains(errOut, c.want) || lab.decision(t) != nil {
				t.Fatalf("exit %d:\n%s%s", code, out, errOut)
			}
			if after := snapshotTree(t, state.InboxPath(lab.dir, "api")); after != before {
				t.Fatal("a refused decision changed the mailbox")
			}
		})
	}
}

// The words are checked before anything is read.
func TestDecideWrongCalls(t *testing.T) {
	lab := newDecideLab(t)
	(&route{}).install(t)
	for _, argv := range [][]string{
		{"decide"},
		{"decide", "API"},
		{"decide", "api", "--arrived"},
		{"decide", "api", lab.report.ID},
		{"decide", "api", lab.report.ID, "--arrived", "--not-arrived"},
		{"decide", "api", "1790370984617481194-000000000000", "--arrived"},
	} {
		if code, _, errOut := run(argv...); code != ExitUsage {
			t.Errorf("%q: exit %d %s", argv, code, errOut)
		}
	}
	if lab.decision(t) != nil {
		t.Fatal("a wrong call decided")
	}
}

// Through the tool a decision runs under a receipt bound to the call, and
// retry with it gives the answer again without deciding twice.
func TestDecideThroughTheTool(t *testing.T) {
	lab := newDecideLab(t)
	t.Setenv(state.SessionEnv, "web")
	r := &route{below: true, wrapper: lab.asWrapper}
	r.install(t)
	caller := newToolCaller(t)
	first := caller.run("decide", "api", lab.report.ID, "--not-arrived")
	found := receiptLine.FindStringSubmatch(first.out)
	if first.code != ExitOK || found == nil || lab.decision(t) == nil || !lab.decision(t).By.Verified {
		t.Fatalf("exit %d:\n%s%s", first.code, first.out, first.errOut)
	}
	again := caller.run("retry", found[1])
	if again.code != ExitOK || !strings.Contains(again.out, "these words ran earlier") || r.asked != 1 {
		t.Fatalf("retry: exit %d, asked %d:\n%s%s", again.code, r.asked, again.out, again.errOut)
	}
	if list := caller.run("decide", "api"); list.code != ExitOK || !strings.Contains(list.out, "decided "+lab.decision(t).ID) {
		t.Fatalf("the list through the tool: exit %d\n%s%s", list.code, list.out, list.errOut)
	}
}
