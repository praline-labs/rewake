// The mod's one module. A load says hello to its run's wrapper at
// session.start and keeps the answer; what the run is (its role, whether it
// hosts spaces) decides what later steps register. The socket's path comes
// from the run's environment, so the same files serve every run
// (docs/v2/stage4-space.md#2-the-protocol).
import type { Register } from 'claude-code'
import { paths } from '../protocol.gen.ts'
import type { HelloAnswer, HelloRequest } from '../protocol.gen.ts'

// A load is this module's life: module state resets on every reload, and
// register and session.start run again, so a reload is a new load with a
// hello of its own.
const loadId = Math.random().toString(36).slice(2) + Math.random().toString(36).slice(2)

// run is the wrapper's answer to this load's hello: undefined until it comes,
// and when no wrapper answered, which later steps read as "not live".
let run: HelloAnswer | undefined

export const register: Register = (on) => {
  on('session.start', async ($, e, next) => {
    const started = await next(e)
    const socket = await $.env.get('REWAKE_MOD_SOCKET')
    if (!socket) {
      // Loaded outside a rewake run: nobody to report to.
      return started
    }
    run = undefined
    try {
      const request: HelloRequest = {
        loadId,
        sessionId: await $.session.id(),
        harnessVersion: (await $.session.version()).version,
      }
      const answer = await $.http.fetch('http://rewake' + paths.hello, {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify(request),
        socketPath: socket,
      })
      // A refusal leaves the load unanswered: the wrapper refused it and
      // knows why, and the mod serves nothing it did not admit.
      if (answer.ok) {
        run = JSON.parse(answer.text) as HelloAnswer
      }
    } catch {
      // No answer is no proof of anything (docs/v2/design-claude.md#when-the-mod-does-not-load):
      // the wrapper says what it has not observed, and the session goes on.
    }
    return started
  })
}
