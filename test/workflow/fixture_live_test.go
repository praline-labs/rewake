package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/channel"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/sessionstate"
)

// The live set and the channel record on the fixture column (S11,
// docs/v2/stage3-steps-adapters.md#s11-the-host-on-the-live-set-codex): each
// run's record carries what its readiness exchange proved, and the decisions
// other sessions make about it read that, never the offer. Every absence goes
// through the program's own switch:
//
//   - full proves everything: the tool connected, the briefing with the whole
//     transport sentence; stopped, its channel record stays frozen at the exit.
//   - wakeless withholds Wake: a note and a task to it are accepted at once,
//     exit 3, and wait readable in its inbox.
//   - toolless withholds the tool: said at launch, the shell's clause alone in
//     its briefing, the tool offered and unproven in its record, never no
//     tool, and its shell read seen there.
//   - dropper closes its connection after its first turn: the whole set is
//     withdrawn at once, its channel fails, and a task to it fails, exit 1.
//   - denied makes one call the harness refuses for permission: the block.
//   - stuck withholds the boundary as a worker: its launch is refused.
//
// The steering refusal without a live Control is fixture-steered's bare.

const (
	obsLiveRecorded  = "each run's record carries what its exchange proved: everything for a full run, nothing it withheld"
	obsLiveUnwoken   = "a letter to a run without a live Wake is accepted at once, exit 3, saying no notice can reach it, and waits readable in its inbox, a task as a note"
	obsLiveDropped   = "a connection lost after a turn withdraws the run's whole live set at once, and a task to it then fails, exit 1, with nothing sent"
	obsLiveLaunch    = "a worker whose run proves no turn boundary is refused at launch, exit 1, saying so, and leaves no session"
	obsChannelLive   = "a run with the tool live shows it connected, and its briefing carries the whole transport sentence"
	obsChannelNone   = "a run whose harness does not make the tool live at its start says so at launch, its briefing carries the shell's clause alone, and its record keeps the tool offered and unproven, never no tool"
	obsChannelShell  = "a run without the tool that reads its mail in the shell has the shell's evidence in its record"
	obsChannelLost   = "a run whose connection is lost shows its mail tool failing, the server gone"
	obsChannelDenied = "a run whose harness denies the tool for permission shows the tool denied by policy"
	obsChannelFrozen = "a run that ended keeps its channel record frozen at the exit, not failing"

	liveTask   = "fixture-live: do the work and report"
	liveNote   = "fixture-live: a heads-up"
	liveSecond = "fixture-live: a second task"
	// The briefing's transport sentence (internal/brief): its first clause
	// is every run's, the rest only a run whose tool is live.
	transportClause = "Run `rewake <words>` through the `rewake` tool when you have it, otherwise in the shell."
	transportRest   = "A call the tool answered \"nothing ran\" may be made again in the shell."
)

var liveObservations = []string{
	obsLiveRecorded, obsLiveUnwoken, obsLiveDropped, obsLiveLaunch,
	obsChannelLive, obsChannelNone, obsChannelShell, obsChannelLost, obsChannelDenied, obsChannelFrozen,
}

func TestFixtureLive(t *testing.T) {
	binary := enterScenario(t, "fixture-live")
	c := Start(t, Spec{
		Name:         "fixture-live",
		Harness:      fixtureColumn.harness,
		Observations: liveObservations,
		Deadline:     120 * time.Second,
	})
	for _, finding := range playLive(t, c, Isolate(t, c, binary)) {
		if finding.held {
			c.Observed(finding.observation, finding.detail)
		} else {
			c.Contradicted(finding.observation, "%s", finding.detail)
		}
	}
}

// liveRow is one session of main's listing: its record, with its live set,
// and the channel record of its telemetry.
type liveRow struct {
	registry.Session
	Telemetry *struct {
		Channel *channel.Record `json:"channel"`
	} `json:"telemetry"`
}

func (r liveRow) channel() *channel.Record {
	if r.Telemetry == nil {
		return nil
	}
	return r.Telemetry.Channel
}

// liveWorld is the case's sessions and main's view of them.
type liveWorld struct {
	c    *Case
	iso  *Isolation
	asks *requests
	home string
}

// rows is main's listing now, by name.
func (w liveWorld) rows() map[string]liveRow {
	code, out, ok := w.asks.ask(w.c, "list", "--json")
	var listed struct {
		Sessions []liveRow `json:"sessions"`
	}
	if !ok || code != 0 || json.Unmarshal([]byte(out), &listed) != nil {
		return nil
	}
	rows := map[string]liveRow{}
	for _, row := range listed.Sessions {
		rows[row.Name] = row
	}
	return rows
}

// await waits for a session's row to satisfy held, and answers the last row seen.
func (w liveWorld) await(window time.Duration, name string, held func(liveRow) bool) (liveRow, bool) {
	var last liveRow
	ok := waitFor(w.c, window, func() bool {
		row, found := w.rows()[name]
		last = row
		return found && held(row)
	})
	return last, ok
}

func (w liveWorld) brief(label string) string {
	raw, _ := os.ReadFile(filepath.Join(w.home, label+".brief"))
	return string(raw)
}

func showChannel(r *channel.Record) string {
	if r == nil {
		return "no channel record"
	}
	return fmt.Sprintf("%q (tool %s, class %q, reason %q, frozen %t)", channel.Label(r), r.Tool, r.Class, r.Reason, r.Frozen)
}

var fullLive = []string{"wake", "turn-boundary", "telemetry", "control", "tool-transport"}

func playLive(t *testing.T, c *Case, iso *Isolation) []telemetryFinding {
	t.Helper()
	col := fixtureColumn.harness
	briefOf := func(label string) string { return shimBriefFile + "=" + filepath.Join(iso.Home, label+".brief") }
	denied, _ := json.Marshal([]fixtureToolCall{{Tool: "whoami", Denied: true}})

	full := startHarnessSession(t, c, iso, col, "full", "--general", briefOf("full"), staysUp(iso, "full"))
	defer stopSession(t, c, full)
	wakeless := startHarnessSession(t, c, iso, col, "wakeless", "--general", shimNoWake+"=1", staysUp(iso, "wakeless"))
	defer stopSession(t, c, wakeless)
	toolless := startHarnessSession(t, c, iso, col, "toolless", "--general", shimNoTool+"=1", briefOf("toolless"), shimInboxJSON+"=1", staysUp(iso, "toolless"))
	defer stopSession(t, c, toolless)
	dropper := startHarnessSession(t, c, iso, col, "dropper", "--general", shimDropAfter+"=1", shimInboxJSON+"=1", staysUp(iso, "dropper"))
	defer stopSession(t, c, dropper)
	deny := startHarnessSession(t, c, iso, col, "denied", "--general", shimToolCalls+"="+string(denied), shimInboxJSON+"=1", staysUp(iso, "denied"))
	defer stopSession(t, c, deny)
	asks := newRequests(iso, "lead")
	lead := startHarnessSession(t, c, iso, col, "lead", "--main", shimInboxJSON+"=1", asks.env())
	defer stopSession(t, c, lead)
	w := liveWorld{c: c, iso: iso, asks: asks, home: iso.Home}

	var out []telemetryFinding
	finding := func(observation string, held bool, detail string, args ...any) {
		out = append(out, telemetryFinding{observation: observation, held: held, judged: true, detail: fmt.Sprintf(detail, args...)})
	}
	for _, session := range []*scenarioSession{full, wakeless, toolless, dropper, deny, lead} {
		if !sessionReady(c, session) {
			for _, observation := range liveObservations {
				out = append(out, telemetryFinding{observation: observation, detail: session.name + " never became ready"})
			}
			return out
		}
	}

	code, said, refused := refusedLaunch(t, c, iso, col, "stuck", "--general", shimNoBoundary+"=1")
	_, stayed := w.rows()["stuck-"+col]
	finding(obsLiveLaunch, refused && code == 1 && strings.Contains(said, "did not make its turn boundary live") && !stayed,
		"exit %d (ended %t), left a session %t: %s", code, refused, stayed, said)

	rows := w.rows()
	recorded := slices.Equal(rows[full.name].Live, fullLive) && !slices.Contains(rows[wakeless.name].Live, "wake") &&
		slices.Contains(rows[wakeless.name].Live, "turn-boundary") && !slices.Contains(rows[toolless.name].Live, "tool-transport")
	finding(obsLiveRecorded, recorded, "full %v, wakeless %v, toolless %v", rows[full.name].Live, rows[wakeless.name].Live, rows[toolless.name].Live)

	unwoken, detail := w.unwoken(wakeless, rows[wakeless.name].Epoch())
	finding(obsLiveUnwoken, unwoken, "%s", detail)

	connected, _ := w.await(10*time.Second, full.name, func(r liveRow) bool { return r.channel() != nil && r.channel().Tool == channel.ToolConnected })
	fullBrief := w.brief("full")
	finding(obsChannelLive, connected.channel() != nil && connected.channel().Tool == channel.ToolConnected &&
		strings.Contains(fullBrief, transportClause) && strings.Contains(fullBrief, transportRest),
		"channel %s; briefing with the tool's rest %t", showChannel(connected.channel()), strings.Contains(fullBrief, transportRest))

	none := rows[toolless.name].channel()
	stderr, _ := os.ReadFile(filepath.Join(iso.Home, "toolless.stderr"))
	toollessBrief := w.brief("toolless")
	// Starting until the hello timer passes, failing with no hello after:
	// what the record shows of any offered tool that never connects.
	unproven := none != nil && none.Reason == "" && (none.Tool == channel.ToolStarting || none.Tool == channel.ToolFailing && none.Class == channel.ClassNoHello)
	told := strings.Contains(string(stderr), "did not make the rewake tool live at its start")
	finding(obsChannelNone, unproven && told &&
		strings.Contains(toollessBrief, transportClause) && !strings.Contains(toollessBrief, transportRest),
		"channel %s; said at launch %t; briefing %q", showChannel(none), told, firstLine(toollessBrief))

	asks.ask(c, "send", toolless.name, liveNote, "--notify")
	// The evidence alone, not the category it makes: whether the shell shows
	// depends on when the tool's interval opened, which is the keeper's
	// other transition.
	shell, _ := w.await(20*time.Second, toolless.name, func(r liveRow) bool { return r.channel() != nil && r.channel().Shell != nil })
	finding(obsChannelShell, shell.channel() != nil && shell.channel().Shell != nil && shell.channel().Shell.OK,
		"channel %s, shell %+v", showChannel(shell.channel()), shellOf(shell.channel()))

	out = append(out, playDropped(w, dropper)...)

	asks.ask(c, "send", deny.name, liveTask)
	blocked, _ := w.await(30*time.Second, deny.name, func(r liveRow) bool { return r.channel() != nil && r.channel().Blocked() })
	finding(obsChannelDenied, blocked.channel() != nil && channel.Label(blocked.channel()) == "mail: tool denied by policy", "channel %s", showChannel(blocked.channel()))

	epoch := rows[full.name].Epoch()
	stopSession(t, c, full)
	last := sessionstate.Load(filepath.Join(iso.StateDir, "rooms", "default"), full.name, epoch).Channel
	finding(obsChannelFrozen, last != nil && last.Frozen && last.Category() != channel.CategoryFailing, "the last snapshot's channel %s", showChannel(last))
	return out
}

// playDropped sends dropper its task, waits for the report and the drop, and
// judges what the record and a second task show.
func playDropped(w liveWorld, dropper *scenarioSession) []telemetryFinding {
	unjudged := []telemetryFinding{{observation: obsLiveDropped}, {observation: obsChannelLost}}
	code, said, _ := w.asks.ask(w.c, "send", dropper.name, liveTask)
	if code != 0 && code != 3 {
		for i := range unjudged {
			unjudged[i].detail = fmt.Sprintf("the first task was not accepted: exit %d, %q", code, firstLine(said))
		}
		return unjudged
	}
	gone, withdrawn := w.await(30*time.Second, dropper.name, func(r liveRow) bool { return len(r.Live) == 0 })
	code, said, _ = w.asks.ask(w.c, "send", dropper.name, liveSecond)
	sent := inboxHolds(w.iso, dropper.name, liveSecond)
	lost, _ := w.await(10*time.Second, dropper.name, func(r liveRow) bool {
		return r.channel() != nil && r.channel().Category() == channel.CategoryFailing
	})
	return []telemetryFinding{
		{
			observation: obsLiveDropped, judged: true,
			held:   withdrawn && code == 1 && strings.Contains(said, "turn boundary is not live") && !sent,
			detail: fmt.Sprintf("live after the drop %v (withdrawn %t); the second task: exit %d, %q, in its inbox %t", gone.Live, withdrawn, code, firstLine(said), sent),
		},
		{
			observation: obsChannelLost, judged: true,
			held:   lost.channel() != nil && lost.channel().Category() == channel.CategoryFailing && lost.channel().Class == channel.ClassServerGone,
			detail: "channel " + showChannel(lost.channel()),
		},
	}
}

// inboxHolds says whether any letter in a session's inbox carries the text.
func inboxHolds(iso *Isolation, name, text string) bool {
	found, _ := filepath.Glob(filepath.Join(iso.StateDir, "rooms", "default", "inbox", name, "*.json"))
	for _, path := range found {
		if raw, err := os.ReadFile(path); err == nil && strings.Contains(string(raw), text) {
			return true
		}
	}
	return false
}

func shellOf(r *channel.Record) any {
	if r == nil || r.Shell == nil {
		return nil
	}
	return *r.Shell
}

// refusedLaunch launches a session expected to be refused and answers its
// exit code, its standard error, and whether it ended within the bound; one
// that did not is asked to stop.
func refusedLaunch(t *testing.T, c *Case, iso *Isolation, harness, name, role string, controls ...string) (int, string, bool) {
	t.Helper()
	session, launch := sessionLaunch(t, c, iso, "", harness, name, role, nil, nil, controls...)
	release := c.captureStderr(launch, iso.Home, name)
	process, err := c.start(launch, true)
	release()
	if err != nil {
		t.Fatalf("launching %s: %v", name, err)
	}
	session.process = process
	ended := waitFor(c, 20*time.Second, process.finished)
	if !ended {
		_ = session.stop(c)
	}
	_ = c.wait(process)
	stderr, _ := os.ReadFile(filepath.Join(iso.Home, name+".stderr"))
	code := -1
	if process.cmd.ProcessState != nil {
		code = process.cmd.ProcessState.ExitCode()
	}
	return code, string(stderr), ended
}

// unwoken sends a note and a task to a run without a live Wake and says
// whether each was accepted at once, exit 3, with the promise that the
// session reads it, and whether the promise holds once the run's server has tried it: the
// letter readable in the inbox, its stored status still waiting.
func (w *liveWorld) unwoken(session *scenarioSession, epoch string) (bool, string) {
	room := filepath.Join(w.iso.StateDir, "rooms", "default")
	held, details := true, []string{}
	for _, text := range []string{liveNote, liveTask} {
		args := []string{"send", session.name, text, "--json"}
		if text == liveNote {
			args = append(args, "--notify")
		}
		code, said, _ := w.asks.ask(w.c, args...)
		var accepted struct {
			ID     string `json:"id"`
			Detail string `json:"detail"`
		}
		_ = json.Unmarshal([]byte(said), &accepted)
		var status inbox.Status
		var unread []inbox.Message
		tried := accepted.ID != "" && waitFor(w.c, 8*time.Second, func() bool {
			status, _, _ = inbox.ReadStatus(room, session.name, accepted.ID)
			unread, _ = inbox.PeekUnread(room, session.name, epoch)
			// The coalescing window keeps a note pending before its try.
			return status.State == inbox.Failed || strings.Contains(status.Detail, "no notice can reach this session")
		})
		readable := slices.ContainsFunc(unread, func(m inbox.Message) bool { return m.ID == accepted.ID })
		// The answer is send's own, decided by the live set before any try:
		// a send that waited for the server's try would carry its detail.
		decided := strings.HasPrefix(accepted.Detail, "accepted; no notice can reach this session")
		ok := code == 3 && decided && tried && readable && status.State == inbox.Pending
		held = held && ok
		details = append(details, fmt.Sprintf("%q: exit %d, answered %q, readable %t, stored %s %q", text, code, accepted.Detail, readable, status.State, status.Detail))
	}
	return held, strings.Join(details, "; ")
}
