// rewake's function-hooks plugin for one Claude Code session, written by the
// rewake wrapper that launched it. It tells that wrapper when a turn of the
// session starts and ends — including a turn a person interrupted, which runs
// no Stop hook — how full the context is, and the auto-compact window it is
// measured against, by running `rewake observe`.
// And it carries out what a main session asks through `rewake compact` and
// `rewake interrupt`: it polls the run's control directory, compacts the
// conversation or aborts the running turn, and writes back the outcome — only
// numbers and the host's own error text (docs/remote-control.md).
// It watches the end of every compaction it did not ask for, reading nothing
// of it, to keep a main's compaction clear of the others.
// Nothing else: it reads no prompt and no answer, sends of the settings only
// that one key, and never sends the compaction's summary.
const argv = __REWAKE_ARGV__
const control = __REWAKE_CONTROL__

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

// told hands one event to rewake and waits until it is sent: $.process.run
// resolves once the process has ended, and `rewake observe` has sent its one
// datagram by then. Only for what must reach rewake ahead of the hooks of a
// compaction the module asked for — never in a turn, which must not wait on it.
async function told($, event) {
  try {
    await $.process.run(argv, { stdin: JSON.stringify(event), timeoutMs: 5000 })
  } catch {}
}

// own is true for an event of the session itself: a subagent's turns carry
// its id, and say nothing about whether the session is working.
const own = (e) => e !== null && typeof e === "object" && e.agentId === undefined
const word = (value) => (typeof value === "string" ? value : "")
const number = (value) => (typeof value === "number" && Number.isFinite(value) ? value : undefined)

// limitOf keeps what rewake reads of the auto-compact window, which the
// harness treats as the context window while neither the status line nor
// session.measure reports it: the variable as $.env holds it — the harness's
// environment, settings env included — and the settings key. The handlers
// read both with the variable's name spelled out and $ only ever in the
// $.noun.verb(...) form, the one the harness's load check accepts
// (docs/research-claude-control.md). A read that throws sends no limit at all:
// a failed read is not a source that went away, and the last limit rewake
// heard stands.
const limitOf = (env, settings) => ({
  env: typeof env === "string" ? env : undefined,
  settings: settings !== null && typeof settings === "object" ? number(settings.autoCompactWindow) : undefined,
})

// hooked is set once a Stop or StopFailure hook ran in the current turn. That
// hook reports the turn; an Esc landing after it still ends the turn as
// aborted, and reporting that too would give the turn two outcomes.
let hooked = false

// turn is the id of the session's own running turn, the one an interrupt
// aborts; aborting is that turn once a request asked for it, with who asked, so
// its end is reported as theirs rather than a person's.
let turn
let aborting
// handled keeps the ids of requests taken by this load of the module; the
// .taken file keeps them across a reload, which runs session.start again.
const handled = new Set()
let polling
let busy = false
// quiet settles a second after the end of the last compaction that was not
// ours: that one's PostCompact reaches rewake a moment after it ends, and
// arriving after the word for ours it would be counted as ours
// (docs/remote-control.md).
let quiet = Promise.resolve()
function settle($) {
  try {
    quiet = Promise.resolve($.clock.sleep(1000)).catch(() => {})
  } catch {}
}

const request = (text) => {
  try {
    const parsed = JSON.parse(text)
    return parsed !== null && typeof parsed === "object" && /^[0-9a-f]{32}$/.test(word(parsed.id)) ? parsed : undefined
  } catch {
    return undefined
  }
}
const message = (error) => String(error !== null && typeof error === "object" && typeof error.message === "string" ? error.message : error)

// refusal maps the host's refusal to one of rewake's reasons by the words the
// host uses (docs/research-claude-actions.md), and keeps its text as it was.
function refusal(error, reasons) {
  const detail = message(error)
  for (const [words, reason] of reasons) {
    if (detail.includes(words)) return { outcome: "refused", reason, detail }
  }
  return { outcome: "failed", detail }
}

// act carries out one request. The host refuses a compaction mid-turn by
// itself, atomically, so the module does not check; an abort needs the turn's
// id, which only the module knows.
// A compaction waits out the second after another one first. Then rewake is
// told who asked, and the module waits until that report is sent before it
// asks the host, so it reaches rewake ahead of the compaction's hooks; a
// refusal is told as well, before the answer, so rewake lays the word aside
// ahead of any later hook. Not while a turn runs: the host refuses that
// compaction, and the turn may compact on its own.
async function act($, asked) {
  if (asked.action === "compact") {
    for (let waited; waited !== quiet; ) {
      waited = quiet
      await waited
    }
    const announced = turn === undefined
    if (announced) await told($, { plugin_event: "compact.asked", request: asked.id, by: word(asked.from) })
    try {
      const focus = word(asked.focus)
      const result = focus === "" ? await $.session.compact() : await $.session.compact({ instructions: focus })
      const counts = result !== null && typeof result === "object" ? result : {}
      return { outcome: "done", tokensBefore: number(counts.tokensBefore), tokensAfter: number(counts.tokensAfter) }
    } catch (error) {
      const answer = refusal(error, [
        ["a turn is running", "in a turn"],
        ["a turn is in flight", "in a turn"],
        ["an external turn is driving the conversation", "in a turn"],
        ["switched off", "compaction switched off"],
        ["lives on the remote session", "remote conversation"],
        ["Not enough messages to compact", "nothing to compact"],
      ])
      // A conversation too short is refused after PreCompact. Every other
      // refusal the host has comes before it starts, and the table holds them
      // all (read in the 2.1.280 binary), so what it does not hold failed
      // after the start: the summary's request, or applying it.
      const started = answer.reason === "nothing to compact" || answer.outcome === "failed"
      if (announced) await told($, { plugin_event: "compact.refused", request: asked.id, started })
      return answer
    }
  }
  if (asked.action === "interrupt") {
    if (turn === undefined) return { outcome: "refused", reason: "no turn running" }
    const running = turn
    aborting = { turn: running, by: word(asked.from) }
    try {
      await $.turn.abort({ turnId: running })
      return { outcome: "done" }
    } catch (error) {
      if (aborting !== undefined && aborting.turn === running) aborting = undefined
      return refusal(error, [["no turn is running", "no turn running"], ["is not the running turn", "no turn running"]])
    }
  }
  return { outcome: "failed", detail: "unknown action " + word(asked.action) }
}

// pending reads the request in place, if any. It asks whether the file exists
// before reading it: reading a missing file is logged as an error by the host,
// four times a second. A file removed between the two reads as none.
async function pending($) {
  if (!(await $.fs.exists(control + "/request.json"))) return undefined
  try {
    return request(await $.fs.read(control + "/request.json"))
  } catch {
    return undefined
  }
}

// poll takes the request waiting in the control directory, if any, once. It
// marks the request taken before acting and then checks it is still in place:
// an asker that gave up removes it before its last look for the mark, so a
// request still there is one the asker will wait for, and one gone is answered
// as withdrawn without acting (docs/remote-control.md).
async function poll($) {
  if (busy) return
  busy = true
  try {
    const asked = await pending($)
    if (asked === undefined || handled.has(asked.id)) return
    handled.add(asked.id)
    const taken = control + "/" + asked.id + ".taken"
    if (await $.fs.exists(taken)) return
    await $.fs.write(taken, "")
    const still = await pending($)
    const answer = still !== undefined && still.id === asked.id ? await act($, asked) : { outcome: "refused", reason: "withdrawn before it was taken" }
    await $.fs.write(control + "/" + asked.id + ".result", JSON.stringify({ id: asked.id, ...answer }))
  } catch {
  } finally {
    busy = false
  }
}

export function register(on) {
  on("session.start", async ($, e, next) => {
    let env, settings, read = true
    try { env = await $.env.get("CLAUDE_CODE_AUTO_COMPACT_WINDOW") } catch { read = false }
    try { settings = await $.settings.read() } catch { read = false }
    report($, { plugin_event: "plugin.ready", limit: read ? limitOf(env, settings) : undefined })
    // Once per load of the module: a reload loads it afresh and starts again.
    if (polling === undefined) {
      if (control !== "") polling = $.clock.every(250, () => poll($))
    }
    return next(e)
  })
  on("turn.start", async ($, e, next) => {
    if (own(e)) {
      hooked = false
      turn = word(e.turnId)
      report($, { plugin_event: "turn.start", turn_id: turn })
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
      const id = word(e.turnId)
      const by = reason === "aborted" && aborting !== undefined && aborting.turn === id ? aborting.by : undefined
      if (!(reason === "aborted" && hooked)) report($, { plugin_event: "turn.complete", turn_id: id, reason, by })
      if (turn === id) turn = undefined
      if (aborting !== undefined && aborting.turn === id) aborting = undefined
    }
    return next(e)
  })
  // Only another's compaction reaches this handler: the host skips a plugin's
  // own handlers for the compaction its own call raised, as re-entry. While it
  // runs the host refuses ours as in flight; its end starts the quiet second.
  on("session.compact", async ($, e, next) => {
    try {
      return await next(e)
    } finally {
      settle($)
    }
  })
  on("session.measure", async ($, e, next) => {
    // The event fires for rate limits and cost as well, and each report is a
    // process; only a change of the context is rewake's.
    const changed = e !== null && typeof e === "object" && Array.isArray(e.changed) ? e.changed : []
    const context = changed.includes("context") ? e.context : undefined
    if (context !== null && typeof context === "object") {
      // Read again at each measure, as the harness reads it again for each
      // request: the limit shown can never be older than one turn.
      let env, settings, read = true
      try { env = await $.env.get("CLAUDE_CODE_AUTO_COMPACT_WINDOW") } catch { read = false }
      try { settings = await $.settings.read() } catch { read = false }
      report($, {
        plugin_event: "session.measure",
        context: { tokens: number(context.tokens), window: number(context.window), percent: number(context.percent) },
        limit: read ? limitOf(env, settings) : undefined,
      })
    }
    return next(e)
  })
}
