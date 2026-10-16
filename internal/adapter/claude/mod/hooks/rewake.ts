// The mod's one module. A load says hello to its run's wrapper at
// session.start and keeps the answer, then reports the turns of the session's
// own conversation: each start once, each end through the outbox until its
// receipt comes. The socket's path and the run's epoch come from the run's
// environment, so the same files serve every run
// (docs/v2/stage4-space.md#2-the-protocol, docs/v2/stage4-turns.md).
import type { Register } from 'claude-code'
import { paths } from '../protocol.gen.ts'
import type { EndReason, HelloAnswer, HelloRequest, TurnEndRequest, TurnStartRequest } from '../protocol.gen.ts'
import { Outbox, cut, listed, loadKey, newId, nextDelay } from './outbox.ts'
import type { Host } from './outbox.ts'

// The harness's reasons, mapped to the protocol's.
const reasons: Record<string, EndReason> = {
  answer: 'answered',
  aborted: 'aborted',
  refusal: 'refused',
  error: 'failed',
}

// takeLoadSeq takes the next number of the run's counter in the store: loads
// of one run follow one another, so the read and the write have no rival.
async function takeLoadSeq(host: Host, epoch: string): Promise<number> {
  if (epoch === '') {
    return 1
  }
  try {
    const held = Number(await host.get(loadKey(epoch)))
    const seq = (Number.isFinite(held) ? held : 0) + 1
    await host.set(loadKey(epoch), seq)
    await listed(host, epoch)
    return seq
  } catch {
    return 1
  }
}

// Load is what a load learned at session.start.
interface Load {
  host: Host
  outbox: Outbox
  // run is the wrapper's answer to this load's hello: undefined until it
  // comes, and when no wrapper answered, which later steps read as "not live".
  run?: HelloAnswer
}

// hello is sent until it is answered, under the same load id and number: a
// fetch that threw, a 500 or a 503 sends it again after 1 s up to 10 s. Any
// other refusal is final for the load: the wrapper refused it and knows why,
// and the mod serves nothing it did not admit. A 410's second hello under a
// higher number, and the heartbeat, are part B's.
async function hello(load: Load, request: HelloRequest, delay: number) {
  try {
    const answer = await load.host.post(paths.hello, request)
    if (answer.ok) {
      load.run = JSON.parse(answer.text) as HelloAnswer
      await load.outbox.admit()
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
    void hello(load, request, next)
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
    load = { host, outbox: new Outbox(host, epoch, loadId) }
    // The run's stored ends are read before this load writes any, whether or
    // not its hello is ever answered.
    await load.outbox.open()
    try {
      const request: HelloRequest = {
        loadId,
        sessionId: await $.session.id(),
        harnessVersion: (await $.session.version()).version,
        loadSeq: await takeLoadSeq(host, epoch),
      }
      await hello(load, request, 0)
    } catch {
      // What the harness would not tell leaves the load unadmitted.
    }
    return started
  })

  // A turn's start is sent once and not retried: no start time rides on it,
  // so a lost one costs nothing (docs/v2/stage4-turns.md#the-start-time).
  on('turn.start', async ($, e, next) => {
    const started = await next(e)
    if (load?.run) {
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
    let answer = e.answer
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
}
