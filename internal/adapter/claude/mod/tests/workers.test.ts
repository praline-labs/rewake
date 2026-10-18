import { expect, test } from 'claude-code/testing'
import { bytes } from '../hooks/keys.ts'
import { Workers } from '../hooks/workers.ts'
import { holdMs, start, world } from './world.ts'

const main = { status: 200, text: '{"role":"main","hostsSpaces":true,"generation":1}' }
const api = { space: 's1', name: 'api', session: 'api-claude', harness: 'claude', state: 'open', activity: 'working', model: 'm1', percent: 34 }
const web = { space: 's2', name: 'web', session: 'web-pi', harness: 'pi', state: 'open' }
const listed = (...workers: unknown[]) => ({ status: 200, text: JSON.stringify({ workers }) })
// cells is a screen of 4 by 2 x's in the default colors, as the host sends it.
const cells = (() => {
  const view = new DataView(new ArrayBuffer(8 * 12))
  for (let i = 0; i < 8; i++) {
    view.setUint32(i * 12, 0x78, true)
    view.setUint32(i * 12 + 4, 0x01000000, true)
    view.setUint32(i * 12 + 8, 0x01000000, true)
  }
  return btoa(String.fromCharCode(...new Uint8Array(view.buffer)))
})()
const screen = (seq: number, ended = false) => ({ status: 200, text: JSON.stringify({ seq, columns: 4, rows: 2, cells, ended }) })

// What Claude Code passes a ui.render hook for the pane: a body of 4 columns
// and, less the list's line and the key line, 2 rows.
const pane = (rows = 4) => ({
  plugin: 'rewake', component: 'Pane', requestId: 'rewake-workers', surface: 'terminal',
  viewport: { columns: 100, rows: 30 },
  props: { title: 'Workers', isFocused: true, bodyColumns: 4, placement: 'inline', scroll: { offset: 0, bodyRows: rows }, view: {} },
}) as any

function harness(on: any) {
  const opened: any[] = []
  const blits: any[] = []
  const closed: any[] = []
  on('command.register', () => ({ value: undefined }))
  on('ui.open', (_: any, e: any) => {
    opened.push(e)
    return { value: { isPlaced: true } }
  })
  on('ui.blit', (_: any, e: any) => {
    blits.push(e)
    return { value: {} }
  })
  on('ui.invalidate', () => ({ value: undefined }))
  on('ui.close', (_: any, e: any) => {
    closed.push(e)
    return { value: undefined }
  })
  on('ui.message', () => ({}))
  return { opened, blits, closed }
}

test("a main's pane opens on /workers alone, with its first worker, draws its screen and blits the next", async ($, on) => {
  const run = world(on, {
    replies: {
      '/v1/hello': [main],
      '/v1/workers/list': [listed(api)],
      '/v1/workers/screen': [screen(1), screen(2), async () => {
        await run.clock.sleep(holdMs)
        return screen(2)
      }],
      '/v1/workers/resize': [{ status: 200, text: '{}' }],
    },
  })
  const ui = harness(on)
  await start($)
  await run.clock.settle()
  expect(ui.opened).toEqual([])
  const answer = await $.command.run({ command: 'workers', args: '' } as any)
  expect(ui.opened.map((one) => [one.id, one.focus, one.closeOnEscape])).toEqual([['rewake-workers', true, true]])
  expect(answer.text).toMatch(/click the screen to type to api; there Alt\+a then q closes the pane/)
  await run.clock.settle()
  const drawn = await $.ui.mount(pane())
  expect(await drawn.find({ type: 'Button', text: /api {2}claude {2}m1 {2}ctx 34% {2}working/ })).toBeDefined()
  expect((await drawn.find({ key: 'screen' }))?.props).toEqual(expect.objectContaining({ columns: 4, rows: 2, cells }))
  await run.clock.settle()
  expect(ui.blits.map((one) => [one.requestId, one.key, one.cells])).toEqual([['rewake-workers', 'screen', cells]])
  expect(run.sent.filter((one) => one.path === '/v1/workers/resize').map((one) => one.body)).toEqual([
    { loadId: run.hellos()[0].body.loadId, space: 's1', columns: 100, rows: 30 },
  ])
  expect(run.sent.filter((one) => one.path === '/v1/workers/screen').map((one) => one.body.after).slice(0, 3)).toEqual([0, 1, 2])
})

test('keys typed in the key line reach the chosen worker in order, Escape and Ctrl+C through the prefix', async ($, on) => {
  const run = world(on, {
    replies: {
      '/v1/hello': [main],
      '/v1/workers/list': [listed(api, web)],
      '/v1/workers/screen': [screen(1), async () => {
        await run.clock.sleep(holdMs)
        return screen(1)
      }],
      '/v1/workers/resize': [{ status: 200, text: '{}' }],
      '/v1/workers/keys': [{ status: 200, text: '{}' }],
    },
  })
  harness(on)
  await start($)
  await run.clock.settle()
  await $.command.run({ command: 'workers', args: '' } as any)
  await run.clock.settle()
  const drawn = await $.ui.mount(pane(5))
  await drawn.press({ key: 'worker-s2' })
  await run.clock.settle()
  await drawn.unmount()
  const again = await $.ui.mount(pane(5))
  expect(await again.find({ type: 'Button', text: /▸ web/ })).toBeDefined()
  for (const key of [{ key: 'l' }, { key: 's' }, { key: 'return' }, { key: 'a', meta: true }, { key: 'e' }, { key: 'a', meta: true }, { key: 'c' }, { key: 'a', meta: true }, { key: 'z' }]) {
    await again.key({ key: 'keys', ...key } as any)
  }
  await run.clock.settle()
  const typed = run.sent.filter((one) => one.path === '/v1/workers/keys').map((one) => [one.body.space, one.body.keys])
  expect(typed).toEqual([['s2', 'l'], ['s2', 's'], ['s2', '\r'], ['s2', '\x1b'], ['s2', '\x03']])
})

test('the prefix chooses a worker by its digit and closes the pane, with no Escape', async ($, on) => {
  const run = world(on, {
    replies: {
      '/v1/hello': [main],
      '/v1/workers/list': [listed(api, web)],
      '/v1/workers/screen': [screen(1), async () => {
        await run.clock.sleep(holdMs)
        return screen(1)
      }],
      '/v1/workers/resize': [{ status: 200, text: '{}' }],
      '/v1/workers/keys': [{ status: 200, text: '{}' }],
    },
  })
  const ui = harness(on)
  await start($)
  await run.clock.settle()
  await $.command.run({ command: 'workers', args: '' } as any)
  await run.clock.settle()
  const drawn = await $.ui.mount(pane(5))
  expect(JSON.stringify(await drawn.drawn({ in: 'keys' }))).toContain('Alt+a: q close · 1-9 worker')
  for (const key of [{ key: 'a', meta: true }, { key: '2' }, { key: 'w' }]) {
    await drawn.key({ key: 'keys', ...key } as any)
  }
  await run.clock.settle()
  await drawn.unmount()
  const again = await $.ui.mount(pane(5))
  expect(await again.find({ type: 'Button', text: /▸ web/ })).toBeDefined()
  for (const key of [{ key: 'a', meta: true }, { key: 'q' }]) {
    await again.key({ key: 'keys', ...key } as any)
  }
  await run.clock.settle()
  const typed = run.sent.filter((one) => one.path === '/v1/workers/keys').map((one) => [one.body.space, one.body.keys])
  expect(typed).toEqual([['s2', 'w']])
  expect(ui.closed.map((one) => one.id)).toEqual(['rewake-workers'])
})

test('a worker whose screen ended is said to have ended, and no key goes to it', async ($, on) => {
  const run = world(on, {
    replies: {
      '/v1/hello': [main],
      '/v1/workers/list': [listed(api)],
      '/v1/workers/screen': [screen(1, true)],
      '/v1/workers/resize': [{ status: 200, text: '{}' }],
    },
  })
  harness(on)
  await start($)
  await run.clock.settle()
  await $.command.run({ command: 'workers', args: '' } as any)
  await run.clock.settle()
  const drawn = await $.ui.mount(pane())
  await run.clock.settle()
  await drawn.unmount()
  const again = await $.ui.mount(pane())
  expect(await again.find({ type: 'Text', text: /api ended/ })).toBeDefined()
  expect(await again.find({ key: 'screen' })).toBeUndefined()
  expect(await again.find({ key: 'keys' })).toBeUndefined()
})

test("a session that opens no spaces asks for no workers, and its /workers says the pane is main's", async ($, on) => {
  const run = world(on)
  harness(on)
  await start($)
  await run.clock.advance(20_000)
  expect(run.sent.filter((one) => one.path.startsWith('/v1/workers/')).length).toBe(0)
  const answer = await $.command.run({ command: 'workers', args: '' } as any)
  expect(answer.text).toContain("main's")
})

test('a key becomes the bytes a terminal sends', () => {
  expect(bytes({ key: 'up' })).toBe('\x1b[A')
  expect(bytes({ key: 'tab', shift: true })).toBe('\x1b[Z')
  expect(bytes({ key: 'b', ctrl: true })).toBe('\x02')
  expect(bytes({ key: 'x', meta: true })).toBe('\x1bx')
  expect(bytes({ key: 'backspace' })).toBe('\x7f')
  expect(bytes({ key: 'привет' })).toBe('привет')
})

// A post the next one replaced in its frame is never delivered, so each post
// carries every key not acknowledged: a key is typed once, in order, however
// many posts carried it.
test('keys carried by several posts are typed once, in order', async ($, on) => {
  const posted: string[] = []
  const host = {
    post: async (_: string, request: any) => {
      posted.push(request.keys)
      return { status: 200, ok: true, text: '{}' }
    },
    get: async () => undefined, set: async () => {}, remove: async () => {},
    after: (_: number, fn: () => void) => { fn() }, log: () => {},
  }
  const shown = { blit: async () => ({}), redraw: () => {} }
  const workers = new Workers(host, shown, 'load')
  workers.workers = [api as any]
  workers.chosen = 's1'
  expect(workers.take({ from: 'k1', keys: [{ n: 1, keys: 'a' }, { n: 2, keys: 'b' }] })).toBe(2)
  // The first post was lost; the second carries its keys again.
  expect(workers.take({ from: 'k1', keys: [{ n: 2, keys: 'b' }, { n: 3, keys: 'c' }] })).toBe(3)
  expect(workers.take({ from: 'k2', keys: [{ n: 1, keys: 'd' }] })).toBe(1)
  await new Promise((resolve) => setTimeout(resolve, 0))
  await new Promise((resolve) => setTimeout(resolve, 0))
  expect(posted).toEqual(['a', 'b', 'c', 'd'])
})
