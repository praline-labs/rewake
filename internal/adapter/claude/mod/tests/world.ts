// The run around the mod, as the mod's tests stand it up.
import { mock } from 'claude-code/testing'

export const socket = '/state/sock/api.1.mod.sock'
export const epoch = '7.100.boot'
export const own = 'rewake/outbox/' + epoch
export const admitted = { status: 200, text: '{"role":"write","generation":1}' }
export const holdMs = 15_000

export type Reply = { status: number; text: string } | 'receipt' | 'throw' | ((body: any) => Promise<Reply>)

interface Sent {
  path: string
  raw: string
  body: any
  socket: string
}

// world stands for the run around the mod: its environment, the harness's
// store and clock, and the run's endpoint, whose replies for a path go in
// order, the last repeating; an end is answered with its receipt unless the
// test says otherwise.
export function world(
  on: any,
  given: {
    usage?: () => unknown
    replies?: Record<string, Reply[]>
    store?: Record<string, unknown>
    env?: Record<string, string>
    refused?: string
    // unwritable is a key the store refuses to write.
    unwritable?: string
    // unreadable is how many reads of the run's outbox the store refuses.
    unreadable?: number
  } = {},
) {
  let unreadable = given.unreadable ?? 0
  const sent: Sent[] = []
  const logs: string[] = []
  const store = new Map<string, unknown>(Object.entries(given.store ?? {}))
  mock.env(on, given.env ?? { REWAKE_MOD_SOCKET: socket, REWAKE_EPOCH: epoch })
  const clock = mock.clock(on)
  const held: Reply = async () => {
    await clock.sleep(holdMs)
    return { status: 200, text: '{}' }
  }
  const replies: Record<string, Reply[]> = {
    '/v1/hello': [admitted],
    '/v1/turn/start': [{ status: 200, text: '{"turnId":"t"}' }],
    '/v1/turn/end': ['receipt'],
    '/v1/mod/state': [{ status: 200, text: '{}' }],
    '/v1/mod/control': [{ status: 200, text: '{}' }],
    '/v1/mod/ended': [{ status: 200, text: '{"ended":[]}' }],
    // A poll is held as the endpoint holds it, then renewed.
    '/v1/mod/alive': [held],
    ...given.replies,
  }
  on('store.get', (_: any, e: any) => {
    if (e.key === own && unreadable > 0) {
      unreadable--
      return { deny: 'the store is busy' }
    }
    return { value: store.get(e.key) }
  })
  on('store.set', (_: any, e: any) => {
    if (e.key === given.unwritable) {
      return { deny: 'the store is full' }
    }
    store.set(e.key, JSON.parse(JSON.stringify(e.value)))
    return { value: undefined }
  })
  on('store.delete', (_: any, e: any) => {
    if (e.key === given.refused) {
      return { deny: 'the harness reloaded the mod' }
    }
    store.delete(e.key)
    return { value: undefined }
  })
  on('ui.log', (_: any, e: any) => {
    logs.push(e.text)
    return { value: undefined }
  })
  on('session.start', () => ({ cwd: '/work' }))
  on('session.id', () => ({ value: 'conversation-1' }))
  on('session.usage', () => ({ value: { context: given.usage?.() ?? { window: 100000, tokens: 25000, percent: 25 }, rateLimits: [] } }))
  on('session.model', () => ({ value: 'test-model' }))
  on('session.version', () => ({ value: { version: '2.1.296', base: '2.1.296' } }))
  on('turn.start', (_: any, e: any) => ({ turnId: e.turnId }))
  on('turn.complete', (_: any, e: any) => ({ text: e.answer }))
  on('http.fetch', async (_: any, e: any) => {
    const path = e.url.replace('http://rewake', '')
    const body = JSON.parse(e.init.body)
    sent.push({ path, raw: e.init.body, body, socket: e.init.socketPath })
    const queue = replies[path] ?? [{ status: 501, text: '{"error":"not served"}' }]
    let reply = queue.length > 1 ? queue.shift()! : queue[0]
    while (typeof reply === 'function') {
      reply = await reply(body)
    }
    if (reply === 'throw') {
      return { deny: 'the socket is gone' }
    }
    if (reply === 'receipt') {
      reply = { status: 200, text: JSON.stringify({ turnId: body.turnId, endId: body.endId }) }
    }
    return { value: { ok: reply.status === 200, status: reply.status, text: reply.text, headers: {} } }
  })
  const ends = () => sent.filter((one) => one.path === '/v1/turn/end')
  const polls = () => sent.filter((one) => one.path === '/v1/mod/alive')
  const hellos = () => sent.filter((one) => one.path === '/v1/hello')
  return { sent, logs, store, clock, ends, polls, hellos }
}

export const start = ($: any) => $.session.start({ surface: 'terminal', isInteractive: true, cwd: '/work' } as any)

export const complete = ($: any, turnId: string, fields: Record<string, unknown> = {}) =>
  $.turn.complete({ turnId, reason: 'answer', answer: 'done ' + turnId, durationMs: 1, isAborted: false, usage: null, ...fields } as any)
