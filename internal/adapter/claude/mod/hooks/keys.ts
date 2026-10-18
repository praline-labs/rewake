// The key surface of the workers' pane (docs/v2/workers-panel.md): a Client
// line that turns each key into the bytes a terminal sends and posts them to
// the hooks module, which types them into the chosen worker. A click gives it
// the keyboard; Escape never arrives here, it returns the keyboard to the
// prompt. The prefix carries what does not arrive: Alt+a, then c for Ctrl+C,
// e or [ for Escape, x for Ctrl+X, Alt+a for Alt+a itself; any other key
// cancels it (P15b, docs/research-claude-mod.md).

const named: Record<string, string> = {
  return: '\r', enter: '\r', tab: '\t', backspace: '\x7f', delete: '\x1b[3~', space: ' ', escape: '\x1b',
  up: '\x1b[A', down: '\x1b[B', right: '\x1b[C', left: '\x1b[D',
  home: '\x1b[H', end: '\x1b[F', pageup: '\x1b[5~', pagedown: '\x1b[6~', insert: '\x1b[2~',
  f1: '\x1bOP', f2: '\x1bOQ', f3: '\x1bOR', f4: '\x1bOS', f5: '\x1b[15~', f6: '\x1b[17~',
  f7: '\x1b[18~', f8: '\x1b[19~', f9: '\x1b[20~', f10: '\x1b[21~', f11: '\x1b[23~', f12: '\x1b[24~',
}
const prefixed: Record<string, string> = { c: '\x03', e: '\x1b', '[': '\x1b', x: '\x18' }
const hint = 'PREFIX: c=^C  e/[=Esc  x=^X  Alt+a=Alt+a  other=cancel'

interface Key {
  key: string
  ctrl?: true
  meta?: true
  shift?: true
}

// bytes is what xterm sends for the key; undefined for one it does not know.
export function bytes(event: Key): string | undefined {
  const key = String(event.key)
  let out: string | undefined
  if (key.startsWith('\x1b')) out = key // a raw sequence, as F1 arrives
  else if (key.length > 1 && named[key.toLowerCase()] !== undefined) out = named[key.toLowerCase()]
  else if (event.ctrl && key.length === 1) out = ctrlByte(key)
  else if (key.length >= 1) out = key // a character, or a run of typed text
  if (out === undefined) return undefined
  if (key === 'tab' && event.shift) out = '\x1b[Z'
  if (event.meta && !out.startsWith('\x1b')) out = '\x1b' + out
  return out
}

function ctrlByte(ch: string): string | undefined {
  const c = ch.toLowerCase()
  if (c >= 'a' && c <= 'z') return String.fromCharCode(c.charCodeAt(0) - 96)
  const table: Record<string, string> = { '@': '\x00', ' ': '\x00', '[': '\x1b', '\\': '\x1c', ']': '\x1d', '^': '\x1e', _: '\x1f', '?': '\x7f' }
  return table[c]
}

// State is the instance's: the prefix, and the keys posted and not yet
// acknowledged. One post per frame is delivered and a later one replaces an
// undelivered one, so every post carries every key not acknowledged, each
// with its number; the hooks module types each number once and answers the
// last it took, which comes back as the props' acked.
interface State {
  pending: boolean
  from: string
  next: number
  queue: { n: number; keys: string }[]
  // seen holds the latest acknowledgement the instance was drawn with: the
  // drawing writes it, the key handler reads it.
  seen: { acked: number }
}

interface Props {
  name: string
  acked?: number
}

export default function Keys(props: Props, surface: any) {
  if (surface.state === undefined) {
    const from = Math.random().toString(36).slice(2, 12)
    const state: State = { pending: false, from, next: 1, queue: [], seen: { acked: 0 } }
    const send = (keys: string) => {
      const now = surface.state as State
      // A queue no answer drains is bounded: what falls off it is lost, as
      // it would be with nobody to type into.
      const queue = [...now.queue.filter((item) => item.n > now.seen.acked), { n: now.next, keys }].slice(-256)
      surface.setState({ ...now, pending: false, next: now.next + 1, queue })
      surface.post({ from, keys: queue })
    }
    surface.onKey((event: Key) => {
      const now = surface.state as State
      const isPrefix = event.meta === true && event.ctrl !== true && event.key === 'a'
      if (now.pending) {
        let sent: string | undefined
        if (isPrefix) sent = '\x1ba'
        else if (event.ctrl !== true && event.meta !== true) sent = prefixed[event.key]
        if (sent !== undefined) send(sent)
        else surface.setState({ ...now, pending: false })
        return
      }
      if (isPrefix) {
        surface.setState({ ...now, pending: true })
        return
      }
      const sent = bytes(event)
      if (sent !== undefined) send(sent)
    })
    surface.setState(state)
  }
  const state = surface.state as State | undefined
  if (state && typeof props?.acked === 'number' && props.acked > state.seen.acked) state.seen.acked = props.acked
  const pending = state?.pending ?? false
  const { Text } = surface.elements
  const name = String(props?.name ?? '').replace(/[\u0000-\u001f\u007f]/g, '')
  return Text({ dimColor: !pending, children: [pending ? hint : `keys → ${name} · click here to type · Alt+a: c=^C e=Esc x=^X`] })
}
