package workflow

// What a module may ask of the session itself — compact its conversation,
// abort its turn — and the $ it needs to poll a control directory for such a
// request: $.clock.every, $.clock.sleep and $.fs.
//
// The harness answers these from inside the session, so the fixture answers
// them from inside its own: the node host sends each call out as a line, the
// session carries it out with the same effects the harness has, and the answer
// goes back in on the host's stdin. A compaction runs the hooks a compaction
// runs, so rewake's telemetry counts it by its own path; an abort ends the held
// turn with turn.complete "aborted" and no Stop hook, as the harness does.
//
// As strict as Claude Code 2.1.280 about every form and refusal the research
// recorded (docs/research-claude-actions.md), and stricter where it knows less:
// a stricter fake fails a module the harness would accept, which a case shows;
// a softer one passes a module the harness refuses, which nothing shows until
// a person tries it (docs/traps.md). Every refusal carries the plugin's name in
// front, as the harness puts it there.

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

// shimHoldFirstTurn makes the session's first turn one that is still running
// when the case looks: it reads its mail and then waits, until a module aborts
// it or holdLimit passes and it ends as an ordinary turn.
const shimHoldFirstTurn = "RW_SHIM_HOLD_FIRST_TURN"

// holdLimit is long enough for a case to compact and interrupt the held turn,
// and short enough to leave the session's own ceiling room to end.
const holdLimit = 12 * time.Second

// pluginControlScript is the host's part for these calls: the channel to the
// session and the $ they use. Spliced between the host's head and its loop.
const pluginControlScript = `
const refuse = (text) => Promise.reject(new Error(pluginName + ": " + text))
const waiting = new Map()
let calls = 0
function call(name, args) {
  const id = ++calls
  return new Promise((resolve, reject) => {
    waiting.set(id, { resolve, reject })
    say({ call: name, id, args })
  })
}
function answered({ answer, result, refusal }) {
  const pending = waiting.get(answer)
  if (pending === undefined) return
  waiting.delete(answer)
  if (refusal) pending.reject(new Error(pluginName + ": " + refusal))
  else pending.resolve(result)
}
const path = (p) => typeof p === "string" && p !== ""
const record = (o) => o !== null && typeof o === "object" && !Array.isArray(o)
$.clock = { every: (ms, fn) => {
  if (!(typeof ms === "number" && Number.isFinite(ms) && ms >= 1)) throw new Error(pluginName + ": $.clock.every takes a non-negative number of milliseconds, at least 1")
  if (typeof fn !== "function") throw new Error(pluginName + ": $.clock.every takes a function")
  const timer = setInterval(async () => {
    try { await fn() } catch (err) { say({ log: "[WARN] a $.clock.every callback threw: " + String(err && err.message || err) }) }
  }, ms)
  return { cancel: () => clearInterval(timer) }
}, sleep: async (ms) => {
  if (!(typeof ms === "number" && Number.isFinite(ms) && ms >= 0)) return refuse("$.clock.sleep takes a non-negative number of milliseconds")
  await new Promise((resolve) => setTimeout(resolve, ms))
} }
$.fs = {
  exists: async (...a) => {
    if (a.length !== 1 || !path(a[0])) return refuse("$.fs.exists takes a path")
    return existsSync(a[0])
  },
  read: async (...a) => {
    if (a.length < 1 || a.length > 2 || !path(a[0])) return refuse("$.fs.read takes a path")
    if (a.length === 2 && !(record(a[1]) && Object.keys(a[1]).every((k) => k === "as"))) return refuse("$.fs.read takes a path and { as }")
    try { return readFileSync(a[0], "utf8") } catch (err) {
      say({ log: "[ERROR] $.fs.read " + a[0] + ": " + err.code })
      return refuse("$.fs.read: " + a[0] + ": " + err.code)
    }
  },
  write: async (...a) => {
    if (a.length !== 2 || !path(a[0]) || typeof a[1] !== "string") return refuse("$.fs.write takes a path and a string")
    if (Buffer.byteLength(a[1]) > 4 * 1024 * 1024) return refuse("$.fs.write: more than 4 MiB")
    mkdirSync(dirname(a[0]), { recursive: true })
    writeFileSync(a[0], a[1])
  },
}
$.session = { compact: async (...a) => {
  const form = a.length === 0 || (a.length === 1 && record(a[0]) && Object.keys(a[0]).every((k) => k === "instructions") && typeof a[0].instructions === "string")
  if (!form) return refuse("$.session.compact takes { instructions } (a string) or nothing")
  return call("compact", { instructions: a.length === 0 ? "" : a[0].instructions })
} }
$.turn = { abort: async (...a) => {
  if (a.length !== 1 || !record(a[0]) || !path(a[0].turnId)) return refuse("$.turn.abort takes { turnId }")
  return call("abort", { turnId: a[0].turnId })
} }
`

// sessionTurns is what the session knows about its own turns that a call on it
// needs: which one runs, whether it is held open, how many have ended, and a
// compaction in progress, which a turn does not start during.
type sessionTurns struct {
	mu        sync.Mutex
	running   string
	held      chan chan struct{}
	completed int
	// compacting is held for a whole compaction; a turn takes it to start.
	compacting sync.Mutex
}

// startTurn marks a turn running, after any compaction in progress.
func (s *claudeSession) startTurn(turn string) {
	s.turns.compacting.Lock()
	defer s.turns.compacting.Unlock()
	s.turns.mu.Lock()
	defer s.turns.mu.Unlock()
	s.turns.running = turn
}

// completeTurn plays the end of a turn to the plugin once the session holds it
// ended: a compaction asked from turn.complete is one between turns.
func (s *claudeSession) completeTurn(turn, text, reason string) {
	s.turns.mu.Lock()
	if s.turns.running == turn {
		s.turns.running = ""
		s.turns.held = nil
		s.turns.completed++
	}
	s.turns.mu.Unlock()
	s.plugin.turnCompleted(turn, text, reason)
}

// holdTurn keeps a turn running until a module aborts it, or ends it as an
// ordinary turn after holdLimit. True when it was aborted.
func (s *claudeSession) holdTurn(turn, text string) bool {
	abort := make(chan chan struct{})
	s.turns.mu.Lock()
	s.turns.held = abort
	s.turns.mu.Unlock()
	select {
	case done := <-abort:
		// The harness ends an aborted turn with no Stop hook: only the
		// plugin hears it.
		s.completeTurn(turn, text, "aborted")
		(&shimSession{}).recordTurn(turn + " aborted " + firstLine(text))
		close(done)
		return true
	case <-time.After(holdLimit):
		s.turns.mu.Lock()
		s.turns.held = nil
		s.turns.mu.Unlock()
		return false
	}
}

// steer answers one call a module made on the session: a result, or the
// refusal the harness gives, without the plugin's name, which the host adds.
func (s *claudeSession) steer(call string, args json.RawMessage) (any, string) {
	switch call {
	case "compact":
		var asked struct {
			Instructions string `json:"instructions"`
		}
		if json.Unmarshal(args, &asked) != nil {
			return nil, "$.session.compact takes { instructions } (a string) or nothing"
		}
		return s.compact(asked.Instructions)
	case "abort":
		var asked struct {
			TurnID string `json:"turnId"`
		}
		if json.Unmarshal(args, &asked) != nil || asked.TurnID == "" {
			return nil, "$.turn.abort takes { turnId }"
		}
		return s.abort(asked.TurnID)
	}
	return nil, "the fixture serves no " + call
}

// compact refuses in the harness's order and words, then runs what a
// compaction runs: session.compact, which the calling plugin's own handlers do
// not see, and the hooks PreCompact, SessionStart with source "compact" and
// PostCompact. A conversation too short is refused after PreCompact, with
// nothing after it, as the harness does.
func (s *claudeSession) compact(instructions string) (any, string) {
	s.turns.compacting.Lock()
	defer s.turns.compacting.Unlock()
	s.turns.mu.Lock()
	running, completed := s.turns.running, s.turns.completed
	s.turns.mu.Unlock()
	switch {
	case running != "":
		return nil, "$.session.compact: a turn is running (" + running + "); the conversation compacts between turns, so call it from turn.complete or later"
	case os.Getenv("DISABLE_COMPACT") != "":
		return nil, "$.session.compact: compaction is switched off in this session (DISABLE_COMPACT), for /compact and plugins alike"
	}
	// Every call here comes from this plugin's modules.
	s.plugin.raised(s.plugin.origin(), "session.compact", map[string]any{"trigger": "plugin", "instructions": instructions})
	summary := "the conversation so far, summarized"
	for _, hook := range []struct {
		event  string
		fields map[string]any
	}{
		{"PreCompact", map[string]any{"trigger": "manual", "custom_instructions": instructions}},
		{"SessionStart", map[string]any{"source": "compact"}},
		{"PostCompact", map[string]any{"trigger": "manual", "compact_summary": summary}},
	} {
		payload := map[string]any{"hook_event_name": hook.event, "session_id": shimConversation(), "cwd": workingDirectory(), "transcript_path": ""}
		for key, value := range hook.fields {
			payload[key] = value
		}
		if encoded, err := json.Marshal(payload); err == nil {
			s.runHook(hook.event, s.launch.settings.observe, encoded)
		}
		if hook.event == "PreCompact" && completed == 0 {
			// Where the harness draws the line is not known; one finished
			// turn is the fixture's, and a case that compacts has worked one.
			return nil, "$.session.compact: Not enough messages to compact."
		}
	}
	return map[string]any{
		"messages":     []map[string]any{{"role": "user", "text": summary}},
		"tokensBefore": 120000, "tokensAfter": 9000, "usage": map[string]any{},
	}, ""
}

// abort ends the running turn, refusing an idle session or another turn's id
// as the harness does. Only a held turn can be aborted here — any other ends
// on its own before a module could ask — and the call answers once the turn
// has ended, as the harness's does.
func (s *claudeSession) abort(turn string) (any, string) {
	s.turns.mu.Lock()
	running, held := s.turns.running, s.turns.held
	s.turns.mu.Unlock()
	switch {
	case running == "":
		return nil, "$.turn.abort: no turn is running (asked for " + turn + ")"
	case turn != running:
		return nil, "$.turn.abort: " + turn + " is not the running turn (" + running + ")"
	case held == nil:
		return nil, "the fixture aborts only a held turn, and " + turn + " is not held"
	}
	done := make(chan struct{})
	select {
	case held <- done:
	case <-time.After(5 * time.Second):
		return nil, "the held turn " + turn + " did not take the abort"
	}
	<-done
	return nil, ""
}

// raised plays an event a plugin's own call raised. The harness runs none of
// that plugin's handlers for it — "hooks module rewake@inline session.compact
// skipped: re-entry (the plugin's own code raised it; origin rewake)", seen
// live on 2.1.280 — and a host that ran them passed a module whose handler a
// live session never calls.
func (p *claudePlugin) raised(origin, name string, fields map[string]any) {
	if p != nil && origin == p.name {
		pluginRecord("%s skipped: re-entry (the plugin's own code raised it; origin %s)", name, origin)
		return
	}
	p.event(name, fields)
}

// origin names the plugin whose call raised an event.
func (p *claudePlugin) origin() string {
	if p == nil {
		return ""
	}
	return p.name
}

// answer carries a module's call to the session and its answer back to the
// module's host.
func (p *claudePlugin) answer(host *pluginHost, id int, call string, args json.RawMessage) {
	var result any
	refusal := "the fixture serves no " + call
	if p.serve != nil {
		result, refusal = p.serve(call, args)
	}
	line, err := json.Marshal(map[string]any{"answer": id, "result": result, "refusal": refusal})
	if err != nil {
		return
	}
	host.writing.Lock()
	_, _ = host.in.Write(append(line, '\n'))
	host.writing.Unlock()
	if refusal != "" {
		pluginRecord("%s refused: %s", call, refusal)
		return
	}
	pluginRecord("%s", call)
}
