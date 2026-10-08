package workflow

// The function-hooks plugin rewake passes this column's session, and the
// events the session plays to it.
//
// The real harness loads a plugin module and calls its handlers with each
// event; the module decides what to run. So the fixture does the same with the
// module rewake wrote: it loads it under node with a host that calls register,
// hands each handler the event the harness would, and runs what the module asks
// $.process.run to run — refusing, as the harness does, any call of another
// form. What reaches the wrapper therefore travels the
// product's own path — the module, `rewake observe`, the collector — rather
// than a stand-in for it.
//
// Stricter than the harness, on purpose: a plugin directory that is not laid
// out as one, or passed without function hooks switched on, refuses the
// launch — the harness would load nothing, and the case would pass on a
// session that never heard its plugin. And an event's prompt or answer throws
// when the module reads it, which the harness does not do.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// shimInterruptFirst makes the session's first turn one a person
	// interrupts: it reads its mail and ends with turn.complete reason
	// "aborted" and no Stop hook, as the harness does after an Esc. The
	// fixture harness's program takes it too and ends that turn interrupted.
	shimInterruptFirst = "RW_SHIM_INTERRUPT_FIRST_TURN"
	// shimInterruptAtStop makes the first turn one a person interrupts just
	// as it ends: its Stop hook runs, and the turn still ends with
	// turn.complete reason "aborted" — the order seen live on 2.1.280.
	shimInterruptAtStop = "RW_SHIM_INTERRUPT_AT_STOP"
	// shimNoFunctionHooks plays a harness that does not load the plugin —
	// another version, an untrusted workspace — while rewake still passes it.
	shimNoFunctionHooks = "RW_SHIM_NO_FUNCTION_HOOKS"
	// shimNode is the node that runs the plugin's modules. The case's PATH
	// is its own and holds no node, so the case that wants the plugin run
	// names one; without it the plugin is passed and not loaded.
	shimNode = "RW_SHIM_NODE"
	// functionHooksEnv is the harness's switch for function hooks.
	functionHooksEnv = "CLAUDE_CODE_ENABLE_FUNCTION_HOOKS"
)

// claudePlugin is a loaded plugin: its modules, the runtime that runs them,
// and one host per module, started with the first event and kept for the
// session, as the harness keeps a module loaded — the module's own state
// lives from one event to the next.
type claudePlugin struct {
	node    string
	name    string
	modules []string
	// serve answers what a module asks of the session itself — a compaction,
	// an abort (claudeshim_control_test.go); nil answers nothing.
	serve func(call string, args json.RawMessage) (any, string)

	mu    sync.Mutex
	hosts map[string]*pluginHost
}

// pluginHost is one running module: events go in on its stdin, a line per
// event comes back once the event's handlers and the processes they started
// are done.
type pluginHost struct {
	in io.WriteCloser
	// writing keeps an event and an answer to a module's call from
	// interleaving on stdin.
	writing sync.Mutex
	replies chan string
	// unloaded is set when the module failed; the harness unloads such a
	// plugin and the session goes on without it.
	unloaded bool
}

// loadPlugin reads the plugin directory rewake passed. No directory is a
// launch without a plugin, which is allowed: an older rewake passed none.
func loadPlugin(dir string) (*claudePlugin, error) {
	if dir == "" || os.Getenv(shimNoFunctionHooks) != "" {
		return nil, nil
	}
	if os.Getenv(functionHooksEnv) != "1" {
		return nil, fmt.Errorf("--plugin-dir without %s=1: the harness would load no module", functionHooksEnv)
	}
	var manifest struct {
		Name string `json:"name"`
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".claude-plugin", "plugin.json"))
	if err != nil || json.Unmarshal(raw, &manifest) != nil || manifest.Name == "" {
		return nil, fmt.Errorf("the plugin in %s has no readable manifest with a name: %v", dir, err)
	}
	var hooks struct {
		Modules []string `json:"modules"`
	}
	raw, err = os.ReadFile(filepath.Join(dir, "hooks", "hooks.json"))
	if err != nil || json.Unmarshal(raw, &hooks) != nil || len(hooks.Modules) == 0 {
		return nil, fmt.Errorf("the plugin in %s names no hooks modules: %v", dir, err)
	}
	plugin := &claudePlugin{name: manifest.Name}
	for _, module := range hooks.Modules {
		path := filepath.Join(dir, "hooks", module)
		if !strings.HasPrefix(path, filepath.Join(dir, "hooks")+string(filepath.Separator)) {
			return nil, fmt.Errorf("the module %s lies outside the plugin", module)
		}
		if _, err := os.Stat(path); err != nil {
			return nil, fmt.Errorf("the module %s: %w", module, err)
		}
		plugin.modules = append(plugin.modules, path)
	}
	node := os.Getenv(shimNode)
	if node == "" {
		// Nothing runs the module, so the session goes on as one whose
		// plugin did not load, which is what every case but the plugin's
		// own asks for.
		pluginRecord("no %s; the plugin is not run", shimNode)
		return nil, nil
	}
	plugin.node = node
	pluginRecord("loaded %v", plugin.modules)
	return plugin, nil
}

// pluginHostScript loads one module and plays it the events that come in on
// stdin, one JSON line each. The handlers of an event are chained the way the
// harness chains them, each passing the event on with next, and the reply
// waits for every process the module started. Beside the replies, a module's
// call on the session itself goes out as a line naming it, and its answer
// comes back on stdin (claudeshim_control_test.go).
//
// Its $.process.run is as strict as the harness's (read in the 2.1.280
// binary, docs/research-claude-control.md): two arguments, argv — a non-empty
// list of strings, the first non-empty — and an optional init of cwd, env,
// stdin and timeoutMs; any other call is refused with a rejected promise and
// named in the reply. A looser host once let a module pass here whose every
// call the harness refused, so the fixture must never accept what the harness
// does not. The same check is in internal/harness/claude/plugin_test.go.
//
// $.env and $.settings answer as the harness does for the person's settings
// file: $.settings.read() is that file, and $.env.get a variable from its env
// block before the process's own — the harness applies that block to its
// environment, and it wins over a variable the process already had
// (docs/research.md).
var pluginHostScript = pluginHostHead + pluginControlScript + pluginHostLoop

const pluginHostHead = `
import { spawn } from "node:child_process"
import { readFileSync, writeFileSync, mkdirSync, existsSync } from "node:fs"
import { join, dirname } from "node:path"
import { createInterface } from "node:readline"
const [pluginName, modulePath] = process.argv.slice(-2)
const say = (message) => process.stdout.write(JSON.stringify(message) + "\n")
const mod = await import("data:text/javascript," + encodeURIComponent(readFileSync(modulePath, "utf8")))
const handlers = {}
mod.register((event, handler) => { (handlers[event] ??= []).push(handler) })
let refused = []
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
let running = []
const readSettings = () => {
  try { return JSON.parse(readFileSync(join(process.env.CLAUDE_CONFIG_DIR || join(process.env.HOME, ".claude"), "settings.json"), "utf8")) } catch { return {} }
}
const $ = { env: { get: async (name) => {
  const block = readSettings().env
  if (block !== null && typeof block === "object" && Object.hasOwn(block, name)) return String(block[name])
  return process.env[name]
} }, settings: { read: async () => readSettings() }, process: { run: (...args) => {
  const refusal = args.length > 2 ? "takes argv and init" : hostCheck(args[0], args[1])
  if (refusal !== undefined) {
    refused.push(refusal)
    return Promise.reject(new Error("process.run: " + refusal + " (host check)"))
  }
  const [argv, init] = args
  const done = new Promise((resolve, reject) => {
    const child = spawn(argv[0], argv.slice(1), { stdio: [init && init.stdin !== undefined ? "pipe" : "ignore", "pipe", "pipe"], cwd: init && init.cwd, env: init && init.env ? { ...process.env, ...init.env } : process.env })
    let stdout = "", stderr = ""
    child.stdout.on("data", (d) => { stdout += d })
    child.stderr.on("data", (d) => { stderr += d })
    child.on("error", reject)
    child.on("close", (code) => resolve({ exitCode: code ?? 1, stdout, stderr }))
    if (init && init.stdin !== undefined) child.stdin.end(init.stdin)
  })
  running.push(done.catch(() => {}))
  return done
} } }
const guarded = (fields) => new Proxy(fields, { get(target, key) {
  if (["text", "answer", "prompt", "last_assistant_message", "error_details"].includes(key)) throw new Error("the module read the " + String(key))
  return target[key]
} })
`

// pluginHostLoop plays the events one after another, and resolves a
// module's call the moment its answer arrives, even while an event plays: an
// abort ends its turn with an event before it resolves. The timers the
// module starts would keep node running, so the end of stdin ends it.
const pluginHostLoop = `
async function play(name, fields) {
  let error = ""
  try {
    let chain = async (e) => e
    for (const handler of [...(handlers[name] ?? [])].reverse()) { const below = chain; chain = (e) => handler($, e, below) }
    await chain(guarded(fields))
  } catch (err) { error = String(err && err.stack || err) }
  await Promise.all(running)
  say({ error, refused })
  running = []
  refused = []
}
let queue = Promise.resolve()
const lines = createInterface({ input: process.stdin })
lines.on("line", (line) => {
  const message = JSON.parse(line)
  if (message.answer !== undefined) {
    answered(message)
    return
  }
  queue = queue.then(() => play(message.name, message.fields))
})
lines.on("close", () => process.exit(0))
`

// event plays one harness event to every module and returns once the
// processes they started have ended. A module that fails is recorded and
// unloaded, as a failing module costs a real session its plugin; a call the
// host refused is recorded too, and costs that report.
func (p *claudePlugin) event(name string, fields map[string]any) {
	if p == nil {
		return
	}
	encoded, err := json.Marshal(map[string]any{"name": name, "fields": fields})
	if err != nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, module := range p.modules {
		host := p.host(module)
		if host == nil || host.unloaded {
			continue
		}
		host.writing.Lock()
		_, err := host.in.Write(append(encoded, '\n'))
		host.writing.Unlock()
		if err != nil {
			pluginRecord("%s: the host is gone: %v", name, err)
			host.unloaded = true
			continue
		}
		select {
		case line, open := <-host.replies:
			var reply struct {
				Error   string   `json:"error"`
				Refused []string `json:"refused"`
			}
			switch {
			case !open || json.Unmarshal([]byte(line), &reply) != nil:
				pluginRecord("%s: the host ended: %q", name, line)
				host.unloaded = true
			case reply.Error != "":
				pluginRecord("%s failed, the module is unloaded: %s", name, reply.Error)
				host.unloaded = true
			case len(reply.Refused) > 0:
				pluginRecord("%s: the host refused %q", name, reply.Refused)
			default:
				pluginRecord("%s", name)
			}
		case <-time.After(10 * time.Second):
			pluginRecord("%s did not end", name)
			host.unloaded = true
		}
	}
}

// host starts a module's host on its first event. Its stdin closes when this
// session exits, which ends it.
func (p *claudePlugin) host(module string) *pluginHost {
	if host, ok := p.hosts[module]; ok {
		return host
	}
	if p.hosts == nil {
		p.hosts = map[string]*pluginHost{}
	}
	cmd := exec.Command(p.node, "--input-type=module", "-e", pluginHostScript, "--", p.name, module)
	cmd.Env = os.Environ()
	cmd.Stderr = os.Stderr
	in, err := cmd.StdinPipe()
	var out io.ReadCloser
	if err == nil {
		out, err = cmd.StdoutPipe()
	}
	if err == nil {
		err = cmd.Start()
	}
	if err != nil {
		pluginRecord("the host of %s did not start: %v", module, err)
		p.hosts[module] = nil
		return nil
	}
	host := &pluginHost{in: in, replies: make(chan string)}
	go func() {
		scanner := bufio.NewScanner(out)
		scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
		for scanner.Scan() {
			line := scanner.Text()
			var tagged struct {
				Call string          `json:"call"`
				ID   int             `json:"id"`
				Args json.RawMessage `json:"args"`
				Log  *string         `json:"log"`
			}
			_ = json.Unmarshal([]byte(line), &tagged)
			switch {
			case tagged.Call != "":
				go p.answer(host, tagged.ID, tagged.Call, tagged.Args)
			case tagged.Log != nil:
				pluginRecord("%s", *tagged.Log)
			default:
				host.replies <- line
			}
		}
		close(host.replies)
		_ = cmd.Wait()
	}()
	p.hosts[module] = host
	return host
}

// turnStarted plays the start of a turn of the session itself.
func (p *claudePlugin) turnStarted(turn, text string) {
	p.event("turn.start", map[string]any{"text": text, "turnId": turn})
}

// turnCompleted plays its end, with the reason the harness gives.
func (p *claudePlugin) turnCompleted(turn, answer, reason string) {
	p.event("turn.complete", map[string]any{
		"answer": answer, "durationMs": 5, "isAborted": reason == "aborted", "turnId": turn, "reason": reason,
	})
}

// firstTurn is true once, for the first turn, when the variable asks for it.
func (s *claudeSession) firstTurn(variable string) bool {
	if os.Getenv(variable) == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.interrupted {
		return false
	}
	s.interrupted = true
	return true
}

// pluginRecord appends a line to the case's evidence about the plugin: every
// event played and how its handlers ended. A session's own output is not kept,
// and a module that failed would otherwise leave only a missing report.
func pluginRecord(format string, args ...any) {
	mark := os.Getenv(shimReadyFile)
	if mark == "" {
		return
	}
	file, err := os.OpenFile(mark+".plugin", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }()
	_, _ = fmt.Fprintf(file, format+"\n", args...)
}
