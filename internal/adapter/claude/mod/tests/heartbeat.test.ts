import { expect, test } from 'claude-code/testing'
import { Heartbeat } from '../hooks/heartbeat.ts'
import type { Host } from '../hooks/outbox.ts'
import { epoch, holdMs, start, world } from './world.ts'
import type { Reply } from './world.ts'

const loadKey = 'rewake/load/' + epoch
const answered = (generation: number) => ({ status: 200, text: JSON.stringify({ role: 'write', generation }) })
const refused = (status: number, body: Record<string, unknown> = { error: 'no' }) => ({ status, text: JSON.stringify(body) })

test('an admitted load polls with its generation, one poll at a time, the next at once after each answer', async ($, on) => {
  const run = world(on)
  await start($)
  await run.clock.settle()
  expect(run.polls().map((one) => one.body)).toEqual([{ loadId: run.hellos()[0].body.loadId, generation: 1 }])
  await run.clock.advance(holdMs)
  expect(run.polls().length).toBe(2)
  await run.clock.advance(holdMs)
  expect(run.polls().length).toBe(3)
  expect(run.logs).toEqual([])
})

test('a 409 of its newest generation says the same hello again and polls the generation that answers', async ($, on) => {
  const run = world(on, {
    replies: {
      '/v1/hello': [answered(1), answered(2)],
      '/v1/mod/alive': [
        refused(409),
        async () => {
          await run.clock.sleep(holdMs)
          return { status: 200, text: '{}' }
        },
      ],
    },
  })
  await start($)
  await run.clock.settle()
  const hellos = run.hellos()
  expect(hellos.length).toBe(2)
  expect(hellos[1].body).toEqual(hellos[0].body)
  expect(run.polls().map((one) => one.body.generation).slice(0, 2)).toEqual([1, 2])
})

test('a 410 ends the chain, and the load says no hello again', async ($, on) => {
  const run = world(on, { replies: { '/v1/mod/alive': [refused(410)] } })
  await start($)
  await run.clock.advance(60_000)
  expect(run.polls().length).toBe(1)
  expect(run.hellos().length).toBe(1)
})

for (const status of [400, 403]) {
  test(`a ${status} ends the chain and is said once`, async ($, on) => {
    const run = world(on, { replies: { '/v1/mod/alive': [refused(status)] } })
    await start($)
    await run.clock.advance(60_000)
    expect(run.polls().length).toBe(1)
    expect(run.logs.filter((line) => line.includes('heartbeat')).length).toBe(1)
  })
}

test('a throw, a 500 and a 503 send the same poll again after 1 s, doubling up to 10 s', async ($, on) => {
  const at: number[] = []
  const timed = (reply: Reply): Reply => async () => {
    at.push(run.clock.now())
    return reply
  }
  const run = world(on, {
    replies: {
      '/v1/mod/alive': [timed('throw'), timed(refused(500)), timed(refused(503)), timed('throw'), timed('throw'), timed('throw'), timed(refused(410))],
    },
  })
  await start($)
  await run.clock.advance(60_000)
  expect(at).toEqual([0, 1000, 3000, 7000, 15000, 25000, 35000])
  expect(new Set(run.polls().map((one) => one.raw)).size).toBe(1)
})

test('a counter the store would not write still gives the load the next number', async ($, on) => {
  const run = world(on, { store: { [loadKey]: 4 }, unwritable: loadKey })
  await start($)
  expect(run.hellos()[0].body.loadSeq).toBe(5)
})

test('a load refused 410 that reads no number above its own says hello once more above the refusal’s, and polls', async ($, on) => {
  const run = world(on, {
    store: { [loadKey]: 4 },
    replies: { '/v1/hello': [refused(410, { error: 'superseded', loadSeq: 7 }), answered(3)] },
  })
  await start($)
  await run.clock.settle()
  const hellos = run.hellos()
  expect(hellos.map((one) => one.body.loadSeq)).toEqual([5, 8])
  expect(hellos[1].body.loadId).toBe(hellos[0].body.loadId)
  expect(run.store.get(loadKey)).toBe(8)
  expect(run.polls()[0].body.generation).toBe(3)
})

test('a second 410 ends the load', async ($, on) => {
  const twice = world(on, { replies: { '/v1/hello': [refused(410, { error: 'superseded', loadSeq: 7 })] } })
  await start($)
  await twice.clock.advance(60_000)
  expect(twice.hellos().length).toBe(2)
  expect(twice.polls().length).toBe(0)
})

test('a 410 ends the load when the store shows a load numbered above it', async ($, on) => {
  const later = world(on, {
    replies: {
      '/v1/hello': [
        async () => {
          later.store.set(loadKey, 9)
          return refused(410, { error: 'superseded', loadSeq: 9 })
        },
      ],
    },
  })
  await start($)
  await later.clock.advance(60_000)
  expect(later.hellos().length).toBe(1)
  expect(later.polls().length).toBe(0)
})

// A host the test drives by hand: each post waits until the test answers it,
// and each timer until the test runs it.
function handHost() {
  const posts: { request: any; answer: (status: number) => void }[] = []
  const timers: (() => void)[] = []
  const host: Host = {
    post: (_path, request) =>
      new Promise((resolve) => posts.push({ request, answer: (status) => resolve({ status, ok: status === 200, text: '{}' }) })),
    get: async () => undefined,
    set: async () => {},
    remove: async () => {},
    after: (_ms, fn) => {
      timers.push(fn)
    },
    log: () => {},
  }
  const tick = async () => {
    const due = timers.splice(0)
    due.forEach((fn) => fn())
    await new Promise((resolve) => setTimeout(resolve, 0))
  }
  return { host, posts, timers, tick }
}

test('a poll of an older generation answered after a newer one began ends its chain, whatever its answer', async () => {
  for (const status of [200, 409, 503]) {
    const { host, posts, timers, tick } = handHost()
    let again = 0
    const heartbeat = new Heartbeat(host, 'load-1', () => again++)
    heartbeat.begin(1)
    await tick()
    heartbeat.begin(2)
    heartbeat.begin(2)
    await tick()
    expect(posts.map((one) => one.request.generation)).toEqual([1, 2])
    posts[0].answer(status)
    await tick()
    expect(timers.length).toBe(0)
    expect(posts.length).toBe(2)
    expect(again).toBe(0)
  }
})
