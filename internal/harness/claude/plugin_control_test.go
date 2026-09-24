package claude

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/control"
)

// controlHost plays the module the parts of $ a control request uses, as
// strict as Claude Code 2.1.280 is about them (read in the binary and seen
// live, docs/research-claude-actions.md):
//
//   - $.clock.every(ms, fn) takes a non-negative number and a function. The
//     host never fires it on its own: a "tick" step runs every callback and
//     waits for it, so the test decides when the module polls.
//     $.clock.sleep(ms) takes the same number and resolves a twentieth of it
//     later; order records when it was asked for and when it resolved.
//   - $.fs takes positional arguments on real files; read of a missing file is
//     what the harness logs as an error, and is recorded as one. With
//     withdraw set, the asker gives up the moment the module marks a request
//     taken: the request is removed, or replaced by the next asker's.
//   - $.session.compact takes nothing or { instructions } and is refused while
//     a turn runs, while another compaction is in flight, or when compaction is
//     switched off; otherwise it runs the compaction itself, which refuses a
//     conversation too short (seen live on September 24, 2026) and answers
//     with the summary's messages, which must never leave the module. The
//     module's own session.compact handler is skipped for it, as re-entry:
//     the harness runs no plugin's handlers for an event that plugin's own
//     call raised (seen live on 2.1.280). A "compaction" step plays one the
//     module did not ask for, through its handler.
//   - $.turn.abort takes { turnId } of the running turn, and ends it with
//     turn.complete "aborted" before its promise settles.
//   - $.process.run resolves once its process has ended, which here is a
//     timer later; order records when each report was sent, when the host
//     compacted and when the module wrote an answer.
//
// Steps: ["event", name, fields], ["tick"], ["write", file, text] into the
// control directory, ["compaction", trigger], and ["reload"], which loads the
// module afresh and runs its session.start again, as the harness does after an
// edit.
const controlHost = hostRun + `
import { pathToFileURL } from "node:url"
import { readFileSync, writeFileSync, mkdirSync, statSync, readdirSync, unlinkSync } from "node:fs"
import { dirname, join } from "node:path"
const [modulePath, controlDir, stepsJSON, worldJSON] = process.argv.slice(2)
const world = JSON.parse(worldJSON)
const calls = [], errors = [], compacts = [], aborts = [], order = []
let timers = [], running, compacting = false, handlers, loads = 0
// The harness puts the plugin's name in front of every refusal it gives.
const refuse = (text) => { errors.push(text); return Promise.reject(new Error("rewake: " + text)) }
const path = (p) => typeof p === "string" && p !== ""
const $ = {
  process: { run: checkedRun((argv, init) => {
    const event = JSON.parse(init.stdin)
    calls.push(event)
    return new Promise((resolve) => setTimeout(() => { order.push("sent " + event.plugin_event); resolve({ exitCode: 0, stdout: "", stderr: "" }) }, 0))
  }) },
  env: { get: async () => undefined },
  settings: { read: async () => ({}) },
  clock: { every: (ms, fn) => {
    if (!(typeof ms === "number" && Number.isFinite(ms) && ms >= 0)) throw new Error("$.clock.every takes a non-negative number of milliseconds")
    if (typeof fn !== "function") throw new Error("$.clock.every takes a function")
    const timer = { fn, load: loads }
    timers.push(timer)
    return { cancel: () => { timers = timers.filter((t) => t !== timer) } }
  }, sleep: async (ms) => {
    if (!(typeof ms === "number" && Number.isFinite(ms) && ms >= 0)) return refuse("$.clock.sleep takes a non-negative number of milliseconds")
    order.push("sleep " + ms)
    await new Promise((resolve) => setTimeout(resolve, ms / 20))
    order.push("slept " + ms)
  } },
  fs: {
    exists: async (...a) => { if (a.length !== 1 || !path(a[0])) return refuse("$.fs.exists takes a path"); try { statSync(a[0]); return true } catch { return false } },
    read: async (...a) => {
      if (!path(a[0])) return refuse("$.fs.read takes a path")
      try { return readFileSync(a[0], "utf8") } catch (e) { return refuse("$.fs.read: " + a[0] + " failed: " + e.code) }
    },
    write: async (...a) => {
      if (a.length !== 2 || !path(a[0]) || typeof a[1] !== "string") return refuse("$.fs.write takes a path and a string")
      mkdirSync(dirname(a[0]), { recursive: true }); writeFileSync(a[0], a[1])
      if (a[0].endsWith(".result")) order.push("write " + a[0].slice(a[0].lastIndexOf("/") + 1))
      if (a[0].endsWith(".taken") && world.withdraw === "remove") unlinkSync(join(controlDir, "request.json"))
      if (a[0].endsWith(".taken") && world.withdraw === "replace") writeFileSync(join(controlDir, "request.json"), JSON.stringify({ id: "f".repeat(32), action: "compact", from: "lead" }))
    },
  },
  session: { compact: async (...a) => {
    const form = a.length === 0 || (a.length === 1 && a[0] !== null && typeof a[0] === "object" && Object.keys(a[0]).every((k) => k === "instructions") && typeof a[0].instructions === "string")
    if (!form) return refuse("$.session.compact takes { instructions } (a string) or nothing")
    if (running !== undefined) return refuse("$.session.compact: a turn is running (" + running + "); the conversation compacts between turns, so call it from turn.complete or later")
    if (compacting || world.inFlight) return refuse("$.session.compact: a turn is in flight; the conversation compacts between turns")
    if (world.compactOff) return refuse("$.session.compact: compaction is switched off in this session (DISABLE_COMPACT), for /compact and plugins alike")
    try {
      return await compaction("plugin", a.length === 0 ? null : a[0].instructions)
    } catch (e) {
      return refuse("$.session.compact: " + e.message)
    }
  } },
  turn: { abort: async (...a) => {
    if (a.length !== 1 || a[0] === null || typeof a[0] !== "object" || typeof a[0].turnId !== "string") return refuse("$.turn.abort takes { turnId }")
    if (running === undefined) return refuse("$.turn.abort: no turn is running (asked for " + a[0].turnId + ")")
    if (a[0].turnId !== running) return refuse("$.turn.abort: " + a[0].turnId + " is not the running turn (" + running + ")")
    aborts.push(a[0].turnId)
    await play("turn.complete", { turnId: running, reason: "aborted", isAborted: true, answer: "partial" })
  } },
}
// compaction runs one compaction, through the module's session.compact
// handler, which passes it on with next, unless the module's own call raised
// it.
async function compaction(trigger, instructions) {
  compacting = true
  try {
    const core = async (e) => {
      if (world.tooShort) throw new Error("Not enough messages to compact.")
      if (trigger === "plugin") compacts.push(instructions)
      order.push(trigger === "plugin" ? "compact" : "compact " + trigger)
      return { messages: [{ role: "user", text: "SECRET SUMMARY" }], tokensBefore: 120000, tokensAfter: 9000, usage: { input: 1 } }
    }
    const e = { trigger, ...(instructions !== null && { instructions }), messages: [{ role: "user", text: "SECRET" }] }
    const handler = trigger === "plugin" ? undefined : handlers["session.compact"]
    return typeof handler === "function" ? await handler($, e, core) : await core(e)
  } finally {
    compacting = false
  }
}
async function play(name, fields) {
  if (name === "turn.start" && fields.agentId === undefined) running = fields.turnId
  if (name === "turn.complete" && fields.agentId === undefined) running = undefined
  const handler = handlers[name]
  if (typeof handler === "function") await handler($, fields, () => "passed on")
}
async function load() {
  loads++
  const mod = await import(pathToFileURL(modulePath).href + "?load=" + loads)
  handlers = {}
  timers = timers.filter((t) => t.load === loads)
  mod.register((name, handler) => { handlers[name] = handler })
  await play("session.start", { cwd: "/w" })
}
await load()
for (const [step, a, b] of JSON.parse(stepsJSON)) {
  if (step === "event") await play(a, b)
  if (step === "tick") for (const timer of [...timers]) await timer.fn()
  if (step === "write") writeFileSync(join(controlDir, a), b)
  if (step === "reload") await load()
  if (step === "compaction") await compaction(a, null)
}
const files = {}
for (const name of readdirSync(controlDir)) files[name] = readFileSync(join(controlDir, name), "utf8")
process.stdout.write(JSON.stringify({ calls, errors, compacts, aborts, order, files, timers: timers.length, refused }))
`

type controlRun struct {
	Calls    []map[string]any  `json:"calls"`
	Errors   []string          `json:"errors"`
	Compacts []*string         `json:"compacts"`
	Aborts   []string          `json:"aborts"`
	Order    []string          `json:"order"`
	Files    map[string]string `json:"files"`
	Timers   int               `json:"timers"`
	Refused  []string          `json:"refused"`
}

type controlWorld struct {
	CompactOff bool   `json:"compactOff,omitempty"`
	InFlight   bool   `json:"inFlight,omitempty"`
	TooShort   bool   `json:"tooShort,omitempty"`
	Withdraw   string `json:"withdraw,omitempty"`
}

// runControl writes the plugin with a control directory (or none, when
// controlled is false), plays steps to its module under node, and returns
// what happened.
func runControl(t *testing.T, controlled bool, world controlWorld, steps ...[]any) controlRun {
	t.Helper()
	return runControlModule(t, controlled, world, nil, steps...)
}

// runControlModule is runControl with another module in place of the plugin's, when
// source is not nil: for holding the host itself to the harness's rules.
func runControlModule(t *testing.T, controlled bool, world controlWorld, source []byte, steps ...[]any) controlRun {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed; the module is not run")
	}
	dir := t.TempDir()
	controlDir := filepath.Join(dir, "control")
	if err := control.Prepare(controlDir); err != nil {
		t.Fatal(err)
	}
	baked := ""
	if controlled {
		baked = controlDir
	}
	if err := writePlugin(filepath.Join(dir, "plugin"), []string{"/bin/rewake", "observe", "/run/s.obs"}, baked); err != nil {
		t.Fatal(err)
	}
	module, err := os.ReadFile(filepath.Join(dir, "plugin", "hooks", "rewake.js"))
	if err != nil {
		t.Fatal(err)
	}
	if source != nil {
		module = source
	}
	if err := os.WriteFile(filepath.Join(dir, "rewake.mjs"), module, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "host.mjs"), []byte(controlHost), 0o600); err != nil {
		t.Fatal(err)
	}
	encodedSteps, _ := json.Marshal(steps)
	encodedWorld, _ := json.Marshal(world)
	out, err := exec.Command(node, filepath.Join(dir, "host.mjs"), filepath.Join(dir, "rewake.mjs"), controlDir, string(encodedSteps), string(encodedWorld)).CombinedOutput()
	if err != nil {
		t.Fatalf("the module failed: %v\n%s", err, out)
	}
	var result controlRun
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if len(result.Refused) != 0 {
		t.Fatalf("the host refused the module's process calls: %v", result.Refused)
	}
	return result
}

const (
	idA = "0123456789abcdef0123456789abcdef"
	idB = "fedcba9876543210fedcba9876543210"
)

func asked(id, action, focus string) []any {
	encoded, _ := json.Marshal(control.Request{ID: id, Action: action, Focus: focus, From: "lead"})
	return []any{"write", "request.json", string(encoded)}
}

func answerIn(t *testing.T, run controlRun, id string) map[string]any {
	t.Helper()
	raw, ok := run.Files[id+".result"]
	if !ok {
		t.Fatalf("no answer to %s; files %v, host errors %v", id, run.Files, run.Errors)
	}
	var answer map[string]any
	if err := json.Unmarshal([]byte(raw), &answer); err != nil {
		t.Fatalf("answer %q: %v", raw, err)
	}
	if _, taken := run.Files[id+".taken"]; !taken {
		t.Fatalf("answered %s without taking it first", id)
	}
	return answer
}

var tick = []any{"tick"}

// An idle session compacts, with the focus as the instructions or with no
// argument at all, and answers with the counts only: the summary never
// leaves the module. Polling with nothing waiting asks whether the file
// exists and reads nothing, so the host logs no error.
func TestTheModuleCompactsAnIdleSession(t *testing.T) {
	run := runControl(t, true, controlWorld{}, tick, tick, asked(idA, control.Compact, "keep the plan"), tick, tick, asked(idB, control.Compact, ""), tick)
	if len(run.Errors) != 0 {
		t.Fatalf("host errors: %v", run.Errors)
	}
	if len(run.Compacts) != 2 || run.Compacts[0] == nil || *run.Compacts[0] != "keep the plan" || run.Compacts[1] != nil {
		t.Fatalf("compacted with %v", run.Compacts)
	}
	for _, id := range []string{idA, idB} {
		want := map[string]any{"id": id, "outcome": "done", "tokensBefore": 120000.0, "tokensAfter": 9000.0}
		if got := answerIn(t, run, id); !reflect.DeepEqual(got, want) {
			t.Fatalf("answer %v, want %v", got, want)
		}
	}
	for name, content := range run.Files {
		if strings.Contains(content, "SECRET") {
			t.Fatalf("%s carries the summary: %s", name, content)
		}
	}
}

// The host's refusals come back as rewake's reasons with the host's text.
func TestTheModulePassesTheHostsRefusalsOn(t *testing.T) {
	busy := runControl(t, true, controlWorld{}, []any{"event", "turn.start", map[string]any{"turnId": "t1"}}, asked(idA, control.Compact, ""), tick)
	answer := answerIn(t, busy, idA)
	if answer["outcome"] != "refused" || answer["reason"] != control.InTurn || !strings.Contains(answer["detail"].(string), "a turn is running (t1)") || len(busy.Aborts) != 0 {
		t.Fatalf("mid-turn: %v, aborts %v", answer, busy.Aborts)
	}
	off := runControl(t, true, controlWorld{CompactOff: true}, asked(idA, control.Compact, ""), tick)
	if answer := answerIn(t, off, idA); answer["reason"] != control.CompactionOff {
		t.Fatalf("switched off: %v", answer)
	}
	short := runControl(t, true, controlWorld{TooShort: true}, asked(idA, control.Compact, ""), tick)
	if answer := answerIn(t, short, idA); answer["outcome"] != "refused" || answer["reason"] != control.NothingToCompact {
		t.Fatalf("too short: %v", answer)
	}
	flight := runControl(t, true, controlWorld{InFlight: true}, asked(idA, control.Compact, ""), tick)
	if answer := answerIn(t, flight, idA); answer["outcome"] != "refused" || answer["reason"] != control.InTurn || !strings.Contains(answer["detail"].(string), "a turn is in flight") {
		t.Fatalf("another compaction in flight: %v", answer)
	}
	idle := runControl(t, true, controlWorld{}, asked(idA, control.Interrupt, ""), tick)
	if answer := answerIn(t, idle, idA); answer["reason"] != control.NoTurn || len(idle.Aborts) != 0 {
		t.Fatalf("interrupting an idle session: %v, aborts %v", answer, idle.Aborts)
	}
	ended := runControl(t, true, controlWorld{}, []any{"event", "turn.start", map[string]any{"turnId": "t1"}},
		[]any{"event", "turn.start", map[string]any{"turnId": "s1", "agentId": "a1"}}, asked(idA, control.Interrupt, ""), tick)
	if answer := answerIn(t, ended, idA); answer["outcome"] != "done" || !reflect.DeepEqual(ended.Aborts, []string{"t1"}) {
		t.Fatalf("a subagent's turn is not the one aborted: %v, aborts %v", answer, ended.Aborts)
	}
}

// An interrupt aborts the running turn, and its end is reported as the
// main's; a person's Esc afterwards is the person's.
func TestTheModuleReportsWhoInterrupted(t *testing.T) {
	run := runControl(t, true, controlWorld{},
		[]any{"event", "turn.start", map[string]any{"turnId": "t1"}}, asked(idA, control.Interrupt, ""), tick,
		[]any{"event", "turn.start", map[string]any{"turnId": "t2"}},
		[]any{"event", "turn.complete", map[string]any{"turnId": "t2", "reason": "aborted", "isAborted": true}})
	if answer := answerIn(t, run, idA); answer["outcome"] != "done" || !reflect.DeepEqual(run.Aborts, []string{"t1"}) {
		t.Fatalf("answer %v, aborts %v", answer, run.Aborts)
	}
	var ends []map[string]any
	for _, call := range run.Calls {
		if call["plugin_event"] == "turn.complete" {
			ends = append(ends, call)
		}
	}
	want := []map[string]any{
		{"plugin_event": "turn.complete", "turn_id": "t1", "reason": "aborted", "by": "lead"},
		{"plugin_event": "turn.complete", "turn_id": "t2", "reason": "aborted"},
	}
	if !reflect.DeepEqual(ends, want) {
		t.Fatalf("reported %v, want %v", ends, want)
	}
}

// A request is carried out once: not again at the next tick, not after a
// reload of the module, and not at all with an id of another shape.
func TestTheModuleCarriesARequestOutOnce(t *testing.T) {
	run := runControl(t, true, controlWorld{}, asked(idA, control.Compact, ""), tick, tick, []any{"reload"}, tick, tick)
	if len(run.Compacts) != 1 || run.Timers != 1 {
		t.Fatalf("compacted %d times, %d timers after the reload", len(run.Compacts), run.Timers)
	}
	odd := runControl(t, true, controlWorld{}, []any{"write", "request.json", `{"id":"../x","action":"compact","from":"lead"}`}, tick)
	if len(odd.Compacts) != 0 || len(odd.Files) != 1 {
		t.Fatalf("an odd id: compacted %d, files %v", len(odd.Compacts), odd.Files)
	}
}

// Without a control directory the module polls nothing.
func TestTheModuleWithoutControlPollsNothing(t *testing.T) {
	run := runControl(t, false, controlWorld{}, asked(idA, control.Compact, ""), tick)
	if run.Timers != 0 || len(run.Compacts) != 0 {
		t.Fatalf("%d timers, %d compactions", run.Timers, len(run.Compacts))
	}
}

// A request the asker withdrew as the module marked it taken — removed, or
// replaced by the next asker's — is answered as withdrawn and not carried out:
// the asker, looking for the mark after withdrawing, waits for that answer.
func TestTheModuleDoesNotCarryOutAWithdrawnRequest(t *testing.T) {
	for _, withdraw := range []string{"remove", "replace"} {
		run := runControl(t, true, controlWorld{Withdraw: withdraw},
			[]any{"event", "turn.start", map[string]any{"turnId": "t1"}}, asked(idA, control.Interrupt, ""), tick)
		answer := answerIn(t, run, idA)
		if answer["outcome"] != "refused" || answer["reason"] != control.Withdrawn || len(run.Aborts) != 0 {
			t.Fatalf("%s: %v, aborts %v", withdraw, answer, run.Aborts)
		}
		if len(run.Errors) != 0 {
			t.Fatalf("%s: host errors %v", withdraw, run.Errors)
		}
	}
}
