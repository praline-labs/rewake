package workflow

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

// The controls of the tool scenarios and of the timed turn start: a product
// mutant for each claim, each required to break its own observation and
// nothing else. Five refuse one tool's calls at the endpoint, a shared shape
// named by the tool; the read has two, one that keeps the letters' text from
// the answer while the read still commits, one that never acknowledges a read
// the answer showed. The crosswise check runs every scenario of the family in
// every other control's world.

// toolRefusedFile is where a refusal mutant refuses: the endpoint, before the
// call's words are made.
const toolRefusedFile = "internal/bridge/endpoint/transport.go"

// toolRefused is the edit that refuses every call of one tool and runs
// nothing, as a tool that does nothing would answer.
func toolRefused(tool string) []edit {
	first := "\ttool, ok := bridge.FindTool(e.cfg.Tools, call.Tool)\n"
	return []edit{{first, "\tif call.Tool == \"" + tool + "\" {\n\t\treturn substitute(\"Rewake: the control refuses this tool, so nothing ran.\\n\").encoded()\n\t}\n" + first}}
}

var (
	mutantToolSendRefused    = mutation{name: "tool-send-refused", file: toolRefusedFile, edits: toolRefused("send")}
	mutantToolPendingRefused = mutation{name: "tool-pending-refused", file: toolRefusedFile, edits: toolRefused("pending")}
	mutantToolWhoamiRefused  = mutation{name: "tool-whoami-refused", file: toolRefusedFile, edits: toolRefused("whoami")}
	mutantToolRetryRefused   = mutation{name: "tool-retry-refused", file: toolRefusedFile, edits: toolRefused("retry")}
	mutantToolListRefused    = mutation{name: "tool-list-refused", file: toolRefusedFile, edits: toolRefused("list")}
	// The read's answer keeps its headers and loses the letters' text; the
	// digest is taken of what is shown, so the read still commits.
	mutantToolInboxUnshown = mutation{
		name: "tool-inbox-unshown",
		file: "internal/cli/inbox_parts_emit.go",
		edits: []edit{{
			"\t\ttext = renderRead(batch.JSON, site.self.Name, *record, segments, replay)\n",
			"\t\ttext = renderRead(batch.JSON, site.self.Name, *record, segments, replay)\n" +
				"\t\tif ctx.scope != nil {\n\t\t\tvar kept []string\n\t\t\tfor _, line := range strings.SplitAfter(text, \"\\n\") {\n" +
				"\t\t\t\tif strings.HasPrefix(line, \"Rewake:\") {\n\t\t\t\t\tkept = append(kept, line)\n\t\t\t\t}\n\t\t\t}\n" +
				"\t\t\ttext = strings.Join(kept, \"\")\n\t\t}\n",
		}},
	}
	// The result of a call reaches the endpoint, and no read is acknowledged.
	mutantToolReadUnacknowledged = mutation{
		name:  "tool-read-unacknowledged",
		file:  "internal/bridge/endpoint/input.go",
		edits: []edit{{"\tif !used || e.cfg.Acknowledge == nil {\n", "\tif used || !used {\n"}},
	}
	// The adapter passes the endpoint no time for the program's start.
	mutantTurnStartUntimed = mutation{
		name:  "turn-start-untimed",
		file:  "internal/harness/fixture/turns.go",
		edits: []edit{{"b.handler.Tool.TurnStarted(thread, frame.Turn, frame.At)", "b.handler.Tool.TurnStarted(thread, frame.Turn, 0)"}},
	}
)

// toolControl is one control: the scenario it belongs to, its mutant, and
// the observations it must break.
type toolControl struct {
	scenario string
	mutant   mutation
	breaks   []string
}

var toolControls = []toolControl{
	{"tool-inbox", mutantToolInboxUnshown, []string{obsToolInboxShows}},
	{"tool-inbox", mutantToolReadUnacknowledged, []string{obsToolInboxReads}},
	{"tool-send", mutantToolSendRefused, []string{obsToolSendOnce}},
	{"tool-pending", mutantToolPendingRefused, []string{obsToolPendingMark}},
	{"tool-whoami", mutantToolWhoamiRefused, []string{obsToolWhoamiNames}},
	{"tool-retry", mutantToolRetryRefused, []string{obsToolRetryJoins}},
	{"tool-list", mutantToolListRefused, []string{obsToolListSessions}},
	{"fixture-turn-start", mutantTurnStartUntimed, []string{obsTurnStartRecorded}},
}

// toolFamily is every scenario the controls cross: the six tools and the
// timed start, each by name with how it plays.
func toolFamily() map[string]func(*testing.T, *Case, *Isolation) []telemetryFinding {
	family := map[string]func(*testing.T, *Case, *Isolation) []telemetryFinding{"fixture-turn-start": playTurnStart}
	for _, scenario := range toolScenarios {
		family[scenario.name] = func(t *testing.T, c *Case, iso *Isolation) []telemetryFinding {
			return playToolScenario(t, c, iso, fixtureColumn, scenario)
		}
	}
	return family
}

func TestToolControls(t *testing.T) {
	runParallel(t)
	family := toolFamily()
	for _, control := range toolControls {
		t.Run(control.mutant.name, func(t *testing.T) {
			runFindingsControlOn(t, fixtureColumn.harness, control.scenario, family[control.scenario], control.mutant, control.breaks...)
		})
	}
}

// crossWithout names the cells left out on purpose, by control: a scenario
// that cannot be judged in that control's world. The retry scenario retries a
// heads-up it first sends through the tool, so with send refused it has
// nothing to retry; its answer is "cannot judge", and a cell that must come
// out unjudgeable is not a control.
var crossWithout = map[string][]string{
	mutantToolSendRefused.name: {"tool-retry"},
}

// TestToolControlsCrosswise runs every scenario of the family under every
// control of another scenario: each of its observations must stand.
func TestToolControlsCrosswise(t *testing.T) {
	if os.Getenv(crossSwitch) == "" {
		t.Skipf("crosswise check skipped: set %s=1 to run every control against every other world", crossSwitch)
	}
	enterScenarioAround(t, "tool-crosswise")
	family := toolFamily()
	names := make([]string, 0, len(family))
	for name := range family {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, control := range toolControls {
		for _, scenario := range names {
			if scenario == control.scenario || slices.Contains(crossWithout[control.mutant.name], scenario) {
				continue
			}
			t.Run(scenario+"-under-"+control.mutant.name, func(t *testing.T) {
				runToolCross(t, scenario, family[scenario], control.mutant)
			})
		}
	}
}

// runToolCross is one cell of the crosswise check: the scenario played under
// another scenario's mutant, every observation required to hold.
func runToolCross(t *testing.T, scenario string, play func(*testing.T, *Case, *Isolation) []telemetryFinding, mutant mutation) {
	t.Helper()
	name := scenario + "-cross-" + mutant.name
	enterScenario(t, name)
	want := "every observation of " + scenario + " stands under the " + mutant.name + " mutant"
	c := Start(t, Spec{Name: name, Harness: fixtureColumn.harness, Observations: []string{want}, Deadline: 120 * time.Second})
	binary, err := buildMutant(c, mutant)
	if err != nil {
		c.Contradicted(want, "the mutant could not be built: %v", err)
		return
	}
	var wrong, held []string
	for _, finding := range play(t, c, Isolate(t, c, binary)) {
		switch {
		case !finding.judged:
			wrong = append(wrong, "could not judge "+finding.observation+": "+finding.detail)
		case !finding.held:
			wrong = append(wrong, finding.observation+" broke: "+finding.detail)
		default:
			held = append(held, finding.observation)
		}
	}
	if len(held) == 0 && len(wrong) == 0 {
		wrong = append(wrong, "the scenario made no observation")
	}
	if len(wrong) > 0 {
		c.Contradicted(want, "%s", strings.Join(wrong, "; "))
		return
	}
	c.Observed(want, fmt.Sprintf("%d held: %s", len(held), strings.Join(held, "; ")))
}
