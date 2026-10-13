package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/mail/mailtest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// asWeb runs the commands that follow in web's session, the run waiting for
// api's report.
func (l decideLab) asWeb(t *testing.T) {
	t.Helper()
	t.Setenv(state.SessionEnv, "web")
	t.Setenv(epochEnv, l.web.Epoch())
}

// A question whose report was decided "arrived" stops being awaited: its
// sender's views call it closed by the decision, delivery unproven, and an
// addition to it is refused.
func TestDecidedArrivedClosesTheWait(t *testing.T) {
	lab := newDecideLab(t)
	(&route{below: true, wrapper: lab.asWrapper}).install(t)
	lab.asWeb(t)
	if _, out, _ := run("inbox", "--awaited"); !strings.Contains(out, questionID+" · question · ") || !strings.Contains(out, "read, being worked on") {
		t.Fatalf("before the decision:\n%s", out)
	}
	if code, out, errOut := run("decide", "api", lab.report.ID, "--arrived"); code != ExitOK {
		t.Fatalf("decide: exit %d\n%s%s", code, out, errOut)
	}
	d := lab.decision(t)
	_, out, _ := run("inbox", "--awaited")
	if !strings.Contains(out, "closed by decision "+d.ID+" of web (verified), delivery unproven") || !strings.Contains(out, "waiting on no reports; 1 will not come") {
		t.Fatalf("after the decision:\n%s", out)
	}
	var model awaitedModel
	_, out, _ = run("inbox", "--awaited", "--json")
	if err := json.Unmarshal([]byte(out), &model); err != nil || len(model.Recipients) != 1 || len(model.Recipients[0].Messages) != 1 {
		t.Fatalf("%v:\n%s", err, out)
	}
	got := model.Recipients[0].Messages[0]
	if got.State != mail.StageClosed || got.Decided == nil || got.Decided.ID != d.ID || !got.Decided.Closed || got.Decided.Delivery != "unproven" || !got.Decided.By.Verified {
		t.Fatalf("the model: %+v %+v", got, got.Decided)
	}
	if code, _, errOut := run("send", "api", "one more thing", "--to", questionID); code == ExitOK || !strings.Contains(errOut, "closed your question "+questionID+", delivery unproven") {
		t.Fatalf("an addition to a closed question: exit %d %s", code, errOut)
	}
}

// A report decided "did not arrive" goes again, and its reader is told the
// copy is a resend: the first may have reached it as well.
func TestTheResendSaysItIsOne(t *testing.T) {
	lab := newDecideLab(t)
	(&route{}).install(t)
	if code, out, errOut := run("decide", "api", lab.report.ID, "--not-arrived"); code != ExitOK {
		t.Fatalf("decide: exit %d\n%s%s", code, out, errOut)
	}
	d := lab.decision(t)
	lab.asWeb(t)
	// The person repairs web's own mark, which stops web's reads as any
	// record that does not read does; the decision changes none of it.
	if err := os.Remove(mailtest.OncePath(lab.dir, "web", lab.web.Epoch(), lab.report.ID)); err != nil {
		t.Fatal(err)
	}
	// Announced, as web's server does before its notice.
	unread := state.UnreadPath(lab.dir, "web")
	if err := state.EnsureSubdir(unread); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(filepath.Join(state.InboxPath(lab.dir, "web"), d.Resent()+".json"), filepath.Join(unread, d.Resent()+".json")); err != nil {
		t.Fatalf("the resend is not in web's mailbox: %v", err)
	}
	code, out, errOut := run("inbox", "--json")
	var model inboxModel
	if code != ExitOK || json.Unmarshal([]byte(out), &model) != nil {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	var resend *messageView
	for i := range model.Messages {
		if model.Messages[i].ID == d.Resent() {
			resend = &model.Messages[i]
		}
	}
	if resend == nil || resend.ResentBy == nil || resend.ResentBy.ID != d.ID || resend.ResentBy.Report != lab.report.ID {
		t.Fatalf("the resend read as %+v", resend)
	}
	line := strings.Join(letterHead(*resend), "\n")
	if !strings.Contains(line, "sent again by decision "+d.ID+" of the person (unverified): report "+lab.report.ID+" went unproven") {
		t.Fatalf("the heading:\n%s", line)
	}
	if _, out, _ := run("inbox", "--awaited"); strings.Contains(out, questionID) {
		t.Fatalf("the question answered by the resend is still awaited:\n%s", out)
	}
}

// A question waiting for its answer ends when a decision closes it, with no
// answer text: none is proven to have reached the asker.
func TestAWaitingQuestionEndsWhenADecisionClosesIt(t *testing.T) {
	lab := newDecideLab(t)
	(&route{below: true, wrapper: lab.asWrapper}).install(t)
	watch := refusalWatch{dir: lab.dir, to: "api", id: questionID, self: "web", epoch: lab.web.Epoch()}
	refused := make(chan refusal, 1)
	wait, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go watchRefusal(wait, watch, refused, cancel)
	if code, out, errOut := run("decide", "api", lab.report.ID, "--arrived"); code != ExitOK {
		t.Fatalf("decide: exit %d\n%s%s", code, out, errOut)
	}
	var why refusal
	select {
	case why = <-refused:
	case <-wait.Done():
		t.Fatal("the wait did not end on the closure")
	}
	if why.closed == nil || why.closed.Decision != lab.decision(t).ID {
		t.Fatalf("ended by %+v", why)
	}
	for _, asJSON := range []bool{false, true} {
		var out bytes.Buffer
		ctx := &Context{Stdout: &out, Stderr: &out, JSON: asJSON}
		err := printClosed(ctx, lab.api, sendModel{ID: questionID, To: "api", From: "web"}, why.closed)
		var exit *ExitCodeError
		if !errors.As(err, &exit) || exit.Code != ExitFailed {
			t.Fatalf("printClosed: %v", err)
		}
		if !strings.Contains(out.String(), "closed by decision "+why.closed.Decision) || strings.Contains(out.String(), "the answer") {
			t.Fatalf("printed:\n%s", out.String())
		}
		var model sendModel
		if asJSON && (json.Unmarshal(out.Bytes(), &model) != nil || model.State != closedState || model.Answer != "" || model.Decided == nil) {
			t.Fatalf("the model:\n%s", out.String())
		}
	}
}

// A decided report whose tail has not run leaves the wait owed, and both
// sides say so.
func TestDecidedStillOwedIsSaid(t *testing.T) {
	d := &awaitedDecision{ID: "D", Report: "R", Choice: mail.ChoiceNotArrived, By: deciderModel{Kind: mail.ByPerson}, Delivery: "unproven", Resent: "R2"}
	view := awaitedView{State: mail.StageOwed, Decided: d}
	if got := awaitedState("api", view); got != "decided, still owed: report R did not arrive by decision D of the person (unverified), sent again as R2" {
		t.Fatalf("awaited: %q", got)
	}
	if !view.coming() {
		t.Fatal("a wait still owed reads as over")
	}
	lines := strings.Join(owedLines([]owedView{{messageView: messageView{Message: mail.Message{ID: "Q", From: "web", Kind: mail.Question, Text: "is it done?"}}, Kept: true, Decided: d}}), "\n")
	if !strings.Contains(lines, "decided, still owed: your report R did not arrive by decision D of the person (unverified)") {
		t.Fatalf("owed:\n%s", lines)
	}
}

// The list says which session stopped and how a decision settles it, and
// stops saying so once one has.
func TestTheListNamesAStoppedMailbox(t *testing.T) {
	lab := newDecideLab(t)
	(&route{}).install(t)
	_, out, _ := run("list")
	if !strings.Contains(out, "api stopped changing its mailbox: 1 report a decision can settle (rewake decide api).") {
		t.Fatalf("the list:\n%s", out)
	}
	var model listModel
	_, out, _ = run("list", "--json")
	if err := json.Unmarshal([]byte(out), &model); err != nil {
		t.Fatal(err)
	}
	for _, session := range model.Sessions {
		if stopped := session.Stopped; (session.Name == "api") != (stopped != nil) || stopped != nil && (len(stopped.Decidable) != 1 || stopped.Decidable[0] != lab.report.ID) {
			t.Fatalf("%s listed as stopped on %+v", session.Name, stopped)
		}
	}
	if code, out, errOut := run("decide", "api", lab.report.ID, "--arrived"); code != ExitOK {
		t.Fatalf("decide: exit %d\n%s%s", code, out, errOut)
	}
	if _, out, _ = run("list"); strings.Contains(out, "stopped changing") {
		t.Fatalf("a settled mailbox still listed as stopped:\n%s", out)
	}
}
