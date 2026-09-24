package claude

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/harness/claude/telemetry"
)

// A launch with a telemetry socket gets the plugin: written beside the socket,
// passed with --plugin-dir, and function hooks switched on for this launch
// only, through the environment. The person's own plugin directories stay.
func TestALaunchCarriesThePlugin(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "s.obs")
	args, env, notes := applyPlugin([]string{"--plugin-dir", "/theirs"}, []string{"HOME=/h"}, socket)
	dir := telemetry.PluginPath(socket)
	if len(notes) != 0 || !slices.Equal(harness.FlagValues(args, pluginDirFlag), []string{"/theirs", dir}) {
		t.Fatalf("args %v, notes %v", args, notes)
	}
	if value, set := lookupEnv(env, functionHooksEnv); !set || value != "1" || !slices.Contains(env, "HOME=/h") {
		t.Fatalf("env %v", env)
	}
	var manifest struct {
		Name string `json:"name"`
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".claude-plugin", "plugin.json"))
	if err != nil || json.Unmarshal(raw, &manifest) != nil || manifest.Name != "rewake" {
		t.Fatalf("manifest %s, %v", raw, err)
	}
	var hooks struct {
		Modules []string `json:"modules"`
	}
	raw, err = os.ReadFile(filepath.Join(dir, "hooks", "hooks.json"))
	if err != nil || json.Unmarshal(raw, &hooks) != nil || !slices.Equal(hooks.Modules, []string{"./rewake.js"}) {
		t.Fatalf("hooks %s, %v", raw, err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	module, err := os.ReadFile(filepath.Join(dir, "hooks", "rewake.js"))
	if err != nil || strings.Contains(string(module), pluginArgv) || !strings.Contains(string(module), "const argv = "+mustJSON(t, []string{executable, harness.Observe, socket})+"\n") {
		t.Fatalf("module %v:\n%s", err, module)
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// Where the plugin cannot load or the person switched function hooks off, it
// is not passed, and the note says interruptions go unheard. Without a
// telemetry socket there is nobody to report to, and nothing is said again.
func TestThePluginStaysOutWhereItCannotWork(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "s.obs")
	for _, tc := range []struct {
		name string
		args []string
		env  []string
	}{
		{"--bare", []string{"--bare"}, nil},
		{"function hooks off", nil, []string{functionHooksEnv + "=0"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args, env, notes := applyPlugin(tc.args, tc.env, socket)
			if harness.HasFlag(args, pluginDirFlag) || !slices.Equal(env, tc.env) || len(notes) != 1 || !strings.HasPrefix(notes[0], "not hearing interrupted turns: ") {
				t.Fatalf("args %v, env %v, notes %v", args, env, notes)
			}
			if _, err := os.Stat(telemetry.PluginPath(socket)); !os.IsNotExist(err) {
				t.Fatalf("a plugin was written anyway: %v", err)
			}
		})
	}
	args, env, notes := applyPlugin(nil, []string{functionHooksEnv + "=true"}, socket)
	if !harness.HasFlag(args, pluginDirFlag) || len(notes) != 0 || !slices.Equal(env, []string{functionHooksEnv + "=1"}) {
		t.Fatalf("switched on by the person: args %v, env %v, notes %v", args, env, notes)
	}
	if args, env, notes := applyPlugin(nil, nil, ""); len(args) != 0 || len(env) != 0 || len(notes) != 0 {
		t.Fatalf("without a socket: %v %v %v", args, env, notes)
	}
}

// hostRun is $.process.run as Claude Code 2.1.280 checks it before running
// anything: two arguments, argv — a non-empty list of strings, the first
// non-empty — and an optional init of cwd, env, stdin and timeoutMs, each of
// its own type. Anything else is refused with a rejected promise, as the harness
// refuses it, and the refusal is kept, so a module calling it another way
// fails here rather than in a live session (the host check was read in the
// binary; docs/research-claude-control.md). Shared by the unit host below and,
// in its own copy, by the workflow fixture's host.
const hostRun = `
const refused = []
const strings = (v) => typeof v === "object" && v !== null && !Array.isArray(v) && Object.values(v).every((s) => typeof s === "string")
function hostCheck(argv, init) {
  if (!(Array.isArray(argv) && argv.length > 0 && argv.every((s) => typeof s === "string") && argv[0] !== "")) return "takes argv, a non-empty list of strings naming the command first"
  if (init === undefined) return undefined
  if (typeof init !== "object" || init === null || Array.isArray(init)) return "takes init, { cwd?, env?, stdin?, timeoutMs? }"
  if (init.cwd !== undefined && (typeof init.cwd !== "string" || init.cwd === "")) return "init.cwd is a non-empty path"
  if (init.env !== undefined && !strings(init.env)) return "init.env is an object of strings"
  if (init.stdin !== undefined && typeof init.stdin !== "string") return "init.stdin is a string"
  if (init.timeoutMs !== undefined && !(Number.isInteger(init.timeoutMs) && init.timeoutMs > 0 && init.timeoutMs <= 600000)) return "init.timeoutMs is a whole number of ms, 1 to 600000"
  return undefined
}
function checkedRun(run) {
  return (...args) => {
    const refusal = args.length > 2 ? "takes argv and init" : hostCheck(args[0], args[1])
    if (refusal !== undefined) {
      refused.push(refusal)
      return Promise.reject(new Error("process.run: " + refusal + " (host check)"))
    }
    return run(args[0], args[1])
  }
}
`

// pluginHost loads the module the way the harness does — register(on), then
// each handler with ($, event, next), the module loaded once for the whole
// session — with a $ that records what the module asks to run instead of
// running it. Its environment and settings are the ones the test names, the
// first read at once and the second through a promise, so the module is held
// to awaiting either; a test that names neither gets a $ without them, as the
// workflow fixture's host once was, and the module must go on regardless. A
// part the test names as failing throws when it is read. The
// events it hands over throw when the prompt, the answer or the last message
// is read, so a module that touched one fails here.
const pluginHost = hostRun + `
import { pathToFileURL } from "node:url"
const mod = await import(pathToFileURL(process.argv[2]).href)
const handlers = {}
mod.register((name, handler) => { handlers[name] = handler })
const calls = []
const $ = { process: { run: checkedRun((argv, init) => { calls.push({ argv, init }); return Promise.resolve({ exitCode: 0, stdout: "", stderr: "" }) }) } }
const world = JSON.parse(process.argv[4])
const fails = (part) => (world.fail ?? []).includes(part)
if (world.env !== undefined) $.env = { get: (name) => { if (fails("env")) throw new Error("env: host refused"); return world.env[name] } }
if (world.settings !== undefined) $.settings = { read: async () => { if (fails("settings")) throw new Error("settings: host refused"); return world.settings } }
const guarded = (fields) => new Proxy(fields, { get(target, key) {
  if (["text", "answer", "prompt", "last_assistant_message", "error_details"].includes(key)) throw new Error("the module read " + String(key))
  return target[key]
} })
const events = JSON.parse(process.argv[3])
for (const [name, event] of events) {
  if (typeof handlers[name] !== "function") throw new Error("no handler for " + name)
  if (await handlers[name]($, guarded(event), () => "passed on") !== "passed on") throw new Error(name + " did not pass the event on")
}
process.stdout.write(JSON.stringify({ calls, refused }))
`

// hostCall is one process the module asked the host to run.
type hostCall struct {
	Argv []string `json:"argv"`
	Init struct {
		Stdin     string `json:"stdin"`
		TimeoutMs int    `json:"timeoutMs"`
	} `json:"init"`
}

// runModule plays events to a module under node, in one process as the
// harness keeps it, and returns what it asked to run and what the host
// refused. Without node the test is skipped: the module's shape is still
// checked above.
func runModule(t *testing.T, module []byte, events [][2]any) ([]hostCall, []string) {
	t.Helper()
	return runModuleIn(t, module, hostWorld{}, events)
}

// hostWorld is the environment and settings the host's $ answers with; nil
// leaves that part of $ out, and a part named in Fail throws when read.
type hostWorld struct {
	Env      map[string]string `json:"env,omitempty"`
	Settings map[string]any    `json:"settings,omitempty"`
	Fail     []string          `json:"fail,omitempty"`
}

func runModuleIn(t *testing.T, module []byte, world hostWorld, events [][2]any) ([]hostCall, []string) {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed; the module is not run")
	}
	dir := t.TempDir()
	// Node reads a .mjs as a module without a package.json; the harness
	// loads the .js as one.
	if err := os.WriteFile(filepath.Join(dir, "rewake.mjs"), module, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "host.mjs"), []byte(pluginHost), 0o600); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	worldJSON, err := json.Marshal(world)
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(node, filepath.Join(dir, "host.mjs"), filepath.Join(dir, "rewake.mjs"), string(encoded), string(worldJSON)).CombinedOutput()
	if err != nil {
		t.Fatalf("the module failed: %v\n%s", err, out)
	}
	var result struct {
		Calls   []hostCall `json:"calls"`
		Refused []string   `json:"refused"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	return result.Calls, result.Refused
}

// The module runs as the harness runs it: each event of the session itself
// becomes one run of `rewake observe` with the fields the collector decodes,
// in the call form the harness accepts. A subagent's turn becomes none, a
// measure of anything but the context none, and an Esc landing after the
// turn's Stop or StopFailure hook none — that hook has reported the turn; the
// next turn's start clears it. Every event is passed on, and neither the
// prompt nor the answer is read.
func TestThePluginModuleReportsOnlyWhatRewakeReads(t *testing.T) {
	dir := t.TempDir()
	argv := []string{"/bin/rewake", "observe", "/run/s.obs"}
	if err := writePlugin(filepath.Join(dir, "plugin"), argv); err != nil {
		t.Fatal(err)
	}
	module, err := os.ReadFile(filepath.Join(dir, "plugin", "hooks", "rewake.js"))
	if err != nil {
		t.Fatal(err)
	}
	stop := map[string]any{"hook_event_name": "Stop", "last_assistant_message": "done", "session_id": "c1"}
	failure := map[string]any{"hook_event_name": "StopFailure", "last_assistant_message": "broke", "error_details": "broke", "error": "api_error"}
	calls, refused := runModule(t, module, [][2]any{
		{"session.start", map[string]any{"cwd": "/w", "surface": "terminal", "isInteractive": true}},
		{"turn.start", map[string]any{"turnId": "t1", "text": "prompt"}},
		{"turn.complete", map[string]any{"turnId": "t1", "reason": "aborted", "isAborted": true, "answer": "partial", "durationMs": 5}},
		{"turn.complete", map[string]any{"turnId": "s1", "reason": "answer", "agentId": "a1", "answer": "child"}},
		{"session.measure", map[string]any{"context": map[string]any{"tokens": 5000, "window": 200000, "percent": 2.5}, "rateLimits": []any{}, "changed": []any{"context"}}},
		{"session.measure", map[string]any{"context": map[string]any{"tokens": 5000, "window": 200000, "percent": 2.5}, "rateLimits": []any{}, "changed": []any{"rateLimits"}}},
		{"turn.start", map[string]any{"turnId": "t2", "text": "prompt"}},
		{"classic.Stop", stop},
		{"turn.complete", map[string]any{"turnId": "t2", "reason": "aborted", "isAborted": true, "answer": "late"}},
		{"turn.start", map[string]any{"turnId": "t3", "text": "prompt"}},
		{"classic.StopFailure", failure},
		{"turn.complete", map[string]any{"turnId": "t3", "reason": "error", "answer": ""}},
		{"turn.start", map[string]any{"turnId": "t4", "text": "prompt"}},
		{"turn.complete", map[string]any{"turnId": "t4", "reason": "aborted", "isAborted": true, "answer": ""}},
	})
	if len(refused) != 0 {
		t.Fatalf("the host refused the module's calls: %v", refused)
	}
	want := []map[string]any{
		{"plugin_event": "plugin.ready"},
		{"plugin_event": "turn.start", "turn_id": "t1"},
		{"plugin_event": "turn.complete", "turn_id": "t1", "reason": "aborted"},
		{"plugin_event": "session.measure", "context": map[string]any{"tokens": 5000.0, "window": 200000.0, "percent": 2.5}},
		{"plugin_event": "turn.start", "turn_id": "t2"},
		{"plugin_event": "turn.start", "turn_id": "t3"},
		{"plugin_event": "turn.complete", "turn_id": "t3", "reason": "error"},
		{"plugin_event": "turn.start", "turn_id": "t4"},
		{"plugin_event": "turn.complete", "turn_id": "t4", "reason": "aborted"},
	}
	var got []map[string]any
	for index, call := range calls {
		var stdin map[string]any
		if err := json.Unmarshal([]byte(call.Init.Stdin), &stdin); err != nil {
			t.Fatalf("run %d stdin %q: %v", index, call.Init.Stdin, err)
		}
		if !slices.Equal(call.Argv, argv) || call.Init.TimeoutMs <= 0 {
			t.Errorf("run %d = %v (timeout %d), want %v", index, call.Argv, call.Init.TimeoutMs, argv)
		}
		if _, ok := telemetry.DecodePlugin([]byte(call.Init.Stdin)); !ok {
			t.Errorf("run %d is not an event the collector decodes: %s", index, call.Init.Stdin)
		}
		got = append(got, stdin)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("the module reported\n%v\nwant\n%v", got, want)
	}
}

// The host is as strict as the harness: the options object as the only
// argument — the form that silenced the first version of the module on a live
// session — is refused, and so is an argv that is not a list of strings.
func TestTheModuleHostRefusesWhatTheHarnessRefuses(t *testing.T) {
	module := []byte(`
export function register(on) {
  on("session.start", async ($, e, next) => {
    for (const call of [
      () => $.process.run({ argv: ["/bin/rewake"], init: { stdin: "{}" } }),
      () => $.process.run("/bin/rewake", { stdin: "{}" }),
      () => $.process.run(["/bin/rewake"], { stdin: {} }),
      () => $.process.run(["/bin/rewake"], { timeoutMs: 0 }),
      () => $.process.run(["/bin/rewake"], { stdin: "{}", timeoutMs: 5000 }),
    ]) { try { await call() } catch {} }
    return next(e)
  })
}
`)
	calls, refused := runModule(t, module, [][2]any{{"session.start", map[string]any{}}})
	if len(calls) != 1 || len(refused) != 4 || !strings.Contains(refused[0], "takes argv") {
		t.Fatalf("ran %v, refused %v", calls, refused)
	}
}
