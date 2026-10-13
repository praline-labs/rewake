import { expect, mock, test } from 'claude-code/testing'

function answering(on: any, sent: any[], reply: { status: number; text: string }) {
  on('session.start', () => ({ cwd: '/work' }))
  on('session.id', () => ({ value: 'conversation-1' }))
  on('session.version', () => ({ value: { version: '2.1.296', base: '2.1.296' } }))
  on('http.fetch', (_: any, e: any) => {
    sent.push(e)
    return { value: { ok: reply.status === 200, status: reply.status, text: reply.text, headers: {} } }
  })
}

test('a load says hello to its run over the socket the run names', async ($, on) => {
  const sent: any[] = []
  mock.env(on, { REWAKE_MOD_SOCKET: '/state/sock/main.1.mod.sock' })
  answering(on, sent, { status: 200, text: '{"role":"main","hostsSpaces":false}' })
  await $.session.start({ surface: 'terminal', isInteractive: true, cwd: '/work' } as any)
  expect(sent.length).toBe(1)
  expect(sent[0].url).toBe('http://rewake/v1/hello')
  expect(sent[0].init.method).toBe('POST')
  expect(sent[0].init.socketPath).toBe('/state/sock/main.1.mod.sock')
  const body = JSON.parse(sent[0].init.body)
  expect(body.sessionId).toBe('conversation-1')
  expect(body.harnessVersion).toBe('2.1.296')
  expect(body.loadId).toMatch(/^[a-z0-9]{8,}$/)
})

test('a load outside a rewake run reports to nobody', async ($, on) => {
  const sent: any[] = []
  mock.env(on, {})
  answering(on, sent, { status: 200, text: '{}' })
  await $.session.start({ surface: 'terminal', isInteractive: true, cwd: '/work' } as any)
  expect(sent.length).toBe(0)
})

test('a refused hello leaves the session going', async ($, on) => {
  const sent: any[] = []
  mock.env(on, { REWAKE_MOD_SOCKET: '/state/sock/main.1.mod.sock' })
  answering(on, sent, { status: 403, text: '{"error":"process 7 is not the harness"}' })
  const started = await $.session.start({ surface: 'terminal', isInteractive: true, cwd: '/work' } as any)
  expect(sent.length).toBe(1)
  expect(started).toMatchObject({ cwd: '/work' })
})
