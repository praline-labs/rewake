package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/core/space"
	"github.com/praline-labs/rewake/internal/core/taskbrief"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// roomCommand is the one call rewake's shell makes each second
// (docs/shell.md#the-prefix-and-the-room). It is hidden like statusline: the
// shell is its only caller, and a documented command would promise agents a
// shape that the shell's own needs would then hold still. The shape lives in
// room_view.schema.json, from which the shell's types and this command's are
// generated.
func roomCommand() *Command {
	return &Command{
		Name:    "room",
		Summary: "The room as rewake's shell draws it: spaces, sessions with their verdicts, and on request the briefs.",
		Hidden:  true,
		Options: []Option{
			jsonOption,
			{Flag: "--wrapper", Value: "<pid>", Summary: "The pid of main's wrapper, a child of the shell calling this; proved, it makes the caller main's shell."},
			{Flag: "--briefs", Summary: "Add the room's briefs, to main's shell."},
			{Flag: "--brief", Value: "<id>", Summary: "Add one brief with its text and comments, to main's shell."},
		},
		Examples: []string{"rewake room --json", "rewake room --json --briefs"},
		Notes: []string{
			"The shell is main's terminal, not a session, so it proves itself by the process tree: main's wrapper is the child of the shell that calls this. Telemetry and briefs, which only main reads, are carried only then (asMain).",
		},
		Access:  viewForm,
		Handler: handleRoom,
	}
}

// shellParent is the process that called rewake room: the shell, when it
// is the shell's own call. A variable so a test can name its own process.
var shellParent = os.Getppid

func handleRoom(ctx *Context, call Call) error {
	dir, err := state.Dir()
	if err != nil {
		return &UsageError{Command: call.Command, Message: err.Error()}
	}
	pid, err := optionalNumber(call, "wrapper")
	if err != nil {
		return err
	}
	briefID, err := optionalNumber(call, "brief")
	if err != nil {
		return err
	}
	sessions, err := registry.ListReadOnly(dir)
	if err != nil {
		return failf("could not read the sessions in %s: %v", dir, err)
	}
	mainName, asMain := provenMain(sessions, pid)
	records, _, err := space.List(dir)
	if err != nil {
		return failf("could not read the spaces in %s: %v", dir, err)
	}
	view := RoomView{Room: filepath.Base(dir), AsMain: asMain, Spaces: spaceRowsOf(dir, records)}
	view.Sessions = sessionRowsOf(dir, records, sessionViewsOf(dir, sessions, asMain), mainName, asMain)
	if asMain {
		home := quietHomeOf(dir)
		if call.Switch("briefs") {
			var problem error
			view.Briefs, view.UnreadableBriefs, problem = briefRowsOf(home)
			if problem != nil {
				view.BriefsProblem = ptr(problem.Error())
			}
		}
		if briefID > 0 {
			view.Brief = briefOpened(home, briefID)
		}
	}
	return printValue(ctx, view, func() []string {
		return []string{fmt.Sprintf("room %s: %d spaces, %d sessions, %d briefs", view.Room, len(view.Spaces), len(view.Sessions), len(view.Briefs))}
	})
}

// optionalNumber is a flag's whole number, 0 where it is not given.
func optionalNumber(call Call, flag string) (int, error) {
	text, given := call.Flags[flag]
	if !given {
		return 0, nil
	}
	number, err := strconv.Atoi(text)
	if err != nil || number < 1 {
		return 0, &UsageError{Command: call.Command, Message: fmt.Sprintf("--%s takes a number from 1, not %q.", flag, text)}
	}
	return number, nil
}

// provenMain is the name of the main whose wrapper runs as pid below the
// process that called, when there is one: the shell is main's terminal and
// not a session, so what proves it main's is that it started main's
// wrapper. Telemetry and briefs are for no other caller.
func provenMain(sessions []registry.Session, pid int) (string, bool) {
	if pid == 0 {
		return "", false
	}
	for _, session := range sessions {
		if session.Role != role.Main.ID || session.ServicePID != pid {
			continue
		}
		parent, start, err := proc.Default.Parent(pid)
		if err != nil || parent != shellParent() || start != session.ServiceStart {
			return "", false
		}
		return session.Name, true
	}
	return "", false
}

// runStateOf is whether a run lives, as the views word it.
func runStateOf(evidence proc.IdentityEvidence) RunState {
	switch evidence {
	case proc.IdentityAlive:
		return RunStateRunning
	case proc.IdentityEnded:
		return RunStateEnded
	}
	return RunStateUnknown
}

// spaceRowsOf are the spaces not over, in the order they opened.
func spaceRowsOf(dir string, records []space.Record) []SpaceRow {
	rows := []SpaceRow{}
	for _, record := range records {
		if record.State.Final() {
			continue
		}
		row := SpaceRow{
			ID: record.ID, Name: record.Name, Worker: record.Worker.Name + "-" + record.Worker.Harness,
			State: SpaceState(record.State), MainRun: runStateOf(space.MainRuns(record)),
			Socket: state.SpaceSocketPath(dir, record.ID),
		}
		// The checkout counts once the worker has one: the shell shows its
		// branch in the strip.
		if record.Checkout.Path != "" {
			row.Checkout = &Checkout{Path: record.Checkout.Path}
			if record.Checkout.Branch != "" {
				row.Checkout.Branch = &record.Checkout.Branch
			}
		}
		rows = append(rows, row)
	}
	return rows
}

// briefRowsOf are the room's briefs, closed ones with them, and the entries
// that do not read as one; or why the list cannot be read, as brief list
// fails, so the shell never shows a list that failed as an empty one.
func briefRowsOf(home taskbrief.Home) ([]BriefView, []Problem, error) {
	briefs, problems, err := taskbrief.List(home, true)
	if err != nil {
		return nil, nil, err
	}
	rows := make([]BriefView, 0, len(briefs))
	for _, brief := range briefs {
		rows = append(rows, briefViewOf(briefSummaryOf(brief)))
	}
	unreadable := make([]Problem, 0, len(problems))
	for _, problem := range problems {
		unreadable = append(unreadable, Problem{Path: problem.Path, Why: problem.Why})
	}
	return rows, unreadable, nil
}

// briefOpened is one brief with its text and comments, nil where it does not
// read.
func briefOpened(home taskbrief.Home, id int) *BriefView {
	brief, err := taskbrief.Load(home, id)
	if err != nil {
		return nil
	}
	detail := briefDetailOf(brief)
	view := briefViewOf(detail.briefSummary)
	view.Text = &detail.Text
	view.Comments = make([]BriefComment, 0, len(detail.Comments))
	for _, comment := range detail.Comments {
		row := BriefComment{N: comment.N, From: comment.From, At: comment.At.Format(time.RFC3339), Text: comment.Text}
		if comment.Capacity != "" {
			row.Capacity = ptr(BindingKind(comment.Capacity))
		}
		view.Comments = append(view.Comments, row)
	}
	return &view
}

func briefViewOf(summary briefSummary) BriefView {
	view := BriefView{
		ID: summary.ID, Title: summary.Title, Closed: summary.Closed, Edition: summary.Edition,
		Done: summary.Done, Total: summary.Total, CommentCount: summary.CommentCount, Bound: []BriefBound{}, Skills: summary.Skills,
	}
	for _, bound := range summary.Bound {
		view.Bound = append(view.Bound, BriefBound{Name: bound.Name, Run: RunState(bound.Run), Kind: BindingKind(bound.Kind)})
	}
	return view
}
