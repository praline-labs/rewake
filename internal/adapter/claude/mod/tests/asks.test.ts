import { expect, test } from 'claude-code/testing'
import { unanswered } from '../hooks/asks.ts'
import { world, start } from './world.ts'
import type { Reply } from './world.ts'

const opened: Reply = { status: 200, text: '{"id":"ab12","lifetimeMs":120000}' }
const pending: Reply = { status: 200, text: '{"state":"pending"}' }

// answered is main's answer to the dialog's one question, read from what the
// mod opened.
function answered(answer: string, reason?: string): Reply {
  return { status: 200, text: JSON.stringify({ state: 'answered', answers: [{ question: '?', answer }], ...(reason ? { reason } : {}) }) }
}

// engine stands for what is below the mod: the verdict a check comes to, the
// session's directory, and the question dialog, which the person answers
// with local after waitMs, or never.
function engine(on: any, run: ReturnType<typeof world>, local?: string, waitMs = 10 ** 9) {
  on('tool.check', () => ({ decision: 'ask' }))
  on('session.cwd', () => ({ value: '/work' }))
  on('classic.SessionStart', () => ({}))
  on('tool.call', async (_: any, e: any) => {
    await run.clock.sleep(waitMs)
    return { result: { questions: e.questions, answers: { [e.questions[0].question]: local } } }
  })
}

// The poll answers carry the question the mod opened, as the wrapper's do.
function keyed(run: ReturnType<typeof world>, reply: Reply): Reply {
  return async () => {
    if (typeof reply !== 'object') return reply
    const question = run.sent.filter((one) => one.path === '/v1/ask/open').at(-1)?.body.questions[0].question
    return { status: reply.status, text: reply.text.replaceAll('"?"', JSON.stringify(question)) }
  }
}

// worker is a worker's load in the given mode; its polls are answered by
// replies['/v1/ask/poll'] as it stands when each poll comes, so a test may set
// it once it knows the run.
async function worker($: any, on: any, replies: Record<string, Reply[]>, local?: string, waitMs?: number, mode = 'default') {
  const queue = (path: string): Reply => async (body: any) => {
    const list = replies[path] ?? [{ status: 501, text: '{"error":"not served"}' }]
    const reply = list.length > 1 ? list.shift()! : list[0]
    return typeof reply === 'function' ? reply(body) : reply
  }
  const run = world(on, { replies: { '/v1/ask/open': [queue('/v1/ask/open')], '/v1/ask/poll': [queue('/v1/ask/poll')] } })
  engine(on, run, local, waitMs)
  await start($)
  await $.classic.SessionStart({ source: 'startup', permission_mode: mode })
  return run
}

const settled = (run: ReturnType<typeof world>) => run.sent.filter((one) => one.path === '/v1/ask/settle').map((one) => one.body.winner)

test('a permission main allows once lets the call run, and main is told of the exact call', async ($, on) => {
  const replies: Record<string, Reply[]> = { '/v1/ask/open': [opened] }
  const run = await worker($, on, replies)
  replies['/v1/ask/poll'] = [keyed(run, answered('Allow once'))]
  const verdict = await $.tool.check({ tool: 'Bash', input: { command: 'make test', timeout: 5 }, tool_use_id: 't1' } as any)
  expect(verdict).toMatchObject({ decision: 'allow' })
  const open = run.sent.find((one) => one.path === '/v1/ask/open')!.body
  expect(open).toMatchObject({ kind: 'permission', tool: 'Bash', input: '{"command":"make test","timeout":5}', cwd: '/work' })
  expect(open.questions[0].options.map((one: any) => one.label)).toEqual(['Allow once', 'Allow for this session', 'Refuse'])
  expect(settled(run)).toEqual(['main'])
})

test('a call allowed for the session passes again without asking, and another call is asked', async ($, on) => {
  const replies: Record<string, Reply[]> = { '/v1/ask/open': [opened] }
  const run = await worker($, on, replies)
  replies['/v1/ask/poll'] = [keyed(run, answered('Allow for this session'))]
  const call = { tool: 'Bash', input: { command: 'make' }, tool_use_id: 't1' }
  expect(await $.tool.check(call as any)).toMatchObject({ decision: 'allow' })
  expect(await $.tool.check({ ...call, input: { command: 'make' }, tool_use_id: 't2' } as any)).toMatchObject({ decision: 'allow' })
  expect(run.sent.filter((one) => one.path === '/v1/ask/open').length).toBe(1)
  replies['/v1/ask/poll'] = [keyed(run, answered('Refuse', 'not on this branch'))]
  const refused = await $.tool.check({ ...call, input: { command: 'make clean' }, tool_use_id: 't3' } as any)
  expect(refused).toMatchObject({ decision: 'deny' })
  expect((refused as any).reason).toContain('not on this branch')
})

test('the person at the screen answering first decides, and the wrapper hears it', async ($, on) => {
  const replies: Record<string, Reply[]> = { '/v1/ask/open': [opened] }
  const run = await worker($, on, replies, 'Refuse', 1000)
  replies['/v1/ask/poll'] = [async () => {
    // The wrapper holds the poll until the settling wakes it.
    while (!settled(run).includes('local')) await run.clock.sleep(100)
    return { status: 200, text: '{"state":"settled"}' }
  }]
  const checking = $.tool.check({ tool: 'Write', input: { file_path: '/work/a', content: 'x' }, tool_use_id: 't1' } as any)
  await run.clock.advance(2000)
  expect(await checking).toMatchObject({ decision: 'deny' })
  expect(settled(run)).toEqual(['local'])
})

test('a permission nobody answers by the deadline is refused, telling the model not to retry blindly', async ($, on) => {
  const replies: Record<string, Reply[]> = { '/v1/ask/open': [opened], '/v1/ask/poll': [pending, { status: 200, text: '{"state":"expired"}' }] }
  const run = await worker($, on, replies)
  const verdict = await $.tool.check({ tool: 'Bash', input: { command: 'deploy' }, tool_use_id: 't1' } as any)
  expect(verdict).toEqual({ decision: 'deny', reason: unanswered })
  expect(run.sent.filter((one) => one.path === '/v1/ask/poll').length).toBe(2)
  expect(settled(run)).toEqual(['expired'])
})

test("the model's own question keeps its choices, and main's answer is its result", async ($, on) => {
  const replies: Record<string, Reply[]> = { '/v1/ask/open': [opened] }
  const run = await worker($, on, replies)
  replies['/v1/ask/poll'] = [keyed(run, answered('Beta, Gamma'))]
  const questions = [{ question: 'Which probe option?', header: 'Probe', multiSelect: true, options: [{ label: 'Alpha', description: 'first' }, { label: 'Beta' }, { label: 'Gamma' }] }]
  const result = await $.tool.call({ tool: 'AskUserQuestion', questions } as any)
  expect(result).toEqual({ result: { questions, answers: { 'Which probe option?': 'Beta, Gamma' } } })
  const open = run.sent.find((one) => one.path === '/v1/ask/open')!.body
  expect(open).toMatchObject({ kind: 'question', questions: [{ question: 'Which probe option?', header: 'Probe', multiSelect: true }] })
  expect(open.questions[0].options[0]).toEqual({ label: 'Alpha', description: 'first' })
  expect(settled(run)).toEqual(['main'])
})

test("main's own asks stay with the engine", async ($, on) => {
  const run = world(on, { replies: { '/v1/hello': [{ status: 200, text: '{"role":"main","hostsSpaces":false,"generation":1}' }] } })
  engine(on, run)
  await start($)
  await $.classic.SessionStart({ source: 'startup', permission_mode: 'default' })
  expect(await $.tool.check({ tool: 'Bash', input: { command: 'make' }, tool_use_id: 't1' } as any)).toEqual({ decision: 'ask' })
  expect(run.sent.some((one) => one.path === '/v1/ask/open')).toBe(false)
})

test("a worker's ask in a mode the person is not asked in stays with the engine", async ($, on) => {
  const run = await worker($, on, {}, undefined, undefined, 'auto')
  expect(await $.tool.check({ tool: 'Bash', input: { command: 'make' }, tool_use_id: 't1' } as any)).toEqual({ decision: 'ask' })
  expect(run.sent.some((one) => one.path === '/v1/ask/open')).toBe(false)
})
