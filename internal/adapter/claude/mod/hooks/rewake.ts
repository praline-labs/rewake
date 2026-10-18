// The mod's one module. A load says hello to its run's wrapper at
// session.start and keeps the answer, then reports the turns of the session's
// own conversation: each start once, each end through the outbox until its
// receipt comes. The socket's path and the run's epoch come from the run's
// environment, so the same files serve every run
// (docs/v2/stage4-space.md#2-the-protocol, docs/v2/stage4-turns.md).
import type { Register } from 'claude-code'
import { paths } from '../protocol.gen.ts'
import type { EndReason, HelloAnswer, HelloRequest, TurnEndRequest, TurnStartRequest } from '../protocol.gen.ts'
import { Management } from './management.ts'
import { Heartbeat } from './heartbeat.ts'
import { Outbox, cut, listed, loadKey, newId, nextDelay } from './outbox.ts'
import type { Host } from './outbox.ts'
import { Workers, paneId } from './workers.ts'
import { drawPane } from './pane.ts'
import { Asks, allowOnce, allowSession, refuseCall } from './asks.ts'

// The harness's reasons, mapped to the protocol's.
const reasons: Record<string, EndReason> = {
  answer: 'answered',
  aborted: 'aborted',
  refusal: 'refused',
  error: 'failed',
}

// counted is a number the store keeps, 0 for none.
function counted(value: unknown): number {
  const held = Number(value)
  return Number.isFinite(held) ? held : 0
}

// takeLoadSeq takes the next number of the run's counter in the store: loads
// of one run follow one another, so the read and the write have no rival. A
// write that fails still leaves the number taken: the host binds a hello of
// another load with the current number as the later one. A counter that
// cannot be read gives 1, and a 410 then names the number to go above.
async function takeLoadSeq(host: Host, epoch: string): Promise<number> {
  if (epoch === '') {
    return 1
  }
  let seq: number
  try {
    seq = counted(await host.get(loadKey(epoch))) + 1
  } catch {
    return 1
  }
  try {
    await host.set(loadKey(epoch), seq)
    await listed(host, epoch)
  } catch {
    // The number stands; the counter did not advance.
  }
  return seq
}

// renumber is the number of the one second hello after a 410: the next above
// the store's counter, this load's number and the one the refusal names —
// unless the store shows a load numbered above this one, which holds the run,
// or cannot be read, which cannot tell.
async function renumber(host: Host, epoch: string, own: number, refusal: string): Promise<number | undefined> {
  let held = 0
  if (epoch !== '') {
    try {
      held = counted(await host.get(loadKey(epoch)))
    } catch {
      return undefined
    }
  }
  if (held > own) {
    return undefined
  }
  let holder = 0
  try {
    holder = counted((JSON.parse(refusal) as { loadSeq?: number }).loadSeq)
  } catch {
    // A refusal that names no number: the store's and this load's decide.
  }
  const seq = Math.max(held, own, holder) + 1
  if (epoch !== '') {
    try {
      await host.set(loadKey(epoch), seq)
    } catch {
      // As takeLoadSeq: the number stands.
    }
  }
  return seq
}

// Load is what a load learned at session.start.
interface Load {
  host: Host
  epoch: string
  outbox: Outbox
  heartbeat: Heartbeat
  management: Management
  workers: Workers
  asks: Asks
  // command registers /workers, for a run that hosts spaces.
  command: () => Promise<void>
  // run is the wrapper's answer to this load's hello: undefined until it
  // comes, and when no wrapper answered, which later steps read as "not live".
  run?: HelloAnswer
}

// hello is sent until it is answered, under the same load id and number: a
// fetch that threw, a 500 or a 503 sends it again after 1 s up to 10 s. An
// admitted hello starts the heartbeat for the generation it answered. A 410
// says a load numbered higher holds the run: a load that sees no such number
// in the store says hello once more under the next number, and a second 410
// ends it. Any other refusal is final for the load: the wrapper refused it
// and knows why, and the mod serves nothing it did not admit.
async function hello(load: Load, request: HelloRequest, delay: number, renumbered = false) {
  try {
    const answer = await load.host.post(paths.hello, request)
    if (answer.ok) {
      load.run = JSON.parse(answer.text) as HelloAnswer
      load.asks.role = load.run.role
      load.management.admit(load.run.generation)
      load.heartbeat.begin(load.run.generation)
      if (load.run.hostsSpaces) {
        load.workers.begin()
        try { await load.command() } catch { /* No /workers in this load: the pane stays closed. */ }
      }
      await load.outbox.admit()
      return
    }
    if (answer.status === 410) {
      const seq = renumbered ? undefined : await renumber(load.host, load.epoch, request.loadSeq, answer.text)
      if (seq === undefined) {
        load.heartbeat.superseded = true
        return
      }
      await hello(load, { ...request, loadSeq: seq }, 0, true)
      return
    }
    if (answer.status !== 500 && answer.status !== 503) {
      return
    }
  } catch {
    // No answer is no proof of anything (docs/v2/design-claude.md#when-the-mod-does-not-load):
    // the wrapper says what it has not observed, and the session goes on.
  }
  const next = nextDelay(delay)
  load.host.after(next, () => {
    void hello(load, request, next, renumbered)
  })
}

export const register: Register = (on) => {
  // A load is this module's life: module state resets on every reload, and
  // register and session.start run again, so a reload is a new load with a
  // hello of its own.
  const loadId = newId()
  let load: Load | undefined

  on('session.start', async ($, e, next) => {
    const started = await next(e)
    const socket = await $.env.get('REWAKE_MOD_SOCKET')
    if (!socket) {
      // Loaded outside a rewake run: nobody to report to.
      return started
    }
    const epoch = (await $.env.get('REWAKE_EPOCH')) ?? ''
    const host: Host = {
      post: (path, request) =>
        $.http.fetch('http://rewake' + path, {
          method: 'POST',
          headers: { 'content-type': 'application/json' },
          body: JSON.stringify(request),
          socketPath: socket,
        }),
      get: (key) => $.store.get(key),
      set: (key, value) => $.store.set(key, value),
      remove: (key) => $.store.delete(key),
      after: (ms, fn) => {
        $.clock.after(ms, fn)
      },
      log: (text) => $.ui.log(text),
    }
    const current: Load = {
      host,
      epoch,
      outbox: new Outbox(host, epoch, loadId),
      management: new Management(host, {
        id: () => $.session.id(),
        model: () => $.session.model(),
        usage: () => $.session.usage(),
        compact: (focus) => $.session.compact({ instructions: focus || undefined }),
        abort: (turnId) => $.turn.abort({ turnId }),
        trace: (text) => $.session.append({ message: { type: 'user', content: [{ type: 'text', text }] } }),
      }, loadId),
      workers: new Workers(host, {
        blit: (cells, columns, rows) => $.ui.blit({ requestId: paneId, key: 'screen', columns, rows, cells }),
        redraw: () => { $.ui.invalidate('ui.render') },
      }, loadId),
      asks: new Asks(host, loadId),
      command: async () => {
        await $.command.register({ name: 'workers', description: "Show the workers of main's spaces, their screens and keys", immediate: true })
      },
      // A lapsed generation's chain says the load's latest hello again.
      heartbeat: new Heartbeat(host, loadId, () => {
        if (latest !== undefined) {
          void hello(current, latest, 0, true)
        }
      }, (answer, generation) => {
        current.management.changed()
        if (answer.control) void current.management.control(answer.control, generation)
      }),
    }
    let latest: HelloRequest | undefined
    load = current
    // The run's stored ends are read before this load writes any, whether or
    // not its hello is ever answered.
    await load.outbox.open()
    try {
      latest = {
        loadId,
        sessionId: await $.session.id(),
        harnessVersion: (await $.session.version()).version,
        loadSeq: await takeLoadSeq(host, epoch),
      }
      await hello(load, latest, 0)
    } catch {
      // What the harness would not tell leaves the load unadmitted.
    }
    return started
  })

  // A turn's start of the main loop is sent once and not retried: no start
  // time rides on it, so a lost one costs nothing
  // (docs/v2/stage4-turns.md#the-start-time). A subagent's, which carries its
  // agent id, is not the session's turn and is not sent, as its end is not.
  on('turn.start', async ($, e, next) => {
    if (e.agentId === undefined) load?.management.started(e.turnId)
    const started = await next(e)
    if (load?.run && e.agentId === undefined) {
      try {
        const request: TurnStartRequest = { loadId, conversation: await $.session.id(), turnId: e.turnId }
        await load.host.post(paths.turnStart, request)
      } catch {
        // Nothing is owed on a start.
      }
    }
    return started
  })

  // A turn's end of the main loop goes to the outbox; a subagent's, which
  // carries its agent id, is not the session's turn and is not sent.
  on('turn.complete', async ($, e, next) => {
    const done = await next(e)
    if (load === undefined || e.agentId !== undefined) {
      return done
    }
    const actor = await load.management.ended(e.turnId, e.reason === 'aborted')
    let answer = actor ?? e.answer
    if (answer === '' && e.reason === 'refusal') {
      answer = e.refusal?.explanation ?? ''
    }
    const end: TurnEndRequest = {
      loadId,
      conversation: await $.session.id(),
      turnId: e.turnId,
      endId: newId(),
      reason: reasons[e.reason] ?? 'failed',
    }
    if (answer !== '') {
      end.answer = answer
    }
    await load.outbox.add(cut(end))
    return done
  })

  // The permission mode the harness reports with its classic events, which an
  // ask is carried to main in only where the person would be asked
  // (asks.ts).
  on('classic.SessionStart', async ($, e, next) => {
    if (load && e.permission_mode) load.asks.mode = e.permission_mode
    return next(e)
  })
  on('classic.UserPromptSubmit', async ($, e, next) => {
    if (load && e.permission_mode) load.asks.mode = e.permission_mode
    return next(e)
  })
  on('classic.PostToolUse', async ($, e, next) => {
    if (load && e.permission_mode) load.asks.mode = e.permission_mode
    return next(e)
  })

  // A worker's call the engine would ask the person about is asked as a
  // question with three choices, which the question bridge below carries to
  // main too; an exact call allowed for the session passes without asking.
  // A failure of the bridge refuses the call: nothing has run.
  on('tool.check', async ($, e, next) => {
    const verdict = await next(e)
    if (verdict.decision !== 'ask' || !load?.asks.asking(e)) return verdict
    const cwd = await $.session.cwd()
    const key = load.asks.key(cwd, e)
    if (load.asks.allows(key)) return { decision: 'allow', reason: 'allowed for this session through rewake' }
    const question = load.asks.permission(cwd, e, key)
    try {
      const answer = await $.ui.ask(question, { header: 'Permission', options: [allowOnce, allowSession, refuseCall] })
      return load.asks.decide(question, answer)
    } catch {
      return load.asks.refused(question)
    }
  }).catch(($, e, next) => ({ decision: 'deny', reason: 'rewake could not carry this permission request (' + next.error.message + '), so the call was refused.' }))

  // Every question dialog of a worker — a permission's and the model's own —
  // is opened for main as well, and the first answer wins (asks.ts). A
  // failure before the dialog opened leaves it to the person.
  on('tool.call', { tool: 'AskUserQuestion' }, async ($, e, next) => {
    if (!load?.run || !load.asks.worker()) return next(e)
    return load.asks.bridge(e, () => next(e), next.signal)
  }).catch(($, e, next) => (next.called ? { deny: 'rewake could not carry this question (' + next.error.message + ').' } : next(e)))

  on('session.measure', async ($, e, next) => {
    const measured = await next(e)
    load?.management.changed()
    return measured
  })

  on('session.compact', async ($, e, next) => {
    if (e.agentId !== undefined || !load) return next(e)
    load.management.compactStarted()
    try {
      const result = await next(e)
      load.management.compactEnded(result.skip === undefined)
      return result
    } catch (error) {
      load.management.compactEnded(false)
      throw error
    }
  })

  on('command.run', { command: 'workers' }, async ($) => {
    if (!load?.run?.hostsSpaces) {
      return { text: 'This session opens no spaces: the workers pane is main\'s.' }
    }
    // Escape closes it while the pane holds the keyboard; once a click gave
    // the screen the keys, Escape hands them back to the prompt first.
    await $.ui.open({ id: paneId, title: 'Workers', rows: 40, focus: true, closeOnEscape: true })
    load.workers.opens()
    const name = load.workers.chosenName()
    return { text: name === ''
      ? 'Workers pane: no worker yet; Esc closes it.'
      : `Workers pane: click the screen to type to ${name}; Esc returns to the prompt, and Esc again at an idle, empty prompt or the pane's ✕ closes it.` }
  })

  on('ui.close', async ($, e, next) => {
    if (e.id === paneId) load?.workers.closes()
    return next(e)
  })

  // The key surface posts every key not yet acknowledged, since a post a
  // later one replaces in the same frame is never delivered; the answer's
  // props acknowledge what was taken, and a key is typed once.
  on('ui.message', async ($, e, next) => {
    await next(e)
    if (e.requestId !== paneId || !load) return {}
    const acked = load.workers.take(e.data)
    return acked === undefined ? {} : { props: { name: load.workers.chosenName(), acked } }
  })

  on('ui.render', { component: 'Pane', requestId: paneId }, async ($, e) => drawPane($, e, load?.workers))
}
