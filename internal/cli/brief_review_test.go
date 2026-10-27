package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/taskbrief"
)

// send --brief --review gives the brief to read and comment only: the
// reviewer is told so with the letter, its tick is refused, and its comment
// shows its author and capacity to every reader, its markdown line for line.
// Given again without --review, the same run works on it.
const reviewNote = "Move it is not done.\n\n- the login still calls the old path\n- no test covers it"

func TestAReviewerCommentsAndIsRefusedATick(t *testing.T) {
	dir, web, api := briefSender(t)
	reviewer := taskbrief.Author{Name: api.Name, Epoch: api.Epoch()}
	id := sendOK(t, "api", "review the login", "--brief", "1", "--review")
	if brief, err := taskbrief.Load(taskbrief.Home{Room: dir}, 1); err != nil || brief.Bound[0].Kind != taskbrief.Review {
		t.Fatalf("bound = %+v, %v", brief.Bound, err)
	}
	leaveUnread(t, dir, mail.Message{ID: id, From: web.Name, FromEpoch: web.Epoch(), To: api.Name, ToEpoch: api.Epoch(), Kind: mail.Task, Text: "review the login", Brief: "1"})

	as(t, api)
	if _, out, _ := run("inbox"); !strings.Contains(out, "given to you to review") || strings.Contains(out, "brief tick") {
		t.Fatalf("the reviewer's inbox = %q", out)
	}
	if code, _, errOut := run("brief", "tick", "1", "1", "--edition", "1"); code != ExitFailed || !strings.Contains(errOut, "to review") || !strings.Contains(errOut, "ticking is the worker's") {
		t.Fatalf("the reviewer's tick: %d %q", code, errOut)
	}
	if code, _, errOut := run("brief", "comment", "1", "--", reviewNote); code != ExitOK {
		t.Fatalf("the reviewer's comment: %d %q", code, errOut)
	}
	if _, out, _ := run("brief", "show", "1"); !strings.Contains(out, "\napi · review · ") || !strings.Contains(out, "bound: api (review)") ||
		!strings.Contains(out, " · #1\n  Move it is not done.\n\n  - the login still calls the old path\n  - no test covers it\n") {
		t.Fatalf("the reviewer's show = %q", out)
	}
	_, raw, _ := run("brief", "show", "1", "--json")
	var model struct {
		Bound    []struct{ Kind string }                 `json:"bound"`
		Comments []struct{ From, Capacity, Text string } `json:"comments"`
	}
	if err := json.Unmarshal([]byte(raw), &model); err != nil || model.Bound[0].Kind != "review" || model.Comments[0].From != "api" || model.Comments[0].Capacity != "review" || model.Comments[0].Text != reviewNote {
		t.Fatalf("show --json = %s (%v)", raw, err)
	}

	as(t, web)
	if _, out, _ := run("brief", "show", "1"); !strings.Contains(out, "\napi · review · ") {
		t.Fatalf("main's show = %q", out)
	}
	sendOK(t, "api", "now fix it", "--brief", "1")
	brief, err := taskbrief.Load(taskbrief.Home{Room: dir}, 1)
	if binding, _ := brief.BindingOf(reviewer); err != nil || len(brief.Bound) != 1 || binding.Kind != taskbrief.Work {
		t.Fatalf("given again as work, bound = %+v, %v", brief.Bound, err)
	}
}
