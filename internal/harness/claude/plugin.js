// rewake's function-hooks plugin for one Claude Code session, written by the
// rewake wrapper that launched it. It tells that wrapper when a turn of the
// session starts and ends — including a turn a person interrupted, which runs
// no Stop hook — and how full the context is, by running `rewake observe`.
// Nothing else: it reads no prompt and no answer, and acts on nothing.
const argv = __REWAKE_ARGV__

// report hands one event to rewake and does not wait for it: a turn must never
// wait on its observer, and a report that fails costs one observation. The
// harness takes the command and its options as two arguments, and refuses any
// other form with a rejected promise (docs/research-claude-control.md).
function report($, event) {
  try {
    const running = $.process.run(argv, { stdin: JSON.stringify(event), timeoutMs: 5000 })
    if (running && typeof running.catch === "function") running.catch(() => {})
  } catch {}
}

// own is true for an event of the session itself: a subagent's turns carry
// its id, and say nothing about whether the session is working.
const own = (e) => e !== null && typeof e === "object" && e.agentId === undefined
const word = (value) => (typeof value === "string" ? value : "")
const number = (value) => (typeof value === "number" && Number.isFinite(value) ? value : undefined)

// hooked is set once a Stop or StopFailure hook ran in the current turn. That
// hook reports the turn; an Esc landing after it still ends the turn as
// aborted, and reporting that too would give the turn two outcomes.
let hooked = false

export function register(on) {
  on("session.start", async ($, e, next) => {
    report($, { plugin_event: "plugin.ready" })
    return next(e)
  })
  on("turn.start", async ($, e, next) => {
    if (own(e)) {
      hooked = false
      report($, { plugin_event: "turn.start", turn_id: word(e.turnId) })
    }
    return next(e)
  })
  on("classic.Stop", async ($, e, next) => {
    hooked = true
    return next(e)
  })
  on("classic.StopFailure", async ($, e, next) => {
    hooked = true
    return next(e)
  })
  on("turn.complete", async ($, e, next) => {
    if (own(e)) {
      const reason = word(e.reason)
      if (!(reason === "aborted" && hooked)) report($, { plugin_event: "turn.complete", turn_id: word(e.turnId), reason })
    }
    return next(e)
  })
  on("session.measure", async ($, e, next) => {
    // The event fires for rate limits and cost as well, and each report is a
    // process; only a change of the context is rewake's.
    const changed = e !== null && typeof e === "object" && Array.isArray(e.changed) ? e.changed : []
    const context = changed.includes("context") ? e.context : undefined
    if (context !== null && typeof context === "object") {
      report($, {
        plugin_event: "session.measure",
        context: { tokens: number(context.tokens), window: number(context.window), percent: number(context.percent) },
      })
    }
    return next(e)
  })
}
