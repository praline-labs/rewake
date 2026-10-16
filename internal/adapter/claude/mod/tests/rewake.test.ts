import { expect, mock, test } from 'claude-code/testing'
import { bounds } from '../protocol.gen.ts'
import { utf8Length } from '../hooks/outbox.ts'

const socket = '/state/sock/api.1.mod.sock'
const epoch = '7.100.boot'
const own = 'rewake/outbox/' + epoch
const admitted = { status: 200, text: '{"role":"write","hostsSpaces":false,"generation":1}' }

type Reply = { status: number; text: string } | 'receipt' | 'throw' | ((body: any) => Promise<Reply>)

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
function world(
  on: any,
  given: { replies?: Record<string, Reply[]>; store?: Record<string, unknown>; env?: Record<string, string>; refused?: string } = {},
) {
  const sent: Sent[] = []
  const logs: string[] = []
  const store = new Map<string, unknown>(Object.entries(given.store ?? {}))
  const replies: Record<string, Reply[]> = {
    '/v1/hello': [admitted],
    '/v1/turn/start': [{ status: 200, text: '{"turnId":"t"}' }],
    '/v1/turn/end': ['receipt'],
    '/v1/mod/ended': [{ status: 200, text: '{"ended":[]}' }],
    ...given.replies,
  }
  mock.env(on, given.env ?? { REWAKE_MOD_SOCKET: socket, REWAKE_EPOCH: epoch })
  const clock = mock.clock(on)
  on('store.get', (_: any, e: any) => ({ value: store.get(e.key) }))
  on('store.set', (_: any, e: any) => {
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
  return { sent, logs, store, clock, ends }
}

const start = ($: any) => $.session.start({ surface: 'terminal', isInteractive: true, cwd: '/work' } as any)

const complete = ($: any, turnId: string, fields: Record<string, unknown> = {}) =>
  $.turn.complete({ turnId, reason: 'answer', answer: 'done ' + turnId, durationMs: 1, isAborted: false, usage: null, ...fields } as any)

test('a load says hello to its run over the socket the run names, with the next number of its counter', async ($, on) => {
  const run = world(on, { store: { ['rewake/load/' + epoch]: 4 } })
  await start($)
  expect(run.sent.length).toBe(1)
  const hello = run.sent[0]
  expect(hello.path).toBe('/v1/hello')
  expect(hello.socket).toBe(socket)
  expect(hello.body.sessionId).toBe('conversation-1')
  expect(hello.body.harnessVersion).toBe('2.1.296')
  expect(hello.body.loadId).toMatch(/^[a-z0-9]{8,}$/)
  expect(hello.body.loadSeq).toBe(5)
  expect(run.store.get('rewake/load/' + epoch)).toBe(5)
  expect(run.store.get('rewake/outboxes')).toEqual([epoch])
})

test('a load outside a rewake run reports to nobody', async ($, on) => {
  const run = world(on, { env: {} })
  await start($)
  await complete($, 't1')
  expect(run.sent.length).toBe(0)
})

test('a refused hello leaves the session going and sends no end', async ($, on) => {
  const run = world(on, { replies: { '/v1/hello': [{ status: 403, text: '{"error":"process 7 is not the harness"}' }] } })
  const started = await start($)
  expect(started).toMatchObject({ cwd: '/work' })
  await complete($, 't1')
  await run.clock.advance(60_000)
  expect(run.sent.map((one) => one.path)).toEqual(['/v1/hello'])
})

test('a hello that failed is sent again under the same load id and number, and its ends then go', async ($, on) => {
  const run = world(on, { replies: { '/v1/hello': ['throw', { status: 503, text: '{"error":"closing"}' }, admitted] } })
  await start($)
  await complete($, 't1')
  expect(run.ends().length).toBe(0)
  await run.clock.advance(1000)
  expect(run.sent.filter((one) => one.path === '/v1/hello').length).toBe(2)
  await run.clock.advance(2000)
  const hellos = run.sent.filter((one) => one.path === '/v1/hello')
  expect(hellos.length).toBe(3)
  expect(new Set(hellos.map((one) => one.raw)).size).toBe(1)
  await run.clock.settle()
  expect(run.ends().map((one) => one.body.turnId)).toEqual(['t1'])
})

test('a turn start is sent once, and not before the hello is admitted', async ($, on) => {
  const run = world(on, { replies: { '/v1/turn/start': ['throw'] } })
  await $.turn.start({ text: 'early', turnId: 't0' } as any)
  await start($)
  await $.turn.start({ text: 'go', turnId: 't1' } as any)
  await run.clock.advance(60_000)
  const starts = run.sent.filter((one) => one.path === '/v1/turn/start')
  expect(starts.length).toBe(1)
  expect(starts[0].body).toMatchObject({ conversation: 'conversation-1', turnId: 't1' })
})

test("each reason reaches the run as its end reason, and a subagent's turn is not sent", async ($, on) => {
  const run = world(on)
  await start($)
  await complete($, 't1', { reason: 'answer' })
  await complete($, 't2', { reason: 'aborted', isAborted: true })
  await complete($, 't3', { reason: 'refusal', answer: '', refusal: { category: 'cyber', explanation: 'declined' } })
  await complete($, 't4', { reason: 'error', answer: '' })
  await complete($, 't5', { agentId: 'agent-1' })
  await run.clock.settle()
  expect(run.ends().map((one) => [one.body.turnId, one.body.reason, one.body.answer])).toEqual([
    ['t1', 'answered', 'done t1'],
    ['t2', 'aborted', 'done t2'],
    ['t3', 'refused', 'declined'],
    ['t4', 'failed', undefined],
  ])
  expect(run.store.has(own)).toBe(false)
})

test('the first attempt is awaited by the hook, and the end leaves the store on its receipt', async ($, on) => {
  const run = world(on)
  await start($)
  await complete($, 't1')
  expect(run.ends().length).toBe(1)
  expect(run.ends()[0].body.endId).toMatch(/^[a-z0-9]{8,}$/)
  expect(run.store.has(own)).toBe(false)
})

test('an end that comes while an older one is in flight queues behind it', async ($, on) => {
  let answered = 0
  const run = world(on, {
    replies: {
      '/v1/turn/end': [
        async () => {
          await run.clock.sleep(5000)
          answered++
          return 'receipt'
        },
        'receipt',
      ],
    },
  })
  await start($)
  const first = complete($, 't1')
  await run.clock.settle()
  await complete($, 't2')
  expect(run.ends().map((one) => one.body.turnId)).toEqual(['t1'])
  expect((run.store.get(own) as any[]).map((end) => end.turnId)).toEqual(['t1', 't2'])
  await run.clock.advance(5000)
  await first
  await run.clock.settle()
  expect(answered).toBe(1)
  expect(run.ends().map((one) => one.body.turnId)).toEqual(['t1', 't2'])
  expect(run.store.has(own)).toBe(false)
})

test("a 503 and a 200 that is not the end's receipt are sent again, the same body each time", async ($, on) => {
  const run = world(on, {
    replies: {
      '/v1/turn/end': [{ status: 503, text: '{"error":"unreadable"}' }, { status: 200, text: '{"turnId":"t1","endId":"other"}' }, 'receipt'],
    },
  })
  await start($)
  await complete($, 't1')
  expect(run.ends().length).toBe(1)
  await run.clock.advance(999)
  expect(run.ends().length).toBe(1)
  await run.clock.advance(1)
  expect(run.ends().length).toBe(2)
  await run.clock.advance(2000)
  expect(run.ends().length).toBe(3)
  expect(new Set(run.ends().map((one) => one.raw)).size).toBe(1)
  await run.clock.advance(60_000)
  expect(run.ends().length).toBe(3)
  expect(run.store.has(own)).toBe(false)
})

test('a 409 and a 410 are dropped and said once, and the next end goes on', async ($, on) => {
  const run = world(on, {
    replies: {
      '/v1/turn/end': [
        { status: 409, text: '{"error":"another end under this id"}' },
        { status: 410, text: '{"error":"the run is gone"}' },
        'receipt',
      ],
    },
  })
  await start($)
  await complete($, 't1')
  await complete($, 't2')
  await complete($, 't3')
  await run.clock.advance(60_000)
  expect(run.ends().map((one) => one.body.turnId)).toEqual(['t1', 't2', 't3'])
  expect(run.logs.length).toBe(2)
  expect(run.logs[0]).toContain('409 another end under this id')
  expect(run.logs[1]).toContain('410 the run is gone')
  expect(run.store.has(own)).toBe(false)
})

for (const [name, unit] of [
  ['plain text', 'a'],
  ['quotes and backslashes', '"\\'],
  ['characters outside ASCII', 'жé😀'],
] as const) {
  test(`a long answer of ${name} is cut at the bound, with its line`, { timeoutMs: 30_000 }, async ($, on) => {
    const run = world(on)
    await start($)
    const answer = unit.repeat(Math.ceil((bounds.maxEndBodyBytes * 1.2) / utf8Length(unit)))
    await complete($, 't1', { answer })
    const end = run.ends()[0]
    expect(utf8Length(end.raw) <= bounds.maxEndBodyBytes).toBe(true)
    const marked = /\n\[rewake: (\d+) characters of this answer were left out[^\]]*\]$/.exec(end.body.answer)
    expect(marked).not.toBeNull()
    const kept = Array.from(end.body.answer.slice(0, marked!.index) as string)
    const all = Array.from(answer)
    expect(kept.join('')).toBe(all.slice(0, kept.length).join(''))
    expect(Number(marked![1])).toBe(all.length - kept.length)
    // One more character would carry the body past the bound.
    const longer = { ...end.body, answer: all.slice(0, kept.length + 1).join('') + `\n[rewake: ${all.length - kept.length - 1}` + marked![0].slice(marked![0].indexOf(' characters')) }
    expect(utf8Length(JSON.stringify(longer)) > bounds.maxEndBodyBytes).toBe(true)
  })
}

test("the store's ends of the run go first after a new load's hello, under their own ids; another run's stay", async ($, on) => {
  const other = '9.200.boot'
  const kept = { loadId: 'old', conversation: 'conversation-1', turnId: 't0', endId: 'end-old', reason: 'answered', answer: 'before the reload' }
  const theirs = { loadId: 'theirs', conversation: 'conversation-9', turnId: 'x', endId: 'end-theirs', reason: 'answered' }
  const run = world(on, { store: { [own]: [kept], ['rewake/outbox/' + other]: [theirs], 'rewake/outboxes': [other, epoch] } })
  await start($)
  await run.clock.settle()
  await complete($, 't1')
  await run.clock.settle()
  expect(run.ends().map((one) => one.body.endId)).toEqual(['end-old', run.ends()[1].body.endId])
  expect(run.ends()[0].body).toEqual(kept)
  expect(run.ends()[1].body.turnId).toBe('t1')
  expect(run.sent.find((one) => one.path === '/v1/mod/ended')!.body.epochs).toEqual([other])
  expect(run.store.get('rewake/outbox/' + other)).toEqual([theirs])
  expect(run.store.has(own)).toBe(false)
})

test('only the keys of runs mod/ended answers ended are removed', async ($, on) => {
  const [gone, alive] = ['1.1.boot', '2.2.boot']
  const run = world(on, {
    replies: { '/v1/mod/ended': [{ status: 200, text: JSON.stringify({ ended: [gone, 'never-asked'] }) }] },
    store: {
      ['rewake/outbox/' + gone]: [{ endId: 'g' }],
      ['rewake/load/' + gone]: 3,
      ['rewake/outbox/' + alive]: [{ endId: 'a' }],
      ['rewake/load/' + alive]: 1,
      'rewake/outboxes': [gone, alive],
    },
  })
  await start($)
  expect(run.store.has('rewake/outbox/' + gone)).toBe(false)
  expect(run.store.has('rewake/load/' + gone)).toBe(false)
  expect(run.store.get('rewake/outbox/' + alive)).toEqual([{ endId: 'a' }])
  expect(run.store.get('rewake/load/' + alive)).toBe(1)
  expect(run.store.get('rewake/outboxes')).toEqual([alive, epoch])
})

test("a cleanup cut short loses no end of the run's own", async ($, on) => {
  const gone = '1.1.boot'
  const kept = { loadId: 'old', conversation: 'conversation-1', turnId: 't0', endId: 'end-old', reason: 'answered' }
  const run = world(on, {
    replies: { '/v1/mod/ended': [{ status: 200, text: JSON.stringify({ ended: [gone] }) }] },
    store: { [own]: [kept], ['rewake/outbox/' + gone]: [{ endId: 'g' }], 'rewake/outboxes': [gone, epoch] },
    refused: 'rewake/load/' + gone,
  })
  await start($)
  await run.clock.settle()
  expect(run.ends().map((one) => one.body.endId)).toEqual(['end-old'])
  expect(run.store.has(own)).toBe(false)
  expect(run.store.get('rewake/outboxes')).toEqual([gone, epoch])
})
